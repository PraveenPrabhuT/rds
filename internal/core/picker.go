package core

import (
	"fmt"
	"net"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
)

// PickWithFuzzyFinder presents an interactive fuzzy finder for instance selection.
func PickWithFuzzyFinder(instances []InstanceInfo) (InstanceInfo, error) {
	idx, err := fuzzyfinder.Find(
		instances,
		func(i int) string {
			return fmt.Sprintf("%-30s | %-12s | %s", instances[i].ID, instances[i].Size, instances[i].Version)
		},
		fuzzyfinder.WithHeader("Select RDS Instance"),
	)
	if err != nil {
		return InstanceInfo{}, err
	}
	return instances[idx], nil
}

// FindInstanceByEndpoint resolves an instance by matching its Endpoint.Address
// against the given host string.
func FindInstanceByEndpoint(instances []InstanceInfo, host string) (InstanceInfo, error) {
	for _, inst := range instances {
		if inst.Host == host {
			return inst, nil
		}
	}
	return InstanceInfo{}, fmt.Errorf("no instance with endpoint '%s'", host)
}

// FindInstanceByEndpointOrAlias matches by exact RDS endpoint hostname first.
// If that fails, it matches internal / Route53-style aliases that resolve to the
// same IP as an instance's Endpoint.Address (A/ALIAS records are invisible to
// CNAME lookups, so this path is needed for names like *.internal.example.com).
func FindInstanceByEndpointOrAlias(instances []InstanceInfo, host string) (InstanceInfo, error) {
	if inst, err := FindInstanceByEndpoint(instances, host); err == nil {
		return inst, nil
	}
	return findInstanceBySharedResolvedIPs(instances, host, net.LookupIP)
}

func findInstanceBySharedResolvedIPs(instances []InstanceInfo, host string, lookup func(string) ([]net.IP, error)) (InstanceInfo, error) {
	hostIPs, err := lookupHostIPSet(host, lookup)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(hostIPs) == 0 {
		return InstanceInfo{}, fmt.Errorf("no IPs resolved for %q", host)
	}

	var matches []InstanceInfo
	for _, inst := range instances {
		instIPs, err := lookupHostIPSet(inst.Host, lookup)
		if err != nil || len(instIPs) == 0 {
			continue
		}
		if hostIPs.overlaps(instIPs) {
			matches = append(matches, inst)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return InstanceInfo{}, fmt.Errorf(
			"multiple RDS instances (%s) share a resolved address with %q; use the instance id or the AWS endpoint hostname",
			strings.Join(ids, ", "), host)
	}
	return InstanceInfo{}, fmt.Errorf(
		"no instance whose endpoint shares a resolved IP with %q (VPN connected? correct AWS profile/region?)",
		host)
}

type ipSet map[string]struct{}

func (a ipSet) overlaps(b ipSet) bool {
	for ip := range a {
		if _, ok := b[ip]; ok {
			return true
		}
	}
	return false
}

func lookupHostIPSet(host string, lookup func(string) ([]net.IP, error)) (ipSet, error) {
	ips, err := lookup(host)
	if err != nil {
		return nil, err
	}
	set := make(ipSet)
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		s := ip.String()
		if v4 := ip.To4(); v4 != nil {
			s = v4.String()
		}
		set[s] = struct{}{}
	}
	return set, nil
}

// FindByName resolves an instance by exact ID, partial match, or interactive
// fuzzy selection when multiple candidates match.
func FindByName(instances []InstanceInfo, name string) (InstanceInfo, error) {
	for _, inst := range instances {
		if inst.ID == name {
			return inst, nil
		}
	}

	var matches []InstanceInfo
	for _, inst := range instances {
		if strings.Contains(inst.ID, name) {
			matches = append(matches, inst)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return PickWithFuzzyFinder(matches)
	}

	return InstanceInfo{}, fmt.Errorf("no instance matching '%s'", name)
}

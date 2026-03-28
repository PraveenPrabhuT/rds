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
// against the given host string (exact hostname or IP). When host is an IP,
// each instance's endpoint hostname is resolved to IP(s) and compared.
func FindInstanceByEndpoint(instances []InstanceInfo, host string) (InstanceInfo, error) {
	for _, inst := range instances {
		if inst.Host == host {
			return inst, nil
		}
	}
	// When URL host is an IP (e.g. private IP), match by resolving instance endpoint to IP(s).
	if ip := net.ParseIP(host); ip != nil {
		for _, inst := range instances {
			addrs, err := net.LookupHost(inst.Host)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if a == host {
					return inst, nil
				}
			}
		}
	}
	return InstanceInfo{}, fmt.Errorf("no instance with endpoint '%s'", host)
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

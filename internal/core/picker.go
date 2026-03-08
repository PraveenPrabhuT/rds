package core

import (
	"fmt"
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

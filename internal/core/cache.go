package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetCacheDir returns the directory used for RDS CLI caches.
// Respects the RDS_CACHE_DIR environment variable.
func GetCacheDir() string {
	if d := os.Getenv("RDS_CACHE_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".cache", "rds")
}

// SaveLastID persists the last connected instance ID for a profile.
func SaveLastID(id, profile string) {
	cacheDir := GetCacheDir()
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(path, []byte(id), 0644)
}

// LoadLastConnected retrieves the last connected instance for a profile.
func LoadLastConnected(instances []InstanceInfo, profile string) (InstanceInfo, error) {
	cacheDir := GetCacheDir()
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))

	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("no history found for profile '%s'", profile)
	}

	lastID := strings.TrimSpace(string(data))
	for _, inst := range instances {
		if inst.ID == lastID {
			return inst, nil
		}
	}
	return InstanceInfo{}, fmt.Errorf("last used instance '%s' not found in current profile", lastID)
}

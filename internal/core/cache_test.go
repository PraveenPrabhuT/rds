package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLastIDAndLoadLastConnected(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{
		meta,
		{ID: "other-db", Host: "other.xxx.rds.amazonaws.com", Port: 5432},
	}

	SaveLastID(testRDSTarget, "testprofile")

	got, err := LoadLastConnected(instances, "testprofile")
	if err != nil {
		t.Fatalf("LoadLastConnected: %v", err)
	}
	if got.ID != testRDSTarget {
		t.Errorf("LoadLastConnected: got ID %q, want %s", got.ID, testRDSTarget)
	}
}

func TestLoadLastConnected_NoHistory(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	instances := []InstanceInfo{{ID: "some-db", Host: "x.rds.amazonaws.com", Port: 5432}}

	_, err := LoadLastConnected(instances, "noprofile")
	if err == nil {
		t.Fatal("LoadLastConnected: expected error when no history file, got nil")
	}
	if err.Error() != "no history found for profile 'noprofile'" {
		t.Errorf("LoadLastConnected: got error %q", err.Error())
	}
}

func TestLoadLastConnected_LastIDNotInList(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	path := filepath.Join(dir, "testprofile_last_connected")
	if err := os.WriteFile(path, []byte("deleted-db"), 0644); err != nil {
		t.Fatal(err)
	}

	instances := []InstanceInfo{{ID: "current-db", Host: "x.rds.amazonaws.com", Port: 5432}}

	_, err := LoadLastConnected(instances, "testprofile")
	if err == nil {
		t.Fatal("LoadLastConnected: expected error when last ID not in list, got nil")
	}
	if err.Error() != "last used instance 'deleted-db' not found in current profile" {
		t.Errorf("LoadLastConnected: got error %q", err.Error())
	}
}

func TestGetCacheDir_Default(t *testing.T) {
	os.Unsetenv("RDS_CACHE_DIR")
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	got := GetCacheDir()
	want := filepath.Join(home, ".cache", "rds")
	if got != want {
		t.Errorf("GetCacheDir(): got %q, want %q", got, want)
	}
}

func TestGetCacheDir_Override(t *testing.T) {
	os.Setenv("RDS_CACHE_DIR", "/custom/cache")
	defer os.Unsetenv("RDS_CACHE_DIR")

	got := GetCacheDir()
	if got != "/custom/cache" {
		t.Errorf("GetCacheDir(): got %q, want /custom/cache", got)
	}
}

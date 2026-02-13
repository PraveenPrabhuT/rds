package connect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

const testRDSTarget = "metabasedev-poc"

func testMetabasePOCInstance() InstanceInfo {
	return InstanceInfo{
		ID:       testRDSTarget,
		Host:     "metabasedev-poc.xxxxx.ap-south-1.rds.amazonaws.com",
		Size:     "db.t3.micro",
		Port:     5432,
		Version:  "15",
		SourceID: "",
	}
}

func TestFindByName_ExactMatch(t *testing.T) {
	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{
		meta,
		{ID: "staging-db-1", Host: "staging.xxx.rds.amazonaws.com", Port: 5432},
	}

	got, err := findByName(instances, testRDSTarget)
	if err != nil {
		t.Fatalf("findByName: unexpected error: %v", err)
	}
	if got.ID != testRDSTarget || got.Host != meta.Host {
		t.Errorf("findByName: got %+v, want target %s", got, testRDSTarget)
	}
}

func TestFindByName_SinglePartialMatch(t *testing.T) {
	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{meta}

	got, err := findByName(instances, "metabasedev")
	if err != nil {
		t.Fatalf("findByName: unexpected error: %v", err)
	}
	if got.ID != testRDSTarget {
		t.Errorf("findByName: got ID %q, want %s", got.ID, testRDSTarget)
	}
}

func TestFindByName_NoMatch(t *testing.T) {
	instances := []InstanceInfo{testMetabasePOCInstance()}

	_, err := findByName(instances, "nonexistent")
	if err == nil {
		t.Fatal("findByName: expected error for no match, got nil")
	}
	if err.Error() != "no instance matching 'nonexistent'" {
		t.Errorf("findByName: got error %q", err.Error())
	}
}

func TestFindByName_EmptyList(t *testing.T) {
	_, err := findByName(nil, "any")
	if err == nil {
		t.Fatal("findByName: expected error for empty list, got nil")
	}
}

func TestSaveLastIDAndLoadLastConnected(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{
		meta,
		{ID: "other-db", Host: "other.xxx.rds.amazonaws.com", Port: 5432},
	}

	saveLastID(testRDSTarget, "testprofile")

	got, err := loadLastConnected(instances, "testprofile")
	if err != nil {
		t.Fatalf("loadLastConnected: %v", err)
	}
	if got.ID != testRDSTarget {
		t.Errorf("loadLastConnected: got ID %q, want %s", got.ID, testRDSTarget)
	}
}

func TestLoadLastConnected_NoHistory(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	instances := []InstanceInfo{{ID: "some-db", Host: "x.rds.amazonaws.com", Port: 5432}}

	_, err := loadLastConnected(instances, "noprofile")
	if err == nil {
		t.Fatal("loadLastConnected: expected error when no history file, got nil")
	}
	if err.Error() != "no history found for profile 'noprofile'" {
		t.Errorf("loadLastConnected: got error %q", err.Error())
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

	_, err := loadLastConnected(instances, "testprofile")
	if err == nil {
		t.Fatal("loadLastConnected: expected error when last ID not in list, got nil")
	}
	if err.Error() != "last used instance 'deleted-db' not found in current profile" {
		t.Errorf("loadLastConnected: got error %q", err.Error())
	}
}

func TestGetCacheDir_Default(t *testing.T) {
	os.Unsetenv("RDS_CACHE_DIR")
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	got := getCacheDir()
	want := filepath.Join(home, ".cache", "rds")
	if got != want {
		t.Errorf("getCacheDir(): got %q, want %q", got, want)
	}
}

func TestGetCacheDir_Override(t *testing.T) {
	os.Setenv("RDS_CACHE_DIR", "/custom/cache")
	defer os.Unsetenv("RDS_CACHE_DIR")

	got := getCacheDir()
	if got != "/custom/cache" {
		t.Errorf("getCacheDir(): got %q, want /custom/cache", got)
	}
}

func TestCacheEnvelopeRoundtrip(t *testing.T) {
	envelope := CacheEnvelope{
		Version:   CacheVersion,
		Instances: []InstanceInfo{{ID: "id1", Host: "h1", Size: "db.t3.micro", Port: 5432, Version: "14", SourceID: ""}},
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CacheEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Version != envelope.Version || len(decoded.Instances) != 1 ||
		decoded.Instances[0].ID != "id1" || decoded.Instances[0].Port != 5432 {
		t.Errorf("roundtrip: got %+v", decoded)
	}
}

func TestInstanceSecretTargetID_NoReplica(t *testing.T) {
	inst := InstanceInfo{ID: "my-db", SourceID: ""}
	got := instanceSecretTargetID(inst)
	if got != "my-db" {
		t.Errorf("instanceSecretTargetID: got %q, want my-db", got)
	}
}

func TestInstanceSecretTargetID_PlainSourceID(t *testing.T) {
	inst := InstanceInfo{ID: "replica-db", SourceID: "master-db"}
	got := instanceSecretTargetID(inst)
	if got != "master-db" {
		t.Errorf("instanceSecretTargetID: got %q, want master-db", got)
	}
}

func TestInstanceSecretTargetID_ARNSourceID(t *testing.T) {
	inst := InstanceInfo{
		ID:       "replica-db",
		SourceID: "arn:aws:rds:ap-south-1:123456789012:db:master-db",
	}
	got := instanceSecretTargetID(inst)
	if got != "master-db" {
		t.Errorf("instanceSecretTargetID: got %q, want master-db", got)
	}
}

func TestInstanceSecretTargetID_ShortARN(t *testing.T) {
	inst := InstanceInfo{ID: "my-db", SourceID: "arn:aws:rds:a:b:c"}
	got := instanceSecretTargetID(inst)
	if got != "my-db" {
		t.Errorf("instanceSecretTargetID (short ARN): got %q, want my-db", got)
	}
}

func TestBuildConnectArgs(t *testing.T) {
	inst := InstanceInfo{Host: "db.example.com", Port: 5432}
	creds := RDSCreds{Username: "admin", Password: "secret"}

	args := buildConnectArgs(inst, creds)

	want := []string{"-h", "db.example.com", "-p", "5432", "-U", "admin", "-d", "postgres"}
	if len(args) != len(want) {
		t.Fatalf("buildConnectArgs: len %d, want %d", len(args), len(want))
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("buildConnectArgs[%d]: got %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildConnectArgs_MetabasePOC(t *testing.T) {
	inst := testMetabasePOCInstance()
	creds := RDSCreds{Username: "postgres", Password: "test-secret"}

	args := buildConnectArgs(inst, creds)

	if len(args) < 8 {
		t.Fatalf("buildConnectArgs: got %d args", len(args))
	}
	wantHost := inst.Host
	wantPort := "5432"
	if args[0] != "-h" || args[1] != wantHost || args[2] != "-p" || args[3] != wantPort {
		t.Errorf("buildConnectArgs (metabasedev-poc): host/port: %v", args[:4])
	}
	if args[4] != "-U" || args[5] != creds.Username || args[6] != "-d" || args[7] != "postgres" {
		t.Errorf("buildConnectArgs (metabasedev-poc): user/db: %v", args[4:8])
	}
}

func TestValidatePritunlConnections_ProfileMappedAndConnected(t *testing.T) {
	conns := []PritunlConnection{
		{Name: "other", Connected: false},
		{Name: "sso_ackodevvpnusers", Connected: true},
	}
	err := ValidatePritunlConnections(conns, "ackodev")
	if err != nil {
		t.Errorf("ValidatePritunlConnections: unexpected error: %v", err)
	}
}

func TestValidatePritunlConnections_ProfileMappedNotConnected(t *testing.T) {
	conns := []PritunlConnection{
		{Name: "sso_ackoprodvpnusers", Connected: false},
	}
	err := ValidatePritunlConnections(conns, "ackoprod")
	if err == nil {
		t.Fatal("ValidatePritunlConnections: expected error when required VPN not connected")
	}
	if err.Error() != "required VPN profile 'sso_ackoprodvpnusers' is not connected" {
		t.Errorf("ValidatePritunlConnections: got %q", err.Error())
	}
}

func TestValidatePritunlConnections_ProfileUnmappedAnyConnected(t *testing.T) {
	conns := []PritunlConnection{{Name: "any-vpn", Connected: true}}
	err := ValidatePritunlConnections(conns, "unknown-profile")
	if err != nil {
		t.Errorf("ValidatePritunlConnections: unexpected error: %v", err)
	}
}

func TestValidatePritunlConnections_NoConnections(t *testing.T) {
	err := ValidatePritunlConnections(nil, "ackodev")
	if err == nil {
		t.Fatal("ValidatePritunlConnections: expected error when no connections")
	}
}

// TestCheckVPNWithPritunl_Integration runs the real VPN check when RDS_TEST_VERIFY_VPN=1.
// Run with: RDS_TEST_VERIFY_VPN=1 AWS_PROFILE=ackodev go test ./internal/connect/... -run TestCheckVPNWithPritunl_Integration -v
func TestCheckVPNWithPritunl_Integration(t *testing.T) {
	if os.Getenv("RDS_TEST_VERIFY_VPN") != "1" {
		t.Skip("Skipping VPN integration test; set RDS_TEST_VERIFY_VPN=1 to run")
	}
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		t.Skip("Set AWS_PROFILE to the profile whose VPN is connected (e.g. ackodev)")
	}

	err := CheckVPNWithPritunl(profile)
	if err != nil {
		t.Errorf("VPN check failed (is Pritunl running and required VPN connected?): %v", err)
	}
}

func TestGetInstancesWithCache_CacheHit(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("RDS_CACHE_DIR", dir)
	defer os.Unsetenv("RDS_CACHE_DIR")

	profile := "testprofile"
	cached := []InstanceInfo{testMetabasePOCInstance()}
	envelope := CacheEnvelope{Version: CacheVersion, Instances: cached}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(dir, "testprofile_ap-south-1_instances.json")
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := aws.Config{Region: "ap-south-1"}
	ctx := context.Background()
	got, err := GetInstancesWithCache(ctx, cfg, profile)
	if err != nil {
		t.Fatalf("GetInstancesWithCache: %v", err)
	}
	if len(got) != 1 || got[0].ID != testRDSTarget || got[0].Host != testMetabasePOCInstance().Host {
		t.Errorf("GetInstancesWithCache: got %+v", got)
	}
}

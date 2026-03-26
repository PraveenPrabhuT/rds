package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

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
	got := InstanceSecretTargetID(inst)
	if got != "my-db" {
		t.Errorf("InstanceSecretTargetID: got %q, want my-db", got)
	}
}

func TestInstanceSecretTargetID_PlainSourceID(t *testing.T) {
	inst := InstanceInfo{ID: "replica-db", SourceID: "master-db"}
	got := InstanceSecretTargetID(inst)
	if got != "master-db" {
		t.Errorf("InstanceSecretTargetID: got %q, want master-db", got)
	}
}

func TestInstanceSecretTargetID_ARNSourceID(t *testing.T) {
	inst := InstanceInfo{
		ID:       "replica-db",
		SourceID: "arn:aws:rds:ap-south-1:123456789012:db:master-db",
	}
	got := InstanceSecretTargetID(inst)
	if got != "master-db" {
		t.Errorf("InstanceSecretTargetID: got %q, want master-db", got)
	}
}

func TestInstanceSecretTargetID_ShortARN(t *testing.T) {
	inst := InstanceInfo{ID: "my-db", SourceID: "arn:aws:rds:a:b:c"}
	got := InstanceSecretTargetID(inst)
	if got != "my-db" {
		t.Errorf("InstanceSecretTargetID (short ARN): got %q, want my-db", got)
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

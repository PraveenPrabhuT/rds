package core

import (
	"testing"
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

	got, err := FindByName(instances, testRDSTarget)
	if err != nil {
		t.Fatalf("FindByName: unexpected error: %v", err)
	}
	if got.ID != testRDSTarget || got.Host != meta.Host {
		t.Errorf("FindByName: got %+v, want target %s", got, testRDSTarget)
	}
}

func TestFindByName_SinglePartialMatch(t *testing.T) {
	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{meta}

	got, err := FindByName(instances, "metabasedev")
	if err != nil {
		t.Fatalf("FindByName: unexpected error: %v", err)
	}
	if got.ID != testRDSTarget {
		t.Errorf("FindByName: got ID %q, want %s", got.ID, testRDSTarget)
	}
}

func TestFindByName_NoMatch(t *testing.T) {
	instances := []InstanceInfo{testMetabasePOCInstance()}

	_, err := FindByName(instances, "nonexistent")
	if err == nil {
		t.Fatal("FindByName: expected error for no match, got nil")
	}
	if err.Error() != "no instance matching 'nonexistent'" {
		t.Errorf("FindByName: got error %q", err.Error())
	}
}

func TestFindByName_EmptyList(t *testing.T) {
	_, err := FindByName(nil, "any")
	if err == nil {
		t.Fatal("FindByName: expected error for empty list, got nil")
	}
}

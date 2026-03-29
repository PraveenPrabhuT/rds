package core

import (
	"net"
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

func TestFindInstanceByEndpoint_Match(t *testing.T) {
	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{meta}

	got, err := FindInstanceByEndpoint(instances, meta.Host)
	if err != nil {
		t.Fatalf("FindInstanceByEndpoint: %v", err)
	}
	if got.ID != meta.ID {
		t.Errorf("FindInstanceByEndpoint: got ID %q, want %q", got.ID, meta.ID)
	}
}

func TestFindInstanceByEndpoint_NoMatch(t *testing.T) {
	instances := []InstanceInfo{testMetabasePOCInstance()}

	_, err := FindInstanceByEndpoint(instances, "nonexistent.rds.amazonaws.com")
	if err == nil {
		t.Fatal("FindInstanceByEndpoint: expected error for no match")
	}
}

func TestFindInstanceByEndpointOrAlias_ExactEndpoint(t *testing.T) {
	meta := testMetabasePOCInstance()
	instances := []InstanceInfo{meta}

	got, err := FindInstanceByEndpointOrAlias(instances, meta.Host)
	if err != nil {
		t.Fatalf("FindInstanceByEndpointOrAlias: %v", err)
	}
	if got.ID != meta.ID {
		t.Errorf("got ID %q, want %q", got.ID, meta.ID)
	}
}

func TestFindInstanceBySharedResolvedIPs_MatchByAliasIP(t *testing.T) {
	shared := net.ParseIP("10.1.2.3")
	lookup := func(host string) ([]net.IP, error) {
		switch host {
		case "alias.internal.example":
			return []net.IP{shared}, nil
		case "real.x.ap-south-1.rds.amazonaws.com":
			return []net.IP{shared}, nil
		default:
			return nil, nil
		}
	}
	instances := []InstanceInfo{
		{ID: "central-2", Host: "real.x.ap-south-1.rds.amazonaws.com", Port: 5432},
	}

	got, err := findInstanceBySharedResolvedIPs(instances, "alias.internal.example", lookup)
	if err != nil {
		t.Fatalf("findInstanceBySharedResolvedIPs: %v", err)
	}
	if got.ID != "central-2" {
		t.Errorf("got ID %q, want central-2", got.ID)
	}
}

func TestFindInstanceBySharedResolvedIPs_Ambiguous(t *testing.T) {
	shared := net.ParseIP("10.9.9.9")
	lookup := func(host string) ([]net.IP, error) {
		return []net.IP{shared}, nil
	}
	instances := []InstanceInfo{
		{ID: "a", Host: "a.rds.amazonaws.com", Port: 5432},
		{ID: "b", Host: "b.rds.amazonaws.com", Port: 5432},
	}

	_, err := findInstanceBySharedResolvedIPs(instances, "alias", lookup)
	if err == nil {
		t.Fatal("expected error when multiple instances share IP")
	}
}

package connect

import (
	"testing"

	"github.com/PraveenPrabhuT/rds/internal/core"
)

func TestBuildConnectArgs(t *testing.T) {
	inst := core.InstanceInfo{Host: "db.example.com", Port: 5432}
	creds := core.RDSCreds{Username: "admin", Password: "secret"}

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
	inst := core.InstanceInfo{
		ID:       "metabasedev-poc",
		Host:     "metabasedev-poc.xxxxx.ap-south-1.rds.amazonaws.com",
		Size:     "db.t3.micro",
		Port:     5432,
		Version:  "15",
		SourceID: "",
	}
	creds := core.RDSCreds{Username: "postgres", Password: "test-secret"}

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

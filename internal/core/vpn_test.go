package core

import (
	"os"
	"testing"
)

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
// Run with: RDS_TEST_VERIFY_VPN=1 AWS_PROFILE=ackodev go test ./internal/core/... -run TestCheckVPNWithPritunl_Integration -v
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

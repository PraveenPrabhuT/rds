package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var vpnProfileMapping = map[string]string{
	"ackodev":   "sso_ackodevvpnusers",
	"ackoprod":  "sso_ackoprodvpnusers",
	"ackolife":  "sso_ackolifevpnusers",
	"ackodrive": "sso_ackodrive_prod",
}

// ValidatePritunlConnections checks if connections satisfy the required VPN for profile.
func ValidatePritunlConnections(conns []PritunlConnection, profile string) error {
	requiredVPN, exists := vpnProfileMapping[profile]
	for _, c := range conns {
		if exists && strings.Contains(c.Name, requiredVPN) && c.Connected {
			return nil
		}
		if !exists && c.Connected {
			return nil
		}
	}
	if exists {
		return fmt.Errorf("required VPN profile '%s' is not connected", requiredVPN)
	}
	return fmt.Errorf("no active VPN connection found")
}

// CheckVPNWithPritunl runs the Pritunl client and validates the current VPN state for profile.
// If the Pritunl CLI is not installed (binary missing), it returns nil so the flow can continue
// for users who use VPN via GUI or a different client.
func CheckVPNWithPritunl(profile string) error {
	bin := "/Applications/Pritunl.app/Contents/Resources/pritunl-client"
	if _, err := os.Stat(bin); err != nil {
		return nil
	}
	out, err := exec.Command(bin, "list", "-j").Output()
	if err != nil {
		return nil
	}
	var conns []PritunlConnection
	if err := json.Unmarshal(out, &conns); err != nil {
		return nil
	}
	return ValidatePritunlConnections(conns, profile)
}

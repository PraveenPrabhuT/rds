package connect

import (
	"context"
	"fmt"
	"os/exec"
)

// Options configures a connect run (profile, region, flags, args).
type Options struct {
	Profile       string
	Region        string
	LastConnected bool
	Args          []string
}

// Run performs optional VPN check (if Pritunl CLI is present), instance selection, credential fetch, and launches pgcli/psql or native client.
// It uses the AWS SDK and Pritunl; profile and region come from Options.
func Run(ctx context.Context, opts Options) error {
	if err := CheckVPNWithPritunl(opts.Profile); err != nil {
		fmt.Printf("⚠️  VPN check: %v (continuing anyway)\n", err)
	}

	cfg, homeRegion, err := loadAWSConfig(ctx, opts.Profile, opts.Region)
	if err != nil {
		return err
	}

	instances, err := GetInstancesWithCache(ctx, cfg, opts.Profile)
	if err != nil {
		return fmt.Errorf("fetch instances: %w", err)
	}

	var selected InstanceInfo
	var selectErr error

	if len(opts.Args) > 0 {
		selected, selectErr = findByName(instances, opts.Args[0])
	} else if opts.LastConnected {
		selected, selectErr = loadLastConnected(instances, opts.Profile)
	} else {
		selected, selectErr = pickWithFuzzyFinder(instances)
	}

	if selectErr != nil {
		return fmt.Errorf("selection: %w", selectErr)
	}

	creds, err := getRDSCredentials(ctx, cfg, selected, homeRegion)
	if err != nil {
		return fmt.Errorf("secrets: %w", err)
	}

	saveLastID(selected.ID, opts.Profile)
	fmt.Printf("\n🚀 Target: %s [%s]\n", selected.ID, selected.Host)

	if path, err := exec.LookPath("pgcli"); err == nil {
		fmt.Println("✨ Launching pgcli...")
		executeExternal(path, selected, creds)
		return nil
	}
	if path, err := exec.LookPath("psql"); err == nil {
		fmt.Println("📂 Launching psql...")
		executeExternal(path, selected, creds)
		return nil
	}

	fmt.Println("⚠️  No binary clients found. Launching Native Fallback...")
	runNativeConnect(selected.Host, selected.Port, creds.Username, creds.Password, "postgres")
	return nil
}

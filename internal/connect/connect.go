package connect

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/PraveenPrabhuT/rds/internal/core"
)

// Options configures a connect run (profile, region, flags, args).
type Options struct {
	Profile       string
	Region        string
	LastConnected bool
	Host          string
	Port          int
	DB            string
	Args          []string
}

// Run performs optional VPN check (if Pritunl CLI is present), instance selection, credential fetch, and launches pgcli/psql or native client.
func Run(ctx context.Context, opts Options) error {
	if err := core.CheckVPNWithPritunl(opts.Profile); err != nil {
		fmt.Printf("⚠️  VPN check: %v (continuing anyway)\n", err)
	}

	cfg, homeRegion, err := core.LoadAWSConfig(ctx, opts.Profile, opts.Region)
	if err != nil {
		return err
	}

	instances, err := core.GetInstancesWithCache(ctx, cfg, opts.Profile)
	if err != nil {
		return fmt.Errorf("fetch instances: %w", err)
	}

	var selected core.InstanceInfo
	var selectErr error

	switch {
	case opts.Host != "":
		selected, selectErr = core.FindInstanceByEndpoint(instances, opts.Host)
	case len(opts.Args) > 0:
		selected, selectErr = core.FindByName(instances, opts.Args[0])
	case opts.LastConnected:
		selected, selectErr = core.LoadLastConnected(instances, opts.Profile)
	default:
		selected, selectErr = core.PickWithFuzzyFinder(instances)
	}

	if selectErr != nil {
		return fmt.Errorf("selection: %w", selectErr)
	}

	if opts.Port != 0 && int32(opts.Port) != selected.Port {
		selected.Port = int32(opts.Port)
	}

	dbname := opts.DB
	if dbname == "" {
		dbname = "postgres"
	}

	creds, err := core.GetRDSCredentials(ctx, cfg, selected, homeRegion)
	if err != nil {
		return fmt.Errorf("secrets: %w", err)
	}

	core.SaveLastID(selected.ID, opts.Profile)
	fmt.Printf("\n🚀 Target: %s [%s]\n", selected.ID, selected.Host)

	if path, err := exec.LookPath("pgcli"); err == nil {
		fmt.Println("✨ Launching pgcli...")
		executeExternal(path, selected, creds, dbname)
		return nil
	}
	if path, err := exec.LookPath("psql"); err == nil {
		fmt.Println("📂 Launching psql...")
		executeExternal(path, selected, creds, dbname)
		return nil
	}

	fmt.Println("⚠️  No binary clients found. Launching Native Fallback...")
	runNativeConnect(selected.Host, selected.Port, creds.Username, creds.Password, dbname)
	return nil
}

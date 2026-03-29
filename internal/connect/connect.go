package connect

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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
	JDBCURL       string
	ShowJDBC      bool
	CopyJDBC      bool
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
	var connectHost string
	var connectPort int32
	dbname := opts.DB
	if dbname == "" {
		dbname = "postgres"
	}

	jdbcURL := opts.JDBCURL
	if jdbcURL == "" && len(opts.Args) == 1 && strings.HasPrefix(opts.Args[0], "jdbc:postgresql://") {
		jdbcURL = opts.Args[0]
	}

	if jdbcURL != "" {
		urlHost, urlPort, urlDB, parseErr := ParseJDBCURL(jdbcURL)
		if parseErr != nil {
			return fmt.Errorf("parse JDBC URL: %w", parseErr)
		}

		connectHost = urlHost
		connectPort = int32(urlPort)
		dbname = urlDB

		resolved, err := ResolveCNAME(urlHost)
		if err != nil {
			return fmt.Errorf("resolve CNAME for %s: %w", urlHost, err)
		}

		selected, selectErr = core.FindInstanceByEndpointOrAlias(instances, resolved)
		if selectErr != nil {
			return fmt.Errorf("no RDS instance found for host '%s' (from JDBC): %w", resolved, selectErr)
		}
	} else {
		switch {
		case opts.Host != "":
			selected, selectErr = core.FindInstanceByEndpointOrAlias(instances, opts.Host)
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

		connectHost = selected.Host
		connectPort = selected.Port
	}

	if opts.Port != 0 {
		connectPort = int32(opts.Port)
	}

	creds, err := core.GetRDSCredentials(ctx, cfg, selected, homeRegion)
	if err != nil {
		return fmt.Errorf("secrets: %w", err)
	}

	core.SaveLastID(selected.ID, opts.Profile)
	fmt.Printf("\n🚀 Target: %s [%s]\n", selected.ID, connectHost)

	if opts.ShowJDBC {
		jdbcURL := BuildJDBCURL(connectHost, connectPort, creds.Username, creds.Password, dbname)
		fmt.Printf("\n📋 JDBC URL:\n%s\n\n", jdbcURL)
		if opts.CopyJDBC {
			copyToClipboard(jdbcURL)
		}
	}

	connInfo := core.InstanceInfo{
		ID:   selected.ID,
		Host: connectHost,
		Port: connectPort,
	}

	if path, err := exec.LookPath("pgcli"); err == nil {
		fmt.Println("✨ Launching pgcli...")
		executeExternal(path, connInfo, creds, dbname)
		return nil
	}
	if path, err := exec.LookPath("psql"); err == nil {
		fmt.Println("📂 Launching psql...")
		executeExternal(path, connInfo, creds, dbname)
		return nil
	}

	fmt.Println("⚠️  No binary clients found. Launching Native Fallback...")
	runNativeConnect(connectHost, connectPort, creds.Username, creds.Password, dbname)
	return nil
}

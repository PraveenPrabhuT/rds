package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/chzyer/readline"
	"github.com/ktr0731/go-fuzzyfinder"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

// --- Structs ---
const CacheVersion = "v2" // Increment this when InstanceInfo changes

type CacheEnvelope struct {
	Version   string         `json:"version"`
	Instances []InstanceInfo `json:"instances"`
}
type InstanceInfo struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Size     string `json:"size"`
	Port     int32  `json:"port"`
	Version  string `json:"version"`
	SourceID string `json:"source_id"`
}

type RDSCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type PritunlConnection struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// --- Variables ---

var (
	lastConnected bool
	instanceName  string
)

// We define the command here. Note: rootCmd must be accessible.
// If your rootCmd is in root.go (common Cobra structure), this works perfectly.
// In cmd/connect.go
var connectCmd = &cobra.Command{
	Use:   "connect [rds-identifier]",
	Short: "Connect to an RDS PostgreSQL instance",
	Long: `Connect dynamically fetches credentials from AWS Secrets Manager 
and establishes a connection using pgcli, psql, or a native Go fallback. 
It requires an active Pritunl VPN connection matching the AWS Profile.`,
	Example: `  # Interactive selection
  rds connect

  # Direct connection with partial name
  rds connect acko-health

  # Reconnect to the last used instance for the current profile
  rds connect -l`,
	Args: cobra.MaximumNArgs(1),
	Run:  runConnect,
}

func init() {
	connectCmd.Flags().BoolVarP(&lastConnected, "last", "l", false, "Connect to the last used RDS instance")

	connectCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete the first argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		ctx := context.Background()

		// 1. Logic to extract the region from the command line flags (-r or --region)
		// Cobra completion passes the current flag state to us
		rFlag, _ := cmd.Flags().GetString("region")

		// 2. Prepare Config Options
		opts := []func(*config.LoadOptions) error{
			config.WithSharedConfigProfile(awsProfile),
		}

		// Priority: -r flag > AWS_REGION env > default config
		if rFlag != "" {
			opts = append(opts, config.WithRegion(rFlag))
		} else if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			opts = append(opts, config.WithRegion(envRegion))
		}

		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		// 3. Fetch/Cache instances for the SPECIFIC region
		instances, err := getInstancesWithCache(ctx, cfg)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string
		for _, inst := range instances {
			if strings.HasPrefix(inst.ID, toComplete) {
				// Return ID \t Description (Size)
				completions = append(completions, fmt.Sprintf("%s\t%s", inst.ID, inst.Size))
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}

	rootCmd.AddCommand(connectCmd)
}

// --- Main Logic ---

func runConnect(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	// 1. VPN Check with Mapping
	if err := checkVPNWithPritunl(); err != nil {
		fmt.Printf("⚠️  VPN Error: %v\n", err)
		os.Exit(1)
	}

	baseCfg, _ := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(awsProfile),
		config.WithRegion("ap-south-1"),
	)
	homeRegion := baseCfg.Region // Now guaranteed to be ap-south-1

	// awsProfile is global in package 'cmd'
	// 1. Prepare Config Options
	opts := []func(*config.LoadOptions) error{
		config.WithSharedConfigProfile(awsProfile),
	}

	// If a region was passed via -r flag, use it; otherwise use default from .aws/config
	if awsRegion != "" {
		opts = append(opts, config.WithRegion(awsRegion))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)

	if err != nil {
		fmt.Printf("❌ Unable to load SDK config: %v\n", err)
		os.Exit(1)
	}

	// 2. Fetch/Cache Instances
	instances, err := getInstancesWithCache(ctx, cfg)
	if err != nil {
		fmt.Printf("❌ Failed to fetch instances: %v\n", err)
		os.Exit(1)
	}

	// 3. Selection Logic Refactored
	var selected InstanceInfo
	var selectErr error

	// Priority: Positional Arg > Last Connected Flag > Fuzzy Finder
	if len(args) > 0 {
		// User provided: rds connect <partial_name>
		selected, selectErr = findByName(instances, args[0])
	} else if lastConnected {
		// User provided: rds connect -l
		selected, selectErr = loadLastConnected(instances, awsProfile)
	} else {
		// User provided: rds connect (Interactive mode)
		selected, selectErr = pickWithFuzzyFinder(instances)
	}

	if selectErr != nil {
		fmt.Printf("❌ Selection Error: %v\n", selectErr)
		return
	}

	// 4. Fetch Secrets
	creds, err := getRDSCredentials(ctx, cfg, selected, homeRegion)
	if err != nil {
		fmt.Printf("❌ Failed to retrieve secrets: %v\n", err)
		return
	}

	// 5. Connection Execution
	saveLastID(selected.ID, awsProfile)
	fmt.Printf("\n🚀 Target: %s [%s]\n", selected.ID, selected.Host)

	// Strategy: pgcli -> psql -> Native Fallback
	if path, err := exec.LookPath("pgcli"); err == nil {
		fmt.Println("✨ Launching pgcli...")
		executeExternal(path, selected, creds)
	} else if path, err := exec.LookPath("psql"); err == nil {
		fmt.Println("📂 Launching psql...")
		executeExternal(path, selected, creds)
	} else {
		fmt.Println("⚠️  No binary clients found. Launching Native Fallback...")
		runNativeConnect(selected.Host, selected.Port, creds.Username, creds.Password, "postgres")
	}
}

// --- Helpers ---

func checkVPNWithPritunl() error {
	vpnMapping := map[string]string{
		"ackodev":   "sso_ackodevvpnusers",
		"ackoprod":  "sso_ackoprodvpnusers",
		"ackolife":  "sso_ackolifevpnusers",
		"ackodrive": "sso_ackodrive_prod",
	}

	requiredVPN, exists := vpnMapping[awsProfile]
	bin := "/Applications/Pritunl.app/Contents/Resources/pritunl-client"
	out, err := exec.Command(bin, "list", "-j").Output()
	if err != nil {
		return fmt.Errorf("pritunl-client utility not found or failed to execute")
	}

	var conns []PritunlConnection
	if err := json.Unmarshal(out, &conns); err != nil {
		return fmt.Errorf("failed to parse Pritunl JSON output")
	}

	for _, c := range conns {
		if exists && strings.Contains(c.Name, requiredVPN) && c.Connected {
			return nil
		} else if !exists && c.Connected {
			return nil
		}
	}

	if exists {
		return fmt.Errorf("required VPN profile '%s' is not connected", requiredVPN)
	}
	return fmt.Errorf("no active VPN connection found")
}

func getInstancesWithCache(ctx context.Context, cfg aws.Config) ([]InstanceInfo, error) {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "rds")
	// NEW: Cache key includes both Profile AND Region
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s_%s_instances.json", awsProfile, cfg.Region))

	// 1. Read Cache
	if data, err := os.ReadFile(cacheFile); err == nil {
		var envelope CacheEnvelope
		if err := json.Unmarshal(data, &envelope); err == nil {
			info, _ := os.Stat(cacheFile)
			// Ensure version match and TTL (1 hour)
			if envelope.Version == CacheVersion && time.Since(info.ModTime()) < time.Hour {
				return envelope.Instances, nil
			}
		}
	}

	fmt.Printf("🔍 Fetching RDS instances [%s:%s]...\n", awsProfile, cfg.Region)

	// 2. Fetch from AWS
	rdsClient := rds.NewFromConfig(cfg)
	out, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo
	for _, db := range out.DBInstances {
		if aws.ToString(db.Engine) == "postgres" {
			instances = append(instances, InstanceInfo{
				ID:       aws.ToString(db.DBInstanceIdentifier),
				Host:     aws.ToString(db.Endpoint.Address),
				Size:     aws.ToString(db.DBInstanceClass),
				Port:     *db.Endpoint.Port,
				Version:  aws.ToString(db.EngineVersion),
				SourceID: aws.ToString(db.ReadReplicaSourceDBInstanceIdentifier),
			})
		}
	}

	// 3. Save Cache
	os.MkdirAll(cacheDir, 0755)
	newCacheData, _ := json.Marshal(CacheEnvelope{
		Version:   CacheVersion,
		Instances: instances,
	})
	os.WriteFile(cacheFile, newCacheData, 0644)

	return instances, nil
}

func pickWithFuzzyFinder(instances []InstanceInfo) (InstanceInfo, error) {
	idx, err := fuzzyfinder.Find(
		instances,
		func(i int) string {
			return fmt.Sprintf("%-30s | %-12s | %s", instances[i].ID, instances[i].Size, instances[i].Version)
		},
		fuzzyfinder.WithHeader("Select RDS Instance"),
	)
	if err != nil {
		return InstanceInfo{}, err
	}
	return instances[idx], nil
}

func findByName(instances []InstanceInfo, name string) (InstanceInfo, error) {
	// PASS 1: Check for an exact match first
	for _, inst := range instances {
		if inst.ID == name {
			return inst, nil
		}
	}

	// PASS 2: If no exact match, look for partial matches
	var matches []InstanceInfo
	for _, inst := range instances {
		if strings.Contains(inst.ID, name) {
			matches = append(matches, inst)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	} else if len(matches) > 1 {
		// Only show fuzzy finder if there was no exact match AND multiple partial matches
		return pickWithFuzzyFinder(matches)
	}

	return InstanceInfo{}, fmt.Errorf("no instance matching '%s'", name)
}

func getRDSCredentials(ctx context.Context, cfg aws.Config, selected InstanceInfo, homeRegion string) (RDSCreds, error) {
	// Determine the secret ID (handling the ARN pivot)
	secretTargetID := selected.ID
	if selected.SourceID != "" {
		if strings.HasPrefix(selected.SourceID, "arn:aws:rds:") {
			parts := strings.Split(selected.SourceID, ":")
			if len(parts) >= 7 {
				secretTargetID = parts[6]
				fmt.Printf("🌐 DR Replica detected. Fetching master secret '%s' from primary region: %s\n",
					secretTargetID, homeRegion)
			}
		} else {
			secretTargetID = selected.SourceID
		}
	}

	sm := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.Region = homeRegion
	})

	secretID := fmt.Sprintf("root/%s/psql", secretTargetID)
	out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &secretID})

	if err != nil {
		return RDSCreds{}, fmt.Errorf("failed to fetch secret '%s' in %s: %w", secretID, homeRegion, err)
	}

	var creds RDSCreds
	if err := json.Unmarshal([]byte(*out.SecretString), &creds); err != nil {
		return RDSCreds{}, err
	}
	return creds, nil
}

func executeExternal(bin string, inst InstanceInfo, creds RDSCreds) {
	args := []string{"-h", inst.Host, "-p", fmt.Sprintf("%d", inst.Port), "-U", creds.Username, "-d", "postgres"}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", creds.Password))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Run()
}

// --- Persistence ---

func saveLastID(id, profile string) {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "rds")
	// Use the profile name in the filename for isolation
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))

	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(path, []byte(id), 0644)
}

func loadLastConnected(instances []InstanceInfo, profile string) (InstanceInfo, error) {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "rds")
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))

	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("no history found for profile '%s'", profile)
	}

	lastID := string(data)
	for _, inst := range instances {
		if inst.ID == lastID {
			return inst, nil
		}
	}
	return InstanceInfo{}, fmt.Errorf("last used instance '%s' not found in current profile", lastID)
}

// --- Native Fallback Implementation ---

func runNativeConnect(host string, port int32, user, password, dbname string) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("❌ Connection Error: %v\n", err)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ Connection Failed: %v\n", err)
		return
	}

	fmt.Printf("✅ Connected to %s (Native Mode)\n", host)
	rl, _ := readline.NewEx(&readline.Config{
		Prompt: dbname + "=> ",
	})
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		query := strings.TrimSpace(line)
		if query == "" {
			continue
		}
		if query == "exit" || query == "quit" {
			break
		}
		executeAndPrint(db, query)
	}
}

func executeAndPrint(db *sql.DB, query string) {
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, " | "))
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		rows.Scan(columnPointers...)
		for _, val := range columns {
			fmt.Printf("%v | ", val)
		}
		fmt.Println()
	}
}

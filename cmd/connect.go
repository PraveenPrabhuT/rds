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

type InstanceInfo struct {
	ID      string `json:"id"`
	Host    string `json:"host"`
	Size    string `json:"size"`
	Port    int32  `json:"port"`
	Version string `json:"version"`
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
var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to an RDS PostgreSQL instance",
	Run:   runConnect,
}

func init() {
	connectCmd.Flags().BoolVarP(&lastConnected, "last", "l", false, "Connect to the last used RDS instance")

	connectCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete the first argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(awsProfile))
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		instances, err := getInstancesWithCache(ctx, cfg)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string
		for _, inst := range instances {
			if strings.HasPrefix(inst.ID, toComplete) {
				// FIX: Use \t (Tab).
				// Zsh will display the size but ONLY insert the ID into your terminal.
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

	// awsProfile is global in package 'cmd'
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(awsProfile))
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
	creds, err := getRDSCredentials(ctx, cfg, selected.ID)
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
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s_instances.json", awsProfile))

	if info, err := os.Stat(cacheFile); err == nil && time.Since(info.ModTime()) < time.Hour {
		data, _ := os.ReadFile(cacheFile)
		var insts []InstanceInfo
		if err := json.Unmarshal(data, &insts); err == nil {
			return insts, nil
		}
	}

	fmt.Printf("🔍 Fetching RDS instances [Profile: %s]...\n", awsProfile)
	client := rds.NewFromConfig(cfg)
	out, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo
	for _, db := range out.DBInstances {
		if aws.ToString(db.Engine) == "postgres" {
			instances = append(instances, InstanceInfo{
				ID:      aws.ToString(db.DBInstanceIdentifier),
				Host:    aws.ToString(db.Endpoint.Address),
				Size:    aws.ToString(db.DBInstanceClass),
				Port:    *db.Endpoint.Port, // Safely dereferencing
				Version: aws.ToString(db.EngineVersion),
			})
		}
	}

	os.MkdirAll(cacheDir, 0755)
	data, _ := json.Marshal(instances)
	os.WriteFile(cacheFile, data, 0644)
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
	var matches []InstanceInfo
	for _, inst := range instances {
		if strings.Contains(inst.ID, name) {
			matches = append(matches, inst)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	} else if len(matches) > 1 {
		return pickWithFuzzyFinder(matches)
	}
	return InstanceInfo{}, fmt.Errorf("no instance matching '%s'", name)
}

func getRDSCredentials(ctx context.Context, cfg aws.Config, id string) (RDSCreds, error) {
	sm := secretsmanager.NewFromConfig(cfg)
	secretID := fmt.Sprintf("root/%s/psql", id)
	out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &secretID})
	if err != nil {
		return RDSCreds{}, err
	}
	var creds RDSCreds
	json.Unmarshal([]byte(*out.SecretString), &creds)
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

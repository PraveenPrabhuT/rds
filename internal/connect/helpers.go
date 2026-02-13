package connect

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
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/chzyer/readline"
	"github.com/ktr0731/go-fuzzyfinder"
	_ "github.com/lib/pq"
)

func getCacheDir() string {
	if d := os.Getenv("RDS_CACHE_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".cache", "rds")
}

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
		// CLI not installed — skip check and continue
		return nil
	}
	out, err := exec.Command(bin, "list", "-j").Output()
	if err != nil {
		// CLI exists but failed (e.g. not running) — skip check and continue
		return nil
	}
	var conns []PritunlConnection
	if err := json.Unmarshal(out, &conns); err != nil {
		return nil
	}
	return ValidatePritunlConnections(conns, profile)
}

// GetInstancesWithCache returns RDS PostgreSQL instances (cached or from AWS). Profile is used for cache key.
func GetInstancesWithCache(ctx context.Context, cfg aws.Config, profile string) ([]InstanceInfo, error) {
	cacheDir := getCacheDir()
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s_%s_instances.json", profile, cfg.Region))

	if data, err := os.ReadFile(cacheFile); err == nil {
		var envelope CacheEnvelope
		if err := json.Unmarshal(data, &envelope); err == nil {
			info, _ := os.Stat(cacheFile)
			if envelope.Version == CacheVersion && time.Since(info.ModTime()) < time.Hour {
				return envelope.Instances, nil
			}
		}
	}

	fmt.Printf("🔍 Fetching RDS instances [%s:%s]...\n", profile, cfg.Region)

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
	for _, inst := range instances {
		if inst.ID == name {
			return inst, nil
		}
	}

	var matches []InstanceInfo
	for _, inst := range instances {
		if strings.Contains(inst.ID, name) {
			matches = append(matches, inst)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return pickWithFuzzyFinder(matches)
	}

	return InstanceInfo{}, fmt.Errorf("no instance matching '%s'", name)
}

func instanceSecretTargetID(selected InstanceInfo) string {
	if selected.SourceID == "" {
		return selected.ID
	}
	if strings.HasPrefix(selected.SourceID, "arn:aws:rds:") {
		parts := strings.Split(selected.SourceID, ":")
		if len(parts) >= 7 {
			return parts[6]
		}
		return selected.ID
	}
	return selected.SourceID
}

func getRDSCredentials(ctx context.Context, cfg aws.Config, selected InstanceInfo, homeRegion string) (RDSCreds, error) {
	secretTargetID := instanceSecretTargetID(selected)
	if selected.SourceID != "" && strings.HasPrefix(selected.SourceID, "arn:aws:rds:") && secretTargetID != selected.ID {
		fmt.Printf("🌐 DR Replica detected. Fetching master secret '%s' from primary region: %s\n",
			secretTargetID, homeRegion)
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

func buildConnectArgs(inst InstanceInfo, creds RDSCreds) []string {
	return []string{"-h", inst.Host, "-p", fmt.Sprintf("%d", inst.Port), "-U", creds.Username, "-d", "postgres"}
}

func executeExternal(bin string, inst InstanceInfo, creds RDSCreds) {
	args := buildConnectArgs(inst, creds)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", creds.Password))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	_ = startAndWait(cmd)
}

func saveLastID(id, profile string) {
	cacheDir := getCacheDir()
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(path, []byte(id), 0644)
}

func loadLastConnected(instances []InstanceInfo, profile string) (InstanceInfo, error) {
	cacheDir := getCacheDir()
	path := filepath.Join(cacheDir, fmt.Sprintf("%s_last_connected", profile))

	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("no history found for profile '%s'", profile)
	}

	lastID := strings.TrimSpace(string(data))
	for _, inst := range instances {
		if inst.ID == lastID {
			return inst, nil
		}
	}
	return InstanceInfo{}, fmt.Errorf("last used instance '%s' not found in current profile", lastID)
}

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

func loadAWSConfig(ctx context.Context, profile, region string) (aws.Config, string, error) {
	homeCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(profile),
		awsconfig.WithRegion("ap-south-1"),
	)
	if err != nil {
		return aws.Config{}, "", fmt.Errorf("load AWS config (home): %w", err)
	}
	homeRegion := homeCfg.Region

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithSharedConfigProfile(profile),
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, "", fmt.Errorf("load AWS config: %w", err)
	}
	return cfg, homeRegion, nil
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

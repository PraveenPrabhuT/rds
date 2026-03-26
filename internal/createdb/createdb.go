package createdb

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/PraveenPrabhuT/rds/internal/core"
	"github.com/jackc/pgx/v5"
)

const passwordLength = 20

// Run orchestrates the full database creation flow matching the Ansible playbook.
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

	// Filter out read replicas
	var primary []core.InstanceInfo
	for _, inst := range instances {
		if inst.SourceID == "" {
			primary = append(primary, inst)
		}
	}

	var selected core.InstanceInfo
	var selectErr error

	switch {
	case opts.Host != "":
		selected, selectErr = core.FindInstanceByEndpoint(primary, opts.Host)
	case len(opts.Args) > 0:
		selected, selectErr = core.FindByName(primary, opts.Args[0])
	default:
		selected, selectErr = core.PickWithFuzzyFinder(primary)
	}
	if selectErr != nil {
		return fmt.Errorf("instance selection: %w", selectErr)
	}

	if opts.Port != 0 {
		selected.Port = int32(opts.Port)
	}

	creds, err := core.GetRDSCredentials(ctx, cfg, selected, homeRegion)
	if err != nil {
		return fmt.Errorf("secrets: %w", err)
	}

	dbName := opts.DBName
	if dbName == "" {
		fmt.Print("Enter database name: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			dbName = strings.TrimSpace(scanner.Text())
		}
		if dbName == "" {
			return fmt.Errorf("database name is required")
		}
	}

	users, err := generateCredentials(dbName, opts)
	if err != nil {
		return fmt.Errorf("generate passwords: %w", err)
	}

	steps := BuildSteps(dbName, opts.Schema, users)

	printSummary(selected, dbName, opts, users)

	if !confirm() {
		fmt.Println("Aborted.")
		return nil
	}

	if opts.DryRun {
		printDryRun(steps)
		return nil
	}

	results := executeSteps(ctx, steps, selected, creds, users[0], dbName, opts)

	printResults(results)
	for _, r := range results {
		if r.Status == "FAILED" {
			return fmt.Errorf("step %q failed: %w", r.Name, r.Error)
		}
	}

	printCredentialsTable(selected, dbName, users)

	if promptStoreSecrets() {
		if err := StoreCredentials(ctx, cfg, homeRegion, dbName, selected.ID, users); err != nil {
			return fmt.Errorf("store secrets: %w", err)
		}
		fmt.Printf("✅ Credentials stored at %s/%s/psql\n", dbName, selected.ID)
	}

	return nil
}

func generateCredentials(dbName string, opts Options) ([]UserCredentials, error) {
	type userDef struct {
		suffix    string
		role      string
		connLimit int
	}

	defs := []userDef{
		{"", "migration", opts.MigrationConnLimit},
		{"_ro_v1", "read-only", opts.ROConnLimit},
		{"_ro_v2", "read-only", opts.ROConnLimit},
		{"_rw_v1", "read-write", opts.RWConnLimit},
		{"_rw_v2", "read-write", opts.RWConnLimit},
	}

	users := make([]UserCredentials, 0, len(defs))
	for _, d := range defs {
		pw, err := GeneratePassword(passwordLength)
		if err != nil {
			return nil, err
		}
		users = append(users, UserCredentials{
			Username:  dbName + d.suffix,
			Password:  pw,
			Role:      d.role,
			ConnLimit: d.connLimit,
		})
	}
	return users, nil
}

func printSummary(inst core.InstanceInfo, dbName string, opts Options, users []UserCredentials) {
	fmt.Println()
	fmt.Println("=== Database Creation Summary ===")
	fmt.Printf("  Instance:  %s [%s]\n", inst.ID, inst.Host)
	fmt.Printf("  Database:  %s\n", dbName)
	fmt.Printf("  Schema:    %s\n", opts.Schema)
	fmt.Println("  Users:")
	for _, u := range users {
		fmt.Printf("    - %-20s (%s, conn_limit=%d)\n", u.Username, u.Role, u.ConnLimit)
	}
	fmt.Printf("  IAM user:  %s_iam\n", dbName)
	if opts.DryRun {
		fmt.Println("  Mode:      DRY RUN (no changes will be made)")
	}
	fmt.Println()
}

func confirm() bool {
	fmt.Print("Proceed? [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

func printDryRun(steps []Step) {
	fmt.Println("\n=== DRY RUN: SQL Statements ===")
	for i, step := range steps {
		fmt.Printf("\n-- Step %d: %s (as %s -> %s)\n", i+1, step.Name, step.ConnectAs, step.ConnectDB)
		for _, stmt := range step.Statements {
			fmt.Printf("%s;\n", stmt)
		}
	}
	fmt.Println()
}

func executeSteps(
	ctx context.Context,
	steps []Step,
	inst core.InstanceInfo,
	superCreds core.RDSCreds,
	migrationUser UserCredentials,
	dbName string,
	opts Options,
) []StepResult {
	var results []StepResult
	failed := false

	for i, step := range steps {
		if failed {
			results = append(results, StepResult{Name: step.Name, Status: "SKIPPED"})
			continue
		}

		fmt.Printf("Step %d/%d: %s... ", i+1, len(steps), step.Name)

		var user, password, targetDB string

		switch step.ConnectAs {
		case "superuser":
			user = superCreds.Username
			password = superCreds.Password
		case "migration":
			user = migrationUser.Username
			password = migrationUser.Password
		}

		switch step.ConnectDB {
		case "default":
			targetDB = opts.DefaultDB
		case "newdb":
			targetDB = dbName
		}

		conn, err := core.NewPgxConn(ctx, inst.Host, inst.Port, user, password, targetDB)
		if err != nil {
			fmt.Printf("FAILED (connect: %v)\n", err)
			results = append(results, StepResult{Name: step.Name, Status: "FAILED", Error: err})
			failed = true
			continue
		}

		stepErr := executeStatements(ctx, conn, step.Statements, opts.Force)
		conn.Close(ctx)

		if stepErr != nil {
			fmt.Printf("FAILED (%v)\n", stepErr)
			results = append(results, StepResult{Name: step.Name, Status: "FAILED", Error: stepErr})
			failed = true
		} else {
			fmt.Println("done")
			results = append(results, StepResult{Name: step.Name, Status: "done"})
		}
	}
	return results
}

func executeStatements(ctx context.Context, conn *pgx.Conn, stmts []string, force bool) error {
	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			if force && isAlreadyExistsError(err) {
				continue
			}
			return fmt.Errorf("exec %q: %w", truncate(stmt, 80), err)
		}
	}
	return nil
}

func isAlreadyExistsError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate key")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func printResults(results []StepResult) {
	fmt.Println()
	for _, r := range results {
		switch r.Status {
		case "done":
			fmt.Printf("  ✅ %s\n", r.Name)
		case "FAILED":
			fmt.Printf("  ❌ %s: %v\n", r.Name, r.Error)
		case "SKIPPED":
			fmt.Printf("  ⏭️  %s (skipped)\n", r.Name)
		}
	}
	fmt.Println()
}

func printCredentialsTable(inst core.InstanceInfo, dbName string, users []UserCredentials) {
	fmt.Printf("\nDatabase %q created on %s\n\n", dbName, inst.Host)
	fmt.Println("Credentials:")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USER\tPASSWORD\tROLE\tCONN LIMIT")
	fmt.Fprintln(w, "----\t--------\t----\t----------")
	for _, u := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", u.Username, u.Password, u.Role, u.ConnLimit)
	}
	w.Flush()
	fmt.Println()
}

func promptStoreSecrets() bool {
	fmt.Print("Store credentials in AWS Secrets Manager? [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

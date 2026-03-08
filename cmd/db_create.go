package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/PraveenPrabhuT/rds/internal/core"
	"github.com/PraveenPrabhuT/rds/internal/createdb"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"
)

var (
	createHost          string
	createPort          int
	createSchema        string
	createDefaultDB     string
	migrationConnLimit  int
	rwConnLimit         int
	roConnLimit         int
	createDryRun        bool
	createForce         bool
)

var dbCreateCmd = &cobra.Command{
	Use:   "create [db-name]",
	Short: "Create a database with users and permissions on an RDS instance",
	Long: `Create provisions a new PostgreSQL database on an RDS instance along with
a standard set of users and permissions matching the organization's playbook:

  - Migration user (all privileges on the database)
  - Read-only users (v1 and v2 with role inheritance)
  - Read-write users (v1 and v2 with role inheritance)
  - IAM database user for AWS IAM authentication

Superuser credentials are fetched automatically from AWS Secrets Manager.
Read replicas are filtered out from the instance picker.`,
	Example: `  # Interactive instance and database name selection
  rds db create

  # Create database "Pricing" with interactive instance picker
  rds db create Pricing

  # Target a specific RDS host
  rds db create Pricing --host my-rds.abc.ap-south-1.rds.amazonaws.com

  # Dry run to preview SQL without executing
  rds db create Pricing --dry-run

  # Use custom schema and connection limits
  rds db create Pricing --schema app --migration-conn-limit 20`,
	Args: cobra.MaximumNArgs(1),
	Run:  runDBCreate,
}

func init() {
	dbCreateCmd.Flags().StringVar(&createHost, "host", "", "RDS host endpoint (bypasses instance picker)")
	dbCreateCmd.Flags().IntVar(&createPort, "port", 5432, "PostgreSQL port")
	dbCreateCmd.Flags().StringVarP(&createSchema, "schema", "s", "public", "Target schema for privilege grants")
	dbCreateCmd.Flags().StringVar(&createDefaultDB, "default-db", "postgres", "Initial database for user creation")
	dbCreateCmd.Flags().IntVar(&migrationConnLimit, "migration-conn-limit", 10, "Connection limit for migration user")
	dbCreateCmd.Flags().IntVar(&rwConnLimit, "rw-conn-limit", 10, "Connection limit for read-write users")
	dbCreateCmd.Flags().IntVar(&roConnLimit, "ro-conn-limit", 10, "Connection limit for read-only users")
	dbCreateCmd.Flags().BoolVar(&createDryRun, "dry-run", false, "Print SQL statements without executing")
	dbCreateCmd.Flags().BoolVarP(&createForce, "force", "f", false, "Skip existing database/users instead of failing")

	dbCreateCmd.ValidArgsFunction = func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		ctx := context.Background()
		rFlag, _ := c.Flags().GetString("region")

		loadOpts := []func(*config.LoadOptions) error{
			config.WithSharedConfigProfile(awsProfile),
		}
		if rFlag != "" {
			loadOpts = append(loadOpts, config.WithRegion(rFlag))
		} else if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			loadOpts = append(loadOpts, config.WithRegion(envRegion))
		} else {
			loadOpts = append(loadOpts, config.WithRegion(defaultAWSRegion))
		}

		cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		instances, err := core.GetInstancesWithCache(ctx, cfg, awsProfile)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string
		for _, inst := range instances {
			if inst.SourceID != "" {
				continue
			}
			if strings.HasPrefix(inst.ID, toComplete) {
				completions = append(completions, fmt.Sprintf("%s\t%s", inst.ID, inst.Size))
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}

	dbCmd.AddCommand(dbCreateCmd)
}

func runDBCreate(c *cobra.Command, args []string) {
	ctx := c.Context()
	region := resolveRegion(awsRegion)

	var dbName string
	if len(args) > 0 {
		dbName = args[0]
	}

	opts := createdb.Options{
		Profile:            awsProfile,
		Region:             region,
		DBName:             dbName,
		Host:               createHost,
		Port:               createPort,
		Schema:             createSchema,
		DefaultDB:          createDefaultDB,
		MigrationConnLimit: migrationConnLimit,
		RWConnLimit:        rwConnLimit,
		ROConnLimit:        roConnLimit,
		DryRun:             createDryRun,
		Force:              createForce,
		Args:               args,
	}

	if err := createdb.Run(ctx, opts); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
}

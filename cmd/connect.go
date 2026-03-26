package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/PraveenPrabhuT/rds/internal/connect"
	"github.com/PraveenPrabhuT/rds/internal/core"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"
)

var (
	lastConnected bool
	connectHost   string
	connectPort   int
	connectDB     string
	connectURL    string
	showJDBC      bool
	copyJDBC      bool
)

var connectCmd = &cobra.Command{
	Use:   "connect [rds-identifier]",
	Short: "Connect to an RDS PostgreSQL instance",
	Long: `Connect dynamically fetches credentials from AWS Secrets Manager 
and establishes a connection using pgcli, psql, or a native Go fallback.
It requires an active Pritunl VPN connection matching the AWS Profile.

Supports connecting by instance name, RDS host endpoint, or JDBC URL.
Credentials are always resolved from Secrets Manager automatically.`,
	Example: `  # Interactive selection
  rds connect

  # Direct connection with partial name
  rds connect acko-health

  # Reconnect to the last used instance for the current profile
  rds connect -l

  # Connect to a specific database
  rds connect my-instance --db myapp

  # Connect by RDS host endpoint
  rds connect --host my-rds.abc.ap-south-1.rds.amazonaws.com

  # Connect via JDBC URL (DNS alias resolved automatically)
  rds connect --url 'jdbc:postgresql://my-db.internal.example.com/myapp'

  # Get JDBC URL for app config and copy to clipboard
  rds connect my-instance --jdbc --copy`,
	Args: cobra.MaximumNArgs(1),
	Run:  runConnect,
}

func init() {
	connectCmd.Flags().BoolVarP(&lastConnected, "last", "l", false, "Connect to the last used RDS instance")
	connectCmd.Flags().StringVar(&connectHost, "host", "", "RDS host endpoint (bypasses instance picker)")
	connectCmd.Flags().IntVar(&connectPort, "port", 5432, "PostgreSQL port")
	connectCmd.Flags().StringVarP(&connectDB, "db", "d", "postgres", "Database name to connect to")
	connectCmd.Flags().StringVar(&connectURL, "url", "", "JDBC URL to connect (jdbc:postgresql://host[:port][/database])")
	connectCmd.Flags().BoolVar(&showJDBC, "jdbc", false, "Print JDBC URL after resolving credentials")
	connectCmd.Flags().BoolVar(&copyJDBC, "copy", false, "Copy JDBC URL to clipboard (use with --jdbc)")

	connectCmd.ValidArgsFunction = func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		ctx := context.Background()
		rFlag, _ := c.Flags().GetString("region")

		opts := []func(*config.LoadOptions) error{
			config.WithSharedConfigProfile(awsProfile),
		}
		if rFlag != "" {
			opts = append(opts, config.WithRegion(rFlag))
		} else if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			opts = append(opts, config.WithRegion(envRegion))
		} else {
			opts = append(opts, config.WithRegion(defaultAWSRegion))
		}

		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		instances, err := core.GetInstancesWithCache(ctx, cfg, awsProfile)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string
		for _, inst := range instances {
			if strings.HasPrefix(inst.ID, toComplete) {
				completions = append(completions, fmt.Sprintf("%s\t%s", inst.ID, inst.Size))
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}

	rootCmd.AddCommand(connectCmd)
}

const defaultAWSRegion = "ap-south-1"

// resolveRegion returns the effective AWS region: flag > AWS_REGION env > default (ap-south-1).
func resolveRegion(flagRegion string) string {
	if flagRegion != "" {
		return flagRegion
	}
	if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
		return envRegion
	}
	return defaultAWSRegion
}

func runConnect(c *cobra.Command, args []string) {
	ctx := c.Context()

	region := resolveRegion(awsRegion)

	opts := connect.Options{
		Profile:       awsProfile,
		Region:        region,
		LastConnected: lastConnected,
		Host:          connectHost,
		Port:          connectPort,
		DB:            connectDB,
		JDBCURL:       connectURL,
		ShowJDBC:      showJDBC,
		CopyJDBC:      copyJDBC,
		Args:          args,
	}

	if err := connect.Run(ctx, opts); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
}

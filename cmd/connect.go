package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/PraveenPrabhuT/rds/internal/connect"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"
)

var lastConnected bool

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
		}

		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		instances, err := connect.GetInstancesWithCache(ctx, cfg, awsProfile)
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

func runConnect(c *cobra.Command, args []string) {
	ctx := c.Context()

	opts := connect.Options{
		Profile:       awsProfile,
		Region:        awsRegion,
		LastConnected: lastConnected,
		Args:          args,
	}

	if err := connect.Run(ctx, opts); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
}

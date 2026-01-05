package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var awsProfile string
var awsRegion string

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "rds",
	Short:   "A powerful CLI toolkit for AWS RDS management",
	Version: Version, // This enables the 'rds --version' flag automatically
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("rds version %s (commit: %s, built: %s)\n", Version, Commit, Date))

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&awsProfile, "profile", "p", os.Getenv("AWS_PROFILE"), "AWS profile to use")
	rootCmd.PersistentFlags().StringVarP(&awsRegion, "region", "r", "", "AWS Region (overrides config/env)")

	// Dynamic completion for the --profile flag
	rootCmd.RegisterFlagCompletionFunc("profile", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		home, _ := os.UserHomeDir()
		credsPath := filepath.Join(home, ".aws", "credentials")

		data, err := os.ReadFile(credsPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var profiles []string
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				profile := strings.Trim(line, "[]")
				if strings.HasPrefix(profile, toComplete) {
					profiles = append(profiles, profile)
				}
			}
		}
		return profiles, cobra.ShellCompDirectiveNoFileComp
	})
}

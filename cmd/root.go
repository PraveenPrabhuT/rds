package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var awsProfile string

var rootCmd = &cobra.Command{
    Use:   "rds",
    Short: "A powerful CLI toolkit for AWS RDS management",
    Run: func(cmd *cobra.Command, args []string) {
        cmd.Help()
    },
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
}

func init() {
    // Global flag definition
    rootCmd.PersistentFlags().StringVarP(&awsProfile, "profile", "p", os.Getenv("AWS_PROFILE"), "AWS profile to use")
}

package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information of rds",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("rds version: %s\n", Version)
		fmt.Printf("commit:      %s\n", Commit)
		fmt.Printf("built at:    %s\n", Date)
		fmt.Printf("go version:  %s\n", runtime.Version())
		fmt.Printf("os/arch:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

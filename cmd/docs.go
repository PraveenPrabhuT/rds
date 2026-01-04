package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsDir string

var docsCmd = &cobra.Command{
	Use:    "gen-docs",
	Short:  "Generate LLM-ready Markdown documentation for the rds tool",
	Hidden: true, // Keep it out of regular 'help' to avoid clutter
	RunE: func(cmd *cobra.Command, args []string) error {
		if !rootCmd.HasSubCommands() {
			rootCmd.AddCommand(connectCmd)
			// Add other commands like createCmd here as well
		}

		// Ensure the directory exists
		if _, err := os.Stat(docsDir); os.IsNotExist(err) {
			if err := os.MkdirAll(docsDir, 0755); err != nil {
				return fmt.Errorf("failed to create docs directory: %w", err)
			}
		}

		fmt.Printf("📄 Generating LLM-ready docs in: %s\n", docsDir)

		// This generates the Markdown tree
		err := doc.GenMarkdownTree(rootCmd, docsDir)
		if err != nil {
			return fmt.Errorf("failed to generate markdown: %w", err)
		}
		fmt.Printf("Commands found: %v\n", rootCmd.Commands())
		fmt.Println("✅ Documentation successfully generated!")
		return nil
	},
}

func init() {
	docsCmd.Flags().StringVarP(&docsDir, "dir", "d", "./docs/reference", "Directory to save the generated docs")
	rootCmd.AddCommand(docsCmd)
}

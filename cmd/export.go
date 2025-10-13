/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/export"
	"mona-actions/gh-migration-validator/internal/migrationarchive"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export source repository data at a point in time",
	Long: `Export repository data from the source organization at the current point in time.

This command fetches and exports repository metadata including:
- Issues count
- Pull requests count (open, closed, merged)
- Tags count
- Releases count
- Commits count
- Latest commit hash

The data can be exported in JSON or CSV format with a timestamp.

Optionally, you can download and extract migration archives using the --download-archive flag.
The tool will automatically search for migrations containing the specified repository
and allow you to select from multiple matches if available.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get parameters from flags
		sourceOrganization := cmd.Flag("source-organization").Value.String()
		sourceToken := cmd.Flag("source-token").Value.String()
		ghHostname := cmd.Flag("source-hostname").Value.String()
		sourceRepo := cmd.Flag("source-repo").Value.String()
		outputFormat := cmd.Flag("format").Value.String()
		outputFile := cmd.Flag("output").Value.String()
		downloadArchive, _ := cmd.Flags().GetBool("download-archive")

		// Only set ENV variables if flag values are provided (not empty)
		if sourceOrganization != "" {
			os.Setenv("GHMV_SOURCE_ORGANIZATION", sourceOrganization)
		}
		if sourceToken != "" {
			os.Setenv("GHMV_SOURCE_TOKEN", sourceToken)
		}
		if ghHostname != "" {
			os.Setenv("GHMV_SOURCE_HOSTNAME", ghHostname)
		}
		if sourceRepo != "" {
			os.Setenv("GHMV_SOURCE_REPO", sourceRepo)
		}

		// Bind ENV variables in Viper
		viper.BindEnv("SOURCE_ORGANIZATION")
		viper.BindEnv("SOURCE_TOKEN")
		viper.BindEnv("SOURCE_HOSTNAME")
		viper.BindEnv("SOURCE_PRIVATE_KEY")
		viper.BindEnv("SOURCE_APP_ID")
		viper.BindEnv("SOURCE_INSTALLATION_ID")
		viper.BindEnv("SOURCE_REPO")

		// Validate required variables for export
		if err := checkExportVars(); err != nil {
			fmt.Printf("Export configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		initializeAPI()

		// Create validator and export source data
		migrationValidator := validator.New(ghAPI)

		// Export the source repository data
		timestamp := time.Now()
		err := export.ExportSourceData(migrationValidator, sourceOrganization, sourceRepo, outputFormat, outputFile, timestamp)
		if err != nil {
			fmt.Printf("Export failed: %v\n", err)
			os.Exit(1)
		}

		// Handle migration archive download if requested
		if downloadArchive {
			fmt.Println("Searching for migration archives...")
			err := migrationarchive.DownloadAndExtractArchive(ghAPI, sourceOrganization, sourceRepo)
			if err != nil {
				fmt.Printf("Migration archive download failed: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	// Add export command to root
	rootCmd.AddCommand(exportCmd)

	// Define flags specific to export command
	exportCmd.Flags().StringP("source-organization", "s", "", "Source Organization to export data from")
	exportCmd.MarkFlagRequired("source-organization")

	exportCmd.Flags().StringP("source-token", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")

	exportCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	exportCmd.Flags().StringP("source-repo", "", "", "Source repository name to export (just the repo name, not owner/repo)")
	exportCmd.MarkFlagRequired("source-repo")

	exportCmd.Flags().StringP("format", "f", "json", "Output format: json or csv")

	exportCmd.Flags().StringP("output", "o", "", "Output file path (if not provided, will use default naming)")

	exportCmd.Flags().BoolP("download-archive", "d", false, "Download and extract migration archive for the specified repository")
}

// checkExportVars validates the configuration for export command
func checkExportVars() error {
	// Check for source token
	sourceToken := viper.GetString("SOURCE_TOKEN")
	if sourceToken == "" {
		return fmt.Errorf("source token is required. Set it via --source-token flag or GHMV_SOURCE_TOKEN environment variable")
	}

	// Check source repository
	sourceRepo := viper.GetString("SOURCE_REPO")
	if sourceRepo == "" {
		return fmt.Errorf("source repository is required. Set it via --source-repo flag")
	}

	return nil
}

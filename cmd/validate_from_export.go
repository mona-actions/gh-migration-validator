/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/export"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// validateFromExportCmd represents the validate-from-export command
var validateFromExportCmd = &cobra.Command{
	Use:   "validate-from-export",
	Short: "Validate target repository against exported source data",
	Long: `Validate a target repository against previously exported source repository data.

This command allows you to validate a migration by comparing the target repository 
against a point-in-time snapshot of the source repository that was previously 
exported using the 'export' command.

This is useful for:
- Validating migrations against an active repository that may have changed since migration
- Comparing target repositories to source state at migration time
- Ensuring migration integrity when source data may have changed

The validation compares the same metrics as the standard validate command:
- Issues count
- Pull requests count (open, closed, merged)
- Tags count  
- Releases count
- Commits count
- Latest commit hash`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get parameters from flags
		exportFile := cmd.Flag("export-file").Value.String()
		targetOrganization := cmd.Flag("github-target-org").Value.String()
		targetToken := cmd.Flag("github-target-pat").Value.String()
		targetHostname := cmd.Flag("target-hostname").Value.String()
		targetRepo := cmd.Flag("target-repo").Value.String()
		markdownTable, err := cmd.Flags().GetBool("markdown-table")
		if err != nil {
			fmt.Printf("Failed to parse 'markdown-table' flag: %v\n", err)
			os.Exit(1)
		}
		markdownFile, err := cmd.Flags().GetString("markdown-file")
		if err != nil {
			fmt.Printf("Failed to parse 'markdown-file' flag: %v\n", err)
			os.Exit(1)
		}

		// Only set ENV variables if flag values are provided (not empty)
		if targetToken != "" {
			os.Setenv("GHMV_TARGET_TOKEN", targetToken)
		}
		if targetHostname != "" {
			os.Setenv("GHMV_TARGET_HOSTNAME", targetHostname)
		}
		if markdownTable {
			os.Setenv("GHMV_MARKDOWN_TABLE", "true")
		}
		if markdownFile != "" {
			os.Setenv("GHMV_MARKDOWN_FILE", markdownFile)
		}
		noLFS, _ := cmd.Flags().GetBool("no-lfs")
		if noLFS {
			os.Setenv("GHMV_NO_LFS", "true")
		}

		// Bind ENV variables in Viper (for optional parameters that can use env vars)
		viper.BindEnv("TARGET_TOKEN")
		viper.BindEnv("TARGET_HOSTNAME")
		viper.BindEnv("TARGET_PRIVATE_KEY")
		viper.BindEnv("TARGET_APP_ID")
		viper.BindEnv("TARGET_INSTALLATION_ID")
		viper.BindEnv("MARKDOWN_TABLE")
		viper.BindEnv("MARKDOWN_FILE")
		viper.BindEnv("NO_LFS")

		// Validate required parameters (using flag values directly for required flags)
		if err := checkExportValidationVars(exportFile); err != nil {
			fmt.Printf("Export validation configuration failed: %v\n", err)
			os.Exit(1)
		}

		// Load export data from file
		exportData, err := export.LoadExportData(exportFile)
		if err != nil {
			fmt.Printf("Failed to load export file: %v\n", err)
			os.Exit(1)
		}

		// Initialize API with target-only clients
		ghAPI, err := api.NewTargetOnlyAPI()
		if err != nil {
			fmt.Printf("Failed to initialize target API: %v\n", err)
			os.Exit(1)
		}

		// Create validator and perform validation
		migrationValidator := validator.New(ghAPI)

		// Set source data from export instead of fetching from API
		// Copy migration archive data to repository data if it exists
		repositoryData := exportData.Repository
		repositoryData.MigrationArchive = exportData.MigrationArchive
		migrationValidator.SetSourceDataFromExport(&repositoryData)

		// Perform validation against target (now returns results directly)
		results, err := migrationValidator.ValidateFromExport(targetOrganization, targetRepo)
		if err != nil {
			fmt.Printf("Validation failed: %v\n", err)
			os.Exit(1)
		}

		// Display results using existing method
		migrationValidator.PrintValidationResults(results)
	},
}

func init() {
	// Add validate-from-export command to root
	rootCmd.AddCommand(validateFromExportCmd)

	// Define flags specific to validate-from-export command
	validateFromExportCmd.Flags().StringP("export-file", "e", "", "Path to the exported JSON file to use as source data")
	validateFromExportCmd.MarkFlagRequired("export-file")

	validateFromExportCmd.Flags().StringP("github-target-org", "t", "", "Target Organization to validate against")
	validateFromExportCmd.MarkFlagRequired("github-target-org")

	validateFromExportCmd.Flags().StringP("github-target-pat", "b", "", "Target Organization GitHub token. Scopes: read:org, read:user, user:email")

	validateFromExportCmd.Flags().StringP("target-hostname", "v", "", "GitHub Enterprise target hostname url (optional) Ex. https://github.example.com")

	validateFromExportCmd.Flags().String("target-repo", "", "Target repository name to validate (just the repo name, not owner/repo)")
	validateFromExportCmd.MarkFlagRequired("target-repo")

	validateFromExportCmd.Flags().BoolP("markdown-table", "m", false, "Output results in markdown table format")
	validateFromExportCmd.Flags().String("markdown-file", "", "Write markdown output to the specified file (optional)")
	validateFromExportCmd.Flags().Bool("no-lfs", false, "Skip LFS object validation")
}

// checkExportValidationVars validates the configuration for validate-from-export command
func checkExportValidationVars(exportFile string) error {
	// Check export file is provided
	if exportFile == "" {
		return fmt.Errorf("export file is required. Set it via --export-file flag")
	}

	// Check if export file exists
	if _, err := os.Stat(exportFile); os.IsNotExist(err) {
		return fmt.Errorf("export file does not exist: %s", exportFile)
	}

	// Check for target token (can come from flag or environment variable)
	targetToken := viper.GetString("TARGET_TOKEN")
	if targetToken == "" {
		return fmt.Errorf("target token is required. Set it via --github-target-pat flag or GHMV_TARGET_TOKEN environment variable")
	}

	return nil
}

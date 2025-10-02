/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
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
		targetOrganization := cmd.Flag("target-organization").Value.String()
		targetToken := cmd.Flag("target-token").Value.String()
		targetHostname := cmd.Flag("target-hostname").Value.String()
		targetRepo := cmd.Flag("target-repo").Value.String()
		markdownTable := cmd.Flag("markdown-table").Value.String() == "true"

		// Only set ENV variables if flag values are provided (not empty)
		if targetOrganization != "" {
			os.Setenv("GHMV_TARGET_ORGANIZATION", targetOrganization)
		}
		if targetToken != "" {
			os.Setenv("GHMV_TARGET_TOKEN", targetToken)
		}
		if targetHostname != "" {
			os.Setenv("GHMV_TARGET_HOSTNAME", targetHostname)
		}
		if targetRepo != "" {
			os.Setenv("GHMV_TARGET_REPO", targetRepo)
		}
		if markdownTable {
			os.Setenv("GHMV_MARKDOWN_TABLE", "true")
		}

		// Bind ENV variables in Viper
		viper.BindEnv("TARGET_ORGANIZATION")
		viper.BindEnv("TARGET_TOKEN")
		viper.BindEnv("TARGET_HOSTNAME")
		viper.BindEnv("TARGET_PRIVATE_KEY")
		viper.BindEnv("TARGET_APP_ID")
		viper.BindEnv("TARGET_INSTALLATION_ID")
		viper.BindEnv("TARGET_REPO")
		viper.BindEnv("MARKDOWN_TABLE")

		// Validate required variables for export validation
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

		// Initialize API for target
		initializeAPI()

		// Create validator and perform validation
		migrationValidator := validator.New(ghAPI)

		// Set source data from export instead of fetching from API
		migrationValidator.SetSourceDataFromExport(&exportData.Repository)

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

	validateFromExportCmd.Flags().StringP("target-organization", "t", "", "Target Organization to validate against")
	validateFromExportCmd.MarkFlagRequired("target-organization")

	validateFromExportCmd.Flags().StringP("target-token", "b", "", "Target Organization GitHub token. Scopes: read:org, read:user, user:email")

	validateFromExportCmd.Flags().StringP("target-hostname", "v", "", "GitHub Enterprise target hostname url (optional) Ex. https://github.example.com")

	validateFromExportCmd.Flags().StringP("target-repo", "", "", "Target repository name to validate (just the repo name, not owner/repo)")
	validateFromExportCmd.MarkFlagRequired("target-repo")

	validateFromExportCmd.Flags().BoolP("markdown-table", "m", false, "Output results in markdown table format")
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

	// Check for target token
	targetToken := viper.GetString("TARGET_TOKEN")
	if targetToken == "" {
		return fmt.Errorf("target token is required. Set it via --target-token flag or GHMV_TARGET_TOKEN environment variable")
	}

	// Check target repository
	targetRepo := viper.GetString("TARGET_REPO")
	if targetRepo == "" {
		return fmt.Errorf("target repository is required. Set it via --target-repo flag")
	}

	return nil
}

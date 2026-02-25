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
		// export-file is a local flag, read directly from cobra
		exportFile := cmd.Flag("export-file").Value.String()

		// Validate required parameters
		if err := checkExportValidationVars(exportFile); err != nil {
			fmt.Printf("Export validation configuration failed: %v\n", err)
			os.Exit(1)
		}

		// Read all values from Viper (single source of truth)
		// Shared target flags are inherited via PersistentFlags + BindPFlag from root
		targetOrganization := viper.GetString("TARGET_ORGANIZATION")
		targetRepo := viper.GetString("TARGET_REPO")

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

		if viper.GetBool("STRICT_EXIT") && validator.HasFailures(results) {
			os.Exit(2)
		}
	},
}

func init() {
	// Add validate-from-export command to root
	rootCmd.AddCommand(validateFromExportCmd)

	// Define flags specific to validate-from-export command only —
	// shared target flags are inherited from rootCmd PersistentFlags
	validateFromExportCmd.Flags().StringP("export-file", "e", "", "Path to the exported JSON file to use as source data")
	validateFromExportCmd.MarkFlagRequired("export-file")
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

	// Check required viper-managed configurations
	required := map[string]requiredConfig{
		"TARGET_TOKEN":        {"--github-target-pat / -b", "GHMV_TARGET_TOKEN"},
		"TARGET_ORGANIZATION": {"--github-target-org / -t", "GHMV_TARGET_ORGANIZATION"},
		"TARGET_REPO":         {"--target-repo", "GHMV_TARGET_REPO"},
	}

	for key, info := range required {
		if viper.GetString(key) == "" {
			return fmt.Errorf("%s is required. Set via %s flag or %s environment variable",
				key, info.flag, info.envVar)
		}
	}

	return nil
}

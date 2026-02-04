/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-migration-validator",
	Short: "Validate GitHub organization migrations",
	Long: `A GitHub CLI extension for validating GitHub organization migrations.

This tool helps ensure that your migration from one GitHub organization to another
has been completed successfully by comparing certain repositories resources
between source and target organizations.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required variables (from either flags OR env vars)
		if err := checkVars(); err != nil {
			fmt.Printf("Configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Read all values from Viper (single source of truth)
		sourceOrganization := viper.GetString("SOURCE_ORGANIZATION")
		targetOrganization := viper.GetString("TARGET_ORGANIZATION")
		sourceRepo := viper.GetString("SOURCE_REPO")
		targetRepo := viper.GetString("TARGET_REPO")

		// Initialize API with both source and target clients
		ghAPI, err := api.NewGitHubAPI()
		if err != nil {
			fmt.Printf("Failed to initialize API clients: %v\n", err)
			os.Exit(1)
		}

		// Create validator and run migration validation
		migrationValidator := validator.New(ghAPI)
		results, err := migrationValidator.ValidateMigration(sourceOrganization, sourceRepo, targetOrganization, targetRepo)
		if err != nil {
			fmt.Printf("Migration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Print the validation results - always report what we found
		migrationValidator.PrintValidationResults(results)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Define flags WITHOUT marking as required - validation happens in checkVars()
	// This allows either flags OR environment variables to provide values
	rootCmd.Flags().StringP("github-source-org", "s", "", "Source Organization to sync teams from")
	rootCmd.Flags().StringP("github-target-org", "t", "", "Target Organization to sync teams from")
	rootCmd.Flags().StringP("github-source-pat", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")
	rootCmd.Flags().StringP("github-target-pat", "b", "", "Target Organization GitHub token. Scopes: admin:org")
	rootCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")
	rootCmd.Flags().StringP("source-repo", "", "", "Source repository name to verify against (just the repo name, not owner/repo)")
	rootCmd.Flags().StringP("target-repo", "", "", "Target repository name to verify against (just the repo name, not owner/repo)")
	rootCmd.Flags().BoolP("markdown-table", "m", false, "Print results as a markdown table")
	rootCmd.Flags().String("markdown-file", "", "Write markdown output to the specified file (optional)")
	rootCmd.Flags().Bool("no-lfs", false, "Skip LFS object validation")

	// Set environment variable prefix: GHMV (GitHub Migration Validator)
	viper.SetEnvPrefix("GHMV")
	viper.AutomaticEnv()

	// Bind flags to Viper keys - this connects flags directly to Viper
	// Priority: Flag value > Environment variable > Default value
	viper.BindPFlag("SOURCE_ORGANIZATION", rootCmd.Flags().Lookup("github-source-org"))
	viper.BindPFlag("TARGET_ORGANIZATION", rootCmd.Flags().Lookup("github-target-org"))
	viper.BindPFlag("SOURCE_TOKEN", rootCmd.Flags().Lookup("github-source-pat"))
	viper.BindPFlag("TARGET_TOKEN", rootCmd.Flags().Lookup("github-target-pat"))
	viper.BindPFlag("SOURCE_HOSTNAME", rootCmd.Flags().Lookup("source-hostname"))
	viper.BindPFlag("SOURCE_REPO", rootCmd.Flags().Lookup("source-repo"))
	viper.BindPFlag("TARGET_REPO", rootCmd.Flags().Lookup("target-repo"))
	viper.BindPFlag("MARKDOWN_TABLE", rootCmd.Flags().Lookup("markdown-table"))
	viper.BindPFlag("MARKDOWN_FILE", rootCmd.Flags().Lookup("markdown-file"))
	viper.BindPFlag("NO_LFS", rootCmd.Flags().Lookup("no-lfs"))

	// Bind environment variables explicitly for additional app authentication options
	viper.BindEnv("SOURCE_PRIVATE_KEY")
	viper.BindEnv("SOURCE_APP_ID")
	viper.BindEnv("SOURCE_INSTALLATION_ID")
	viper.BindEnv("TARGET_PRIVATE_KEY")
	viper.BindEnv("TARGET_APP_ID")
	viper.BindEnv("TARGET_INSTALLATION_ID")
	viper.BindEnv("MARKDOWN_FILE")
}

// requiredConfig defines a required configuration with its flag and env var names
type requiredConfig struct {
	flag   string
	envVar string
}

func checkVars() error {
	// Required configurations with helpful error messages
	required := map[string]requiredConfig{
		"SOURCE_ORGANIZATION": {"--github-source-org / -s", "GHMV_SOURCE_ORGANIZATION"},
		"TARGET_ORGANIZATION": {"--github-target-org / -t", "GHMV_TARGET_ORGANIZATION"},
		"SOURCE_TOKEN":        {"--github-source-pat / -a", "GHMV_SOURCE_TOKEN"},
		"TARGET_TOKEN":        {"--github-target-pat / -b", "GHMV_TARGET_TOKEN"},
		"SOURCE_REPO":         {"--source-repo", "GHMV_SOURCE_REPO"},
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

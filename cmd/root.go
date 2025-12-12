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
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		// Get parameters from flags
		sourceOrganization := cmd.Flag("github-source-org").Value.String()
		targetOrganization := cmd.Flag("github-target-org").Value.String()
		sourceToken := cmd.Flag("github-source-pat").Value.String()
		targetToken := cmd.Flag("github-target-pat").Value.String()
		ghHostname := cmd.Flag("source-hostname").Value.String()
		sourceRepo := cmd.Flag("source-repo").Value.String()
		targetRepo := cmd.Flag("target-repo").Value.String()
		markdownTable := cmd.Flag("markdown-table").Value.String()

		// Only set ENV variables if flag values are provided (not empty)
		if sourceOrganization != "" {
			os.Setenv("GHMV_SOURCE_ORGANIZATION", sourceOrganization)
		}
		if targetOrganization != "" {
			os.Setenv("GHMV_TARGET_ORGANIZATION", targetOrganization)
		}
		if sourceToken != "" {
			os.Setenv("GHMV_SOURCE_TOKEN", sourceToken)
		}
		if targetToken != "" {
			os.Setenv("GHMV_TARGET_TOKEN", targetToken)
		}
		if ghHostname != "" {
			os.Setenv("GHMV_SOURCE_HOSTNAME", ghHostname)
		}
		if sourceRepo != "" {
			os.Setenv("GHMV_SOURCE_REPO", sourceRepo)
		}
		if targetRepo != "" {
			os.Setenv("GHMV_TARGET_REPO", targetRepo)
		}
		if markdownTable != "" {
			os.Setenv("GHMV_MARKDOWN_TABLE", markdownTable)
		}

		// Bind ENV variables in Viper
		viper.BindEnv("SOURCE_ORGANIZATION")
		viper.BindEnv("TARGET_ORGANIZATION")
		viper.BindEnv("SOURCE_TOKEN")
		viper.BindEnv("TARGET_TOKEN")
		viper.BindEnv("SOURCE_HOSTNAME")
		viper.BindEnv("SOURCE_PRIVATE_KEY")
		viper.BindEnv("SOURCE_APP_ID")
		viper.BindEnv("SOURCE_INSTALLATION_ID")
		viper.BindEnv("TARGET_PRIVATE_KEY")
		viper.BindEnv("TARGET_APP_ID")
		viper.BindEnv("TARGET_INSTALLATION_ID")
		viper.BindEnv("SOURCE_REPO")
		viper.BindEnv("TARGET_REPO")
		viper.BindEnv("MARKDOWN_TABLE")

		// Validate required variables and configuration
		if err := checkVars(); err != nil {
			fmt.Printf("Configuration validation failed: %v\n", err)
			os.Exit(1)
		}

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
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Cobra also supports local flags, which will only run
	// when this action is called directly.

	rootCmd.Flags().StringP("github-source-org", "s", "", "Source Organization to sync teams from")
	rootCmd.MarkFlagRequired("github-source-org")

	rootCmd.Flags().StringP("github-target-org", "t", "", "Target Organization to sync teams from")
	rootCmd.MarkFlagRequired("github-target-org")

	rootCmd.Flags().StringP("github-source-pat", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")
	//rootCmd.MarkFlagRequired("github-source-pat")

	rootCmd.Flags().StringP("github-target-pat", "b", "", "Target Organization GitHub token. Scopes: admin:org")
	//rootCmd.MarkFlagRequired("github-target-pat")

	rootCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	rootCmd.Flags().StringP("source-repo", "", "", "Source repository name to verify against (just the repo name, not owner/repo)")

	rootCmd.Flags().StringP("target-repo", "", "", "Target repository name to verify against (just the repo name, not owner/repo)")

	//boolean flag for printing the markdown table
	rootCmd.Flags().BoolP("markdown-table", "m", false, "Print results as a markdown table")

	viper.SetEnvPrefix("GHMV") // Set the environment variable prefix, GHMV (GitHub Migration Validator)

	// Read in environment variables that match
	viper.AutomaticEnv()
}

func checkVars() error {
	// Check for tokens - they can be provided via flags or environment variables
	sourceToken := viper.GetString("SOURCE_TOKEN")
	targetToken := viper.GetString("TARGET_TOKEN")

	if sourceToken == "" {
		return fmt.Errorf("source token is required. Set it via --github-source-pat flag or GHMV_SOURCE_TOKEN environment variable")
	}

	if targetToken == "" {
		return fmt.Errorf("target token is required. Set it via --github-target-pat flag or GHMV_TARGET_TOKEN environment variable")
	}

	// Check repository configuration
	sourceRepo := viper.GetString("SOURCE_REPO")
	targetRepo := viper.GetString("TARGET_REPO")

	// We need both source and target repositories
	if sourceRepo == "" {
		return fmt.Errorf("source repository is required. Set it via --source-repo flag")
	}

	if targetRepo == "" {
		return fmt.Errorf("target repository is required. Set it via --target-repo flag")
	}

	return nil
}

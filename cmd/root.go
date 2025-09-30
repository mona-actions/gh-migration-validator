/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ghAPI *api.GitHubAPI

func initializeAPI() {
	ghAPI = api.GetAPI()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gh-migration-validator",
	Short: "Validate GitHub organization migrations",
	Long: `A GitHub CLI extension for validating GitHub organization migrations.

This tool helps ensure that your migration from one GitHub organization to another
has been completed successfully by comparing certain repositories resources
between source and target organizations.

Examples:
  # Single repository validation
  gh migration-validator --source-org myorg --target-org neworg --source-repo repo1 --target-repo repo1-migrated

  # Multiple repositories from CSV file
  gh migration-validator --source-org myorg --target-org neworg --repo-list repos.txt`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		// Get parameters from flags
		sourceOrganization := cmd.Flag("source-organization").Value.String()
		targetOrganization := cmd.Flag("target-organization").Value.String()
		sourceToken := cmd.Flag("source-token").Value.String()
		targetToken := cmd.Flag("target-token").Value.String()
		ghHostname := cmd.Flag("source-hostname").Value.String()
		sourceRepo := cmd.Flag("source-repo").Value.String()
		targetRepo := cmd.Flag("target-repo").Value.String()
		repoList := cmd.Flag("repo-list").Value.String()
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
		if repoList != "" {
			os.Setenv("GHMV_REPO_LIST", repoList)
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
		viper.BindEnv("REPO_LIST")
		viper.BindEnv("MARKDOWN_TABLE")

		// Validate required variables and configuration
		if err := checkVars(); err != nil {
			fmt.Printf("Configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		initializeAPI()

		// Create validator
		migrationValidator := validator.New(ghAPI)

		// Determine if this is single repo or batch mode
		repoListFile := viper.GetString("REPO_LIST")

		if repoListFile != "" {
			// BATCH MODE: Multiple repositories from CSV file
			if err := runBatchValidation(migrationValidator, sourceOrganization, targetOrganization, repoListFile); err != nil {
				pterm.Error.Printf("Batch validation failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			// SINGLE REPO MODE: Individual repository validation
			sourceRepo := viper.GetString("SOURCE_REPO")
			targetRepo := viper.GetString("TARGET_REPO")

			if err := runSingleValidation(migrationValidator, sourceOrganization, sourceRepo, targetOrganization, targetRepo); err != nil {
				pterm.Error.Printf("Single validation failed: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// runBatchValidation handles validation of multiple repositories from CSV file
func runBatchValidation(mv *validator.MigrationValidator, sourceOrg, targetOrg, repoListFile string) error {
	pterm.Info.Printf("Starting batch validation from: %s\n", repoListFile)

	// Parse the repository list from CSV
	pairs, err := mv.ParseRepositoryList(repoListFile)
	if err != nil {
		return fmt.Errorf("failed to parse repository list: %w", err)
	}

	pterm.Info.Printf("Found %d repository pairs to validate\n", len(pairs))

	// Validate all repositories in batch
	batchResult, err := mv.ValidateBatch(sourceOrg, targetOrg, pairs)
	if err != nil {
		return fmt.Errorf("batch validation failed: %w", err)
	}

	// Print batch results using executive summary format (Option 4 + Option 2 style)
	mv.PrintBatchResults(batchResult)

	// Handle session management - save if issues found or in CI
	handleSessionManagement(mv)

	// Return error if there were failures (for CI/automation)
	if batchResult.Summary.Failed > 0 {
		return fmt.Errorf("batch validation completed with %d failures", batchResult.Summary.Failed)
	}

	return nil
}

// runSingleValidation handles validation of a single repository pair
func runSingleValidation(mv *validator.MigrationValidator, sourceOrg, sourceRepo, targetOrg, targetRepo string) error {
	pterm.Info.Printf("Starting single repository validation: %s/%s → %s/%s\n", sourceOrg, sourceRepo, targetOrg, targetRepo)

	// Run single repository validation
	results, err := mv.ValidateMigration(sourceOrg, sourceRepo, targetOrg, targetRepo)
	if err != nil {
		return fmt.Errorf("migration validation failed: %w", err)
	}

	// Print single repository results using current detailed format
	mv.PrintValidationResults(results)

	// Use shared function to determine status and return error for CI/automation
	status, _ := validator.DetermineRepositoryStatus(results)
	if status == "❌ FAIL" {
		return fmt.Errorf("validation failed - some data is missing in target repository")
	}

	return nil
}

// handleSessionManagement implements smart session saving logic
func handleSessionManagement(mv *validator.MigrationValidator) {
	// Only attempt session management for batch results
	if mv.BatchResult == nil {
		return
	}

	result := mv.BatchResult
	shouldSave := false

	// Save if there are failures or warnings
	if result.Summary.Failed > 0 || result.Summary.Warnings > 0 {
		shouldSave = true
	}

	// Always save in CI or non-interactive environments
	if isNonInteractive() {
		shouldSave = true
	}

	if shouldSave {
		sessionPath, err := mv.SaveSession()
		if err != nil {
			pterm.Warning.Printf("Failed to save session: %v\n", err)
			return
		}

		pterm.Success.Printf("💾 Session saved: %s\n", sessionPath)

		// Show helpful hints if there are issues to investigate
		if result.Summary.Failed > 0 || result.Summary.Warnings > 0 {
			pterm.Info.Println()
			pterm.Info.Printf("💡 To investigate specific repositories, use:\n")
			pterm.Info.Printf("   gh migration-validator inspect <repository-name>\n")
		}
	}
}

// isNonInteractive checks if we're running in a non-interactive environment
func isNonInteractive() bool {
	// Check for common CI environment variables
	ciEnvVars := []string{"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "JENKINS_URL", "BUILD_NUMBER"}
	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	// Check if stdout is not a terminal
	stat, err := os.Stdout.Stat()
	if err != nil {
		return true
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
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

	rootCmd.Flags().StringP("source-organization", "s", "", "Source Organization to sync teams from")
	rootCmd.MarkFlagRequired("source-organization")

	rootCmd.Flags().StringP("target-organization", "t", "", "Target Organization to sync teams from")
	rootCmd.MarkFlagRequired("target-organization")

	rootCmd.Flags().StringP("source-token", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")

	rootCmd.Flags().StringP("target-token", "b", "", "Target Organization GitHub token. Scopes: admin:org")

	rootCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	rootCmd.Flags().StringP("source-repo", "", "", "Source repository name to verify against (just the repo name, not owner/repo)")

	rootCmd.Flags().StringP("target-repo", "", "", "Target repository name to verify against (just the repo name, not owner/repo)")

	rootCmd.Flags().StringP("repo-list", "l", "", "Path to CSV file containing repository pairs (format: source,target)")

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
		return fmt.Errorf("source token is required. Set it via --source-token flag or GHMV_SOURCE_TOKEN environment variable")
	}

	if targetToken == "" {
		return fmt.Errorf("target token is required. Set it via --target-token flag or GHMV_TARGET_TOKEN environment variable")
	}

	// Check repository configuration
	sourceRepo := viper.GetString("SOURCE_REPO")
	targetRepo := viper.GetString("TARGET_REPO")
	repoListFile := viper.GetString("REPO_LIST")

	// If repo list file is provided, we don't need individual source/target repos
	if repoListFile != "" {
		// Validate that the repo list file exists
		if _, err := os.Stat(repoListFile); os.IsNotExist(err) {
			return fmt.Errorf("repo list file does not exist: %s", repoListFile)
		}
		return nil
	}

	// We need both source and target repositories for single mode
	if sourceRepo == "" {
		return fmt.Errorf("source repository is required. Set it via --source-repo flag or provide --repo-list")
	}

	if targetRepo == "" {
		return fmt.Errorf("target repository is required. Set it via --target-repo flag or provide --repo-list")
	}

	return nil
}

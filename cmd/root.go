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
between source and target organizations.`,
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
		// repoList := cmd.Flag("repo-list").Value.String()
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
		// if repoList != "" {
		// 	os.Setenv("GHMV_REPO_LIST", repoList)
		// }
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
		// viper.BindEnv("REPO_LIST")
		viper.BindEnv("MARKDOWN_TABLE")

		// Validate required variables and configuration
		if err := checkVars(); err != nil {
			fmt.Printf("Configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		initializeAPI()

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

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gh-migration-validator.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.

	rootCmd.Flags().StringP("source-organization", "s", "", "Source Organization to sync teams from")
	rootCmd.MarkFlagRequired("source-organization")

	rootCmd.Flags().StringP("target-organization", "t", "", "Target Organization to sync teams from")
	rootCmd.MarkFlagRequired("target-organization")

	rootCmd.Flags().StringP("source-token", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")
	//rootCmd.MarkFlagRequired("source-token")

	rootCmd.Flags().StringP("target-token", "b", "", "Target Organization GitHub token. Scopes: admin:org")
	//rootCmd.MarkFlagRequired("target-token")

	rootCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	rootCmd.Flags().StringP("source-repo", "", "", "Source repository name to verify against (just the repo name, not owner/repo)")

	rootCmd.Flags().StringP("target-repo", "", "", "Target repository name to verify against (just the repo name, not owner/repo)")

	// rootCmd.Flags().StringP("repo-list", "l", "", "Path to a file containing a list of repositories to validate (one per line)")

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
	// repoListFile := viper.GetString("REPO_LIST")

	// If repo list file is provided, we don't need individual source/target repos
	// if repoListFile != "" {
	// 	// Validate that the repo list file exists
	// 	if _, err := os.Stat(repoListFile); os.IsNotExist(err) {
	// 		return fmt.Errorf("repo list file does not exist: %s", repoListFile)
	// 	}
	// 	return nil
	// }

	// We need both source and target repositories
	if sourceRepo == "" {
		return fmt.Errorf("source repository is required. Set it via --source-repo flag")
	}

	if targetRepo == "" {
		return fmt.Errorf("target repository is required. Set it via --target-repo flag")
	}

	return nil
}

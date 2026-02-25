/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/api/bitbucket"
	"mona-actions/gh-migration-validator/internal/output"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// bbsCmd represents the bitbucket command
var bbsCmd = &cobra.Command{
	Use:   "bitbucket",
	Short: "Validate Bitbucket to GitHub migration",
	Long: `Validate a migration from Bitbucket (Server / Data Center) to GitHub by comparing 
repository metrics between the source Bitbucket repository and the target GitHub repository.

This command compares the following metrics:
- Pull Requests (Total, Open, Merged, Declined→Closed)
- Tags count
- Commits count on default branch
- Latest commit SHA
- Branch Permissions (advisory comparison with Branch Protection Rules)
- Webhooks count

Metrics not available in Bitbucket (Issues, Releases, LFS) are automatically skipped.`,
	// PreRun binds BBS-specific flags to Viper at execution time.
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("BBS_SERVER_URL", cmd.Flags().Lookup("bbs-server-url"))
		viper.BindPFlag("BBS_PROJECT", cmd.Flags().Lookup("bbs-project"))
		viper.BindPFlag("BBS_REPO", cmd.Flags().Lookup("bbs-repo"))
		viper.BindPFlag("BBS_TOKEN", cmd.Flags().Lookup("bbs-token"))
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required variables (from either flags OR env vars)
		if err := checkBBSVars(); err != nil {
			fmt.Printf("Bitbucket configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Read all values from Viper (single source of truth)
		bbsHostname := viper.GetString("BBS_SERVER_URL")
		bbsProjectKey := viper.GetString("BBS_PROJECT")
		bbsRepoSlug := viper.GetString("BBS_REPO")
		bbsToken := viper.GetString("BBS_TOKEN")
		targetOrganization := viper.GetString("TARGET_ORGANIZATION")
		targetRepo := viper.GetString("TARGET_REPO")

		// Create BBS client
		bbsClient, err := bitbucket.NewBBSClient(bbsHostname, bbsToken)
		if err != nil {
			fmt.Printf("Failed to initialize Bitbucket client: %v\n", err)
			os.Exit(1)
		}

		// Validate BBS repository access
		fmt.Println("Validating Bitbucket repository access...")
		if err := bbsClient.ValidateRepoAccess(bbsProjectKey, bbsRepoSlug); err != nil {
			fmt.Printf("Bitbucket repository access failed: %v\n", err)
			os.Exit(1)
		}

		// Initialize GitHub target API
		ghAPI, err := api.NewTargetOnlyAPI()
		if err != nil {
			fmt.Printf("Failed to initialize target API: %v\n", err)
			os.Exit(1)
		}

		// Retrieve BBS repository metrics
		spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching data from Bitbucket %s/%s...", bbsProjectKey, bbsRepoSlug))
		bbsData, errorMsgs, err := bbsClient.GetRepositoryMetrics(bbsProjectKey, bbsRepoSlug, spinner)

		// Log any API errors
		output.LogAPIErrors(errorMsgs, bbsProjectKey, bbsRepoSlug, err)

		if err != nil {
			fmt.Printf("Failed to retrieve Bitbucket data: %v\n", err)
			os.Exit(1)
		}

		// Create validator and set source data from BBS
		migrationValidator := validator.New(ghAPI)
		migrationValidator.SetSourceData(bbsData)

		// Perform validation with BBS-specific options
		results, err := migrationValidator.ValidateWithOptions(targetOrganization, targetRepo, validator.ValidationOptions{
			SkipIssues:                true,
			SkipReleases:              true,
			SkipLFS:                   true,
			SkipMigrationLogOffset:    true,
			SkipMigrationArchive:      true,
			BranchPermissionsAdvisory: true,
			SourceLabel:               "Bitbucket",
		})
		if err != nil {
			fmt.Printf("Validation failed: %v\n", err)
			os.Exit(1)
		}

		// Display results
		migrationValidator.PrintValidationResults(results)

		if viper.GetBool("STRICT_EXIT") && validator.HasFailures(results) {
			os.Exit(2)
		}
	},
}

func init() {
	// Add bitbucket command to root
	rootCmd.AddCommand(bbsCmd)

	// Define BBS-specific flags only — shared target flags are inherited
	// from rootCmd PersistentFlags (github-target-org, github-target-pat,
	// target-hostname, target-repo, markdown-table, markdown-file, no-lfs)
	// Flag names aligned with GEI bbs2gh CLI (--bbs-server-url, --bbs-project, --bbs-repo)
	bbsCmd.Flags().StringP("bbs-server-url", "H", "", "Bitbucket Server URL (e.g., https://bitbucket.example.com)")
	bbsCmd.Flags().StringP("bbs-project", "p", "", "Bitbucket project key (use ~username for personal repos)")
	bbsCmd.Flags().StringP("bbs-repo", "r", "", "Bitbucket repository slug")
	bbsCmd.Flags().StringP("bbs-token", "k", "", "Bitbucket personal access token")
}

// checkBBSVars validates the configuration for the bitbucket command
func checkBBSVars() error {
	required := map[string]requiredConfig{
		"BBS_SERVER_URL":      {"--bbs-server-url / -H", "GHMV_BBS_SERVER_URL"},
		"BBS_PROJECT":         {"--bbs-project / -p", "GHMV_BBS_PROJECT"},
		"BBS_REPO":            {"--bbs-repo / -r", "GHMV_BBS_REPO"},
		"BBS_TOKEN":           {"--bbs-token / -k", "GHMV_BBS_TOKEN"},
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

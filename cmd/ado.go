/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/api/ado"
	"mona-actions/gh-migration-validator/internal/output"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// adoValidationOptions returns the ValidationOptions used for Azure DevOps sources.
// ADO has no native git issues or releases, and branch policies are advisory only.
// The migration log offset does not apply to non-GitHub sources. LFS objects are
// compared (ADO supports Git LFS) unless the user passes --no-lfs.
func adoValidationOptions() validator.ValidationOptions {
	return validator.ValidationOptions{
		SkipIssues:                true,
		SkipReleases:              true,
		SkipPRs:                   viper.GetBool("NO_PRS"),
		SkipWebhooks:              viper.GetBool("NO_WEBHOOKS"),
		SkipMigrationLogOffset:    true,
		SkipMigrationArchive:      true,
		BranchPermissionsAdvisory: true,
		SourceLabel:               "Azure DevOps",
	}
}

// adoCmd represents the ado command
var adoCmd = &cobra.Command{
	Use:   "ado",
	Short: "Validate Azure DevOps (Server or Services/cloud) to GitHub migration",
	Long: `Validate a migration from Azure DevOps to GitHub by comparing repository metrics
between the source ADO repository and the target GitHub repository. Works with both
Azure DevOps Server (on-prem) and Azure DevOps Services (cloud).

This command compares the following metrics:
- Pull Requests (Total, Active→Open, Completed→Merged, Abandoned→Closed)
- Tags count
- Commits count on default branch
- Latest commit SHA
- Branch Policies (advisory comparison with Branch Protection Rules)
- Service Hooks (compared with GitHub webhooks)

Metrics not available in Azure DevOps (Issues, Releases) are automatically skipped. LFS objects are compared unless --no-lfs is set.

Validate a single cloud (Azure DevOps Services) repository — omit --ado-server-url:

  gh migration-validator ado \
    --ado-org "my-org" \
    --ado-team-project "MyProject" \
    --ado-repo "my-repo" \
    --github-target-org "target-org" \
    --target-repo "my-repo"

Validate a single on-prem (Azure DevOps Server) repository — pass --ado-server-url:

  gh migration-validator ado \
    --ado-server-url "https://ado.example.com/tfs" \
    --ado-org "my-collection" \
    --ado-team-project "MyProject" \
    --ado-repo "my-repo" \
    --github-target-org "target-org" \
    --target-repo "my-repo"

Omit --ado-repo to validate every repository in the ADO team project against the
same-named repository in the target GitHub organization, producing a single
consolidated report.

To validate a specific subset of repositories, or when the ADO repository name
differs from the target GitHub repository name, provide a CSV file via --repo-list:

  source_repo,target_repo
  my-repo,my-repo-migrated
  shared-lib,shared-lib

If a line has only one column, the target name is assumed to match the source.
Lines starting with # are comments.`,
	// PreRun binds ADO-specific flags to Viper at execution time.
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("ADO_SERVER_URL", cmd.Flags().Lookup("ado-server-url"))
		viper.BindPFlag("ADO_ORG", cmd.Flags().Lookup("ado-org"))
		viper.BindPFlag("ADO_TEAM_PROJECT", cmd.Flags().Lookup("ado-team-project"))
		viper.BindPFlag("ADO_REPO", cmd.Flags().Lookup("ado-repo"))
		viper.BindPFlag("ADO_PAT", cmd.Flags().Lookup("ado-pat"))
		viper.BindPFlag("ADO_API_VERSION", cmd.Flags().Lookup("ado-api-version"))
		viper.BindPFlag("NO_PRS", cmd.Flags().Lookup("no-prs"))
		viper.BindPFlag("NO_WEBHOOKS", cmd.Flags().Lookup("no-webhooks"))
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required variables (from either flags OR env vars)
		if err := checkADOVars(); err != nil {
			fmt.Printf("Azure DevOps configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Read all values from Viper (single source of truth)
		adoServerURL := viper.GetString("ADO_SERVER_URL")
		adoOrg := viper.GetString("ADO_ORG")
		adoProject := viper.GetString("ADO_TEAM_PROJECT")
		adoRepo := viper.GetString("ADO_REPO")
		adoToken := viper.GetString("ADO_PAT")
		adoAPIVersion := viper.GetString("ADO_API_VERSION")

		// Create ADO client
		adoClient, err := ado.NewADOClient(adoServerURL, adoOrg, adoToken, adoAPIVersion)
		if err != nil {
			fmt.Printf("Failed to initialize Azure DevOps client: %v\n", err)
			os.Exit(1)
		}

		// When the API version was not pinned, negotiate a supported one. Server
		// generations differ (TFS 2018, ADO Server 2019/2020+, ADO Services).
		if adoAPIVersion == "" {
			spinner, _ := pterm.DefaultSpinner.Start("Detecting Azure DevOps API version...")
			version, err := adoClient.NegotiateAPIVersion()
			if err != nil {
				spinner.Fail(fmt.Sprintf("Failed to detect a supported API version: %v", err))
				os.Exit(1)
			}
			spinner.Success(fmt.Sprintf("Using Azure DevOps API version %s", version))
		}

		// Initialize GitHub target API
		ghAPI, err := api.NewTargetOnlyAPI()
		if err != nil {
			fmt.Printf("Failed to initialize target API: %v\n", err)
			os.Exit(1)
		}

		if adoRepo == "" {
			runADOProjectValidation(adoClient, ghAPI, adoProject, cmd)
			return
		}

		runADOSingleRepoValidation(adoClient, ghAPI, adoProject, adoRepo)
	},
}

// runADOSingleRepoValidation validates a single ADO repository against a target GitHub repository.
func runADOSingleRepoValidation(adoClient *ado.ADOClient, ghAPI *api.GitHubAPI, project, repo string) {
	targetOrganization := viper.GetString("TARGET_ORGANIZATION")
	targetRepo := viper.GetString("TARGET_REPO")

	// Validate ADO repository access
	fmt.Println("Validating Azure DevOps repository access...")
	if err := adoClient.ValidateRepoAccess(project, repo); err != nil {
		fmt.Printf("Azure DevOps repository access failed: %v\n", err)
		os.Exit(1)
	}

	// Retrieve ADO repository metrics
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching data from Azure DevOps %s/%s...", project, repo))
	adoData, errorMsgs, err := adoClient.GetRepositoryMetrics(project, repo, spinner)

	// Log any API errors
	output.LogAPIErrors(errorMsgs, project, repo, err)

	if err != nil {
		fmt.Printf("Failed to retrieve Azure DevOps data: %v\n", err)
		os.Exit(1)
	}

	// Create validator and set source data from ADO
	migrationValidator := validator.New(ghAPI)
	migrationValidator.SetSourceData(adoData)

	results, err := migrationValidator.ValidateWithOptions(targetOrganization, targetRepo, adoValidationOptions())
	if err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		os.Exit(1)
	}

	// Display results (also handles markdown output)
	migrationValidator.PrintValidationResults(results)

	if viper.GetBool("STRICT_EXIT") && validator.HasFailures(results) {
		os.Exit(2)
	}
}

// runADOProjectValidation validates every repository in an ADO team project against the
// target GitHub organization, producing a consolidated report. When --repo-list is set,
// it validates the source→target repository pairs from the CSV; otherwise it lists every
// repository in the project and maps each to the same-named target repository.
func runADOProjectValidation(adoClient *ado.ADOClient, ghAPI *api.GitHubAPI, project string, cmd *cobra.Command) {
	targetOrganization := viper.GetString("TARGET_ORGANIZATION")
	repoListFile, _ := cmd.Flags().GetString("repo-list")

	var mappings []validator.RepoMapping

	if repoListFile != "" {
		// Load explicit source→target repo mappings from CSV
		parsed, err := validator.ParseRepoListCSV(repoListFile)
		if err != nil {
			fmt.Printf("Failed to parse repo list: %v\n", err)
			os.Exit(1)
		}
		mappings = parsed
		pterm.Success.Printf("Loaded %d repository mappings from %s\n", len(mappings), repoListFile)
	} else {
		// List all repositories in the ADO project, mapping each to the same-named target
		spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Listing repositories in Azure DevOps project %s...", project))
		repos, err := adoClient.ListRepositories(project)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Failed to list repositories: %v", err))
			os.Exit(1)
		}
		spinner.Success(fmt.Sprintf("Found %d repositories in %s", len(repos), project))

		for _, repo := range repos {
			mappings = append(mappings, validator.RepoMapping{SourceRepo: repo, TargetRepo: repo})
		}
	}

	if len(mappings) == 0 {
		pterm.Warning.Println("No repositories to validate")
		return
	}

	summary := &validator.OrgValidationSummary{
		SourceOrg: fmt.Sprintf("%s (Azure DevOps)", project),
		TargetOrg: targetOrganization,
	}

	markdownFile := viper.GetString("MARKDOWN_FILE")

	for i, mapping := range mappings {
		pterm.DefaultSection.Printf("[%d/%d] Validating repository: %s\n", i+1, len(mappings), mapping.Label())

		entry := validator.RepoValidationResult{
			SourceRepoName: mapping.SourceRepo,
			TargetRepoName: mapping.TargetRepo,
		}

		results, err := validateSingleADORepo(adoClient, ghAPI, project, mapping.SourceRepo, mapping.TargetRepo, targetOrganization)
		if err != nil {
			entry.Error = err.Error()
			pterm.Error.Printf("  %s: %v\n", mapping.SourceRepo, err)
		} else {
			entry.Results = results
		}

		summary.Repos = append(summary.Repos, entry)

		// Incremental markdown write so partial progress survives interruptions
		if markdownFile != "" {
			writeOrgMarkdownFile(summary, markdownFile)
		}
	}

	// Print consolidated results
	validator.PrintOrgValidationResults(summary)

	// Handle markdown output (final write / stdout table)
	outputOrgMarkdown(summary)

	if viper.GetBool("STRICT_EXIT") && validator.OrgHasFailures(summary) {
		os.Exit(2)
	}
}

// validateSingleADORepo fetches ADO metrics for one repo and validates them against
// the target GitHub repository, returning the per-metric results.
func validateSingleADORepo(adoClient *ado.ADOClient, ghAPI *api.GitHubAPI, project, sourceRepo, targetRepo, targetOrganization string) ([]validator.ValidationResult, error) {
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching data from Azure DevOps %s/%s...", project, sourceRepo))
	adoData, errorMsgs, err := adoClient.GetRepositoryMetrics(project, sourceRepo, spinner)

	output.LogAPIErrors(errorMsgs, project, sourceRepo, err)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve Azure DevOps data: %w", err)
	}

	// Fresh validator per repo to avoid state leaking between repositories
	migrationValidator := validator.New(ghAPI)
	migrationValidator.SetSourceData(adoData)

	return migrationValidator.ValidateWithOptions(targetOrganization, targetRepo, adoValidationOptions())
}

func init() {
	// Add ado command to root
	rootCmd.AddCommand(adoCmd)

	// Define ADO-specific flags only — shared target flags are inherited
	// from rootCmd PersistentFlags (github-target-org, github-target-pat,
	// target-hostname, target-repo, markdown-table, markdown-file, no-lfs).
	// Flag names aligned with GEI ado2gh CLI.
	adoCmd.Flags().StringP("ado-server-url", "H", "", "Azure DevOps Server URL for on-prem (e.g., https://ado.example.com/tfs); leave empty for Azure DevOps Services (cloud)")
	adoCmd.Flags().StringP("ado-org", "o", "", "Azure DevOps organization (cloud) or collection (on-prem) name")
	adoCmd.Flags().StringP("ado-team-project", "P", "", "Azure DevOps team project name")
	adoCmd.Flags().StringP("ado-repo", "r", "", "Azure DevOps repository name (omit to validate all repos in the project)")
	adoCmd.Flags().StringP("ado-pat", "k", "", "Azure DevOps personal access token")
	adoCmd.Flags().String("ado-api-version", "", "Azure DevOps REST API version (default: auto-detected; e.g. 7.1 for cloud, 4.1/5.0 for older TFS/ADO Server)")
	adoCmd.Flags().String("repo-list", "", "Path to CSV file with source,target repository mappings (project-wide validation only; omit --ado-repo)")
	adoCmd.Flags().Bool("no-prs", false, "Skip pull request validation (useful for git-only migrations that don't migrate PRs)")
	adoCmd.Flags().Bool("no-webhooks", false, "Skip webhook/service hook validation")
}

// checkADOVars validates the configuration for the ado command.
// When ADO_REPO is set, a single repository is validated and TARGET_REPO is required.
// When ADO_REPO is omitted, all repos in the project are validated against the target org.
func checkADOVars() error {
	required := map[string]requiredConfig{
		"ADO_ORG":             {"--ado-org / -o", "GHMV_ADO_ORG"},
		"ADO_TEAM_PROJECT":    {"--ado-team-project / -P", "GHMV_ADO_TEAM_PROJECT"},
		"ADO_PAT":             {"--ado-pat / -k", "GHMV_ADO_PAT"},
		"TARGET_TOKEN":        {"--github-target-pat / -b", "GHMV_TARGET_TOKEN"},
		"TARGET_ORGANIZATION": {"--github-target-org / -t", "GHMV_TARGET_ORGANIZATION"},
	}

	// A target repository is only required for single-repository validation.
	if viper.GetString("ADO_REPO") != "" {
		required["TARGET_REPO"] = requiredConfig{"--target-repo", "GHMV_TARGET_REPO"}
	}

	for key, info := range required {
		if viper.GetString(key) == "" {
			return fmt.Errorf("%s is required. Set via %s flag or %s environment variable",
				key, info.flag, info.envVar)
		}
	}

	return nil
}

package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/validator"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// inspectCmd represents the inspect command
var inspectCmd = &cobra.Command{
	Use:   "inspect <repository-name>",
	Short: "Show detailed validation results for a specific repository",
	Long: `Show detailed validation results for a specific repository from a saved session.

This command allows you to dive deep into the validation results for a single repository
that was previously validated as part of a batch operation.

Examples:
  gh migration-validator inspect my-repo                    # Uses latest session
  gh migration-validator inspect my-repo --session latest  # Explicitly use latest
  gh migration-validator inspect my-repo --session 2025-09-29_14-30-45
  gh migration-validator inspect my-repo --session ./custom-session.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repoName := args[0]
		sessionFlag := cmd.Flag("session").Value.String()

		// Default to "latest" if no session specified
		if sessionFlag == "" {
			sessionFlag = "latest"
		}

		// Load session
		result, err := validator.LoadSession(sessionFlag)
		if err != nil {
			pterm.Error.Printf("Failed to load session: %v\n", err)
			return
		}

		// Find the repository in the results
		var repoResult *validator.RepositoryValidationResult
		for i := range result.Repositories {
			repo := &result.Repositories[i]
			if repo.SourceRepo == repoName || repo.TargetRepo == repoName {
				repoResult = repo
				break
			}
		}

		if repoResult == nil {
			pterm.Error.Printf("Repository '%s' not found in session\n", repoName)

			// Show available repositories
			var availableRepos []string
			for _, repo := range result.Repositories {
				availableRepos = append(availableRepos, repo.SourceRepo)
			}

			if len(availableRepos) > 0 {
				pterm.Info.Printf("Available repositories: %s\n", strings.Join(availableRepos, ", "))
			}
			return
		}

		// Print detailed results for this repository
		printDetailedRepositoryResults(result, repoResult)
	},
}

func printDetailedRepositoryResults(batchResult *validator.BatchValidationResult, repoResult *validator.RepositoryValidationResult) {
	// Header
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).WithTextStyle(pterm.NewStyle(pterm.FgWhite)).Printf("📊 Detailed Report: %s", repoResult.SourceRepo)

	// Repository info boxes
	sourceInfo := pterm.DefaultBox.WithTitle("Source Repository").WithTitleTopLeft().Sprint(fmt.Sprintf("Repository: %s/%s", repoResult.SourceOwner, repoResult.SourceRepo))
	targetInfo := pterm.DefaultBox.WithTitle("Target Repository").WithTitleTopLeft().Sprint(fmt.Sprintf("Repository: %s/%s", repoResult.TargetOwner, repoResult.TargetRepo))

	pterm.DefaultPanel.WithPanels([][]pterm.Panel{
		{{Data: sourceInfo}, {Data: targetInfo}},
	}).Render()

	fmt.Println() // Add spacing

	// Overall status
	switch repoResult.OverallStatus {
	case "✅ PASS":
		pterm.Success.Println("✅ Repository validation PASSED - All data matches!")
	case "❌ FAIL":
		pterm.Error.Printf("❌ Repository validation FAILED - %s\n", repoResult.FailureReason)
	case "⚠️ WARN":
		pterm.Warning.Printf("⚠️ Repository validation has WARNINGS - %s\n", repoResult.FailureReason)
	}

	fmt.Println() // Add spacing

	// Detailed metrics table
	if len(repoResult.Results) > 0 {
		pterm.DefaultSection.Println("📋 Detailed Metrics")

		tableData := [][]string{
			{"Metric", "Status", "Source Value", "Target Value", "Difference"},
		}

		for _, result := range repoResult.Results {
			diffStr := ""
			if result.Difference > 0 {
				diffStr = fmt.Sprintf("Missing: %d", result.Difference)
			} else if result.Difference < 0 {
				diffStr = fmt.Sprintf("Extra: %d", -result.Difference)
			} else if result.Metric == "Latest Commit SHA" {
				diffStr = "N/A"
			} else {
				diffStr = "Perfect match"
			}

			tableData = append(tableData, []string{
				result.Metric,
				result.Status,
				fmt.Sprintf("%v", result.SourceVal),
				fmt.Sprintf("%v", result.TargetVal),
				diffStr,
			})
		}

		table := pterm.DefaultTable.WithHasHeader().WithData(tableData)
		table.Render()

		fmt.Println() // Add spacing
	}

	// Session info
	pterm.DefaultSection.Println("📅 Session Information")
	sessionInfo := [][]string{
		{"Property", "Value"},
		{"Validation Time", batchResult.Timestamp.Format("2006-01-02 15:04:05")},
		{"Source Organization", batchResult.SourceOrg},
		{"Target Organization", batchResult.TargetOrg},
		{"Total Repositories", fmt.Sprintf("%d", batchResult.Summary.Total)},
		{"Batch Status", fmt.Sprintf("%d✅ %d❌ %d⚠️", batchResult.Summary.Passed, batchResult.Summary.Failed, batchResult.Summary.Warnings)},
	}

	sessionTable := pterm.DefaultTable.WithHasHeader().WithData(sessionInfo)
	sessionTable.Render()

	fmt.Println() // Add spacing

	// Markdown export for this specific repository
	printRepositoryMarkdown(batchResult, repoResult)
}

func printRepositoryMarkdown(batchResult *validator.BatchValidationResult, repoResult *validator.RepositoryValidationResult) {
	pterm.DefaultSection.Println("📋 Markdown Export (Copy-Paste Ready)")

	fmt.Println("```markdown")
	fmt.Printf("# Migration Validation Report: %s\n\n", repoResult.SourceRepo)
	fmt.Printf("**Validation Date:** %s  \n", batchResult.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("**Source:** `%s/%s`  \n", repoResult.SourceOwner, repoResult.SourceRepo)
	fmt.Printf("**Target:** `%s/%s`  \n\n", repoResult.TargetOwner, repoResult.TargetRepo)

	// Status badge
	switch repoResult.OverallStatus {
	case "✅ PASS":
		fmt.Println("**Status:** ✅ PASSED  ")
	case "❌ FAIL":
		fmt.Printf("**Status:** ❌ FAILED - %s  \n", repoResult.FailureReason)
	case "⚠️ WARN":
		fmt.Printf("**Status:** ⚠️ WARNING - %s  \n", repoResult.FailureReason)
	}

	fmt.Println()

	if len(repoResult.Results) > 0 {
		fmt.Println("## Detailed Metrics\n")
		fmt.Println("| Metric | Status | Source Value | Target Value | Difference |")
		fmt.Println("|--------|--------|--------------|--------------|------------|")

		for _, result := range repoResult.Results {
			diffStr := ""
			if result.Difference > 0 {
				diffStr = fmt.Sprintf("Missing: %d", result.Difference)
			} else if result.Difference < 0 {
				diffStr = fmt.Sprintf("Extra: %d", -result.Difference)
			} else if result.Metric == "Latest Commit SHA" {
				diffStr = "N/A"
			} else {
				diffStr = "Perfect match"
			}

			fmt.Printf("| %s | %s | %v | %v | %s |\n",
				result.Metric,
				result.Status,
				result.SourceVal,
				result.TargetVal,
				diffStr)
		}

		fmt.Println()
	}

	fmt.Printf("---\n")
	fmt.Printf("*Generated by gh-migration-validator*\n")
	fmt.Println("```")

	pterm.Info.Println("💡 Tip: You can select and copy the entire markdown section above!")
}

func init() {
	rootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().StringP("session", "s", "", "Session file to load (default: latest)")
}

package validator

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

// RepositoryData holds all the metrics for a repository
type RepositoryData struct {
	Owner           string
	Name            string
	Issues          int
	PRs             *api.PRCounts
	Tags            int
	Releases        int
	CommitCount     int
	LatestCommitSHA string
}

// ValidationResult represents the comparison between source and target
type ValidationResult struct {
	Metric     string
	SourceVal  interface{}
	TargetVal  interface{}
	Status     string // "✅ PASS", "❌ FAIL", "⚠️ WARN"
	Difference int    // How many items are missing in target (negative if target has more)
}

// RepositoryPair represents a source/target repository pair
type RepositoryPair struct {
	SourceRepo string
	TargetRepo string
}

// ValidationError represents a detailed error that occurred during validation
type ValidationError struct {
	ErrorType    string `json:"error_type"`    // "api_error", "validation_error", "network_error", etc.
	ErrorMessage string `json:"error_message"` // Detailed error message
	Timestamp    string `json:"timestamp"`     // When the error occurred
}

// RepositoryValidationResult holds validation results for a single repository pair
type RepositoryValidationResult struct {
	SourceRepo      string             `json:"source_repo"`
	TargetRepo      string             `json:"target_repo"`
	SourceOwner     string             `json:"source_owner"`
	TargetOwner     string             `json:"target_owner"`
	Results         []ValidationResult `json:"results"`
	OverallStatus   string             `json:"overall_status"`             // "✅ PASS", "❌ FAIL", "⚠️ WARN"
	FailureReason   string             `json:"failure_reason"`             // Summary of what failed
	ValidationError *ValidationError   `json:"validation_error,omitempty"` // Detailed error if validation failed
	ProcessingTime  time.Duration      `json:"processing_time"`            // How long validation took
}

// BatchValidationResult holds results for multiple repository validations
type BatchValidationResult struct {
	Timestamp    time.Time
	SourceOrg    string
	TargetOrg    string
	Repositories []RepositoryValidationResult
	Summary      BatchSummary
}

// BatchSummary provides aggregate statistics
type BatchSummary struct {
	Total          int            `json:"total"`
	Passed         int            `json:"passed"`
	Failed         int            `json:"failed"`
	Warnings       int            `json:"warnings"`
	ErrorSummary   map[string]int `json:"error_summary"`   // Count of each error type
	ProcessingTime time.Duration  `json:"processing_time"` // Total time for batch processing
	FailedRepos    []string       `json:"failed_repos"`    // List of repositories that failed
}

// MigrationValidator handles the validation of GitHub organization migrations
type MigrationValidator struct {
	api         *api.GitHubAPI
	SourceData  *RepositoryData
	TargetData  *RepositoryData
	BatchResult *BatchValidationResult
}

// New creates a new MigrationValidator instance
func New(githubAPI *api.GitHubAPI) *MigrationValidator {
	return &MigrationValidator{
		api:        githubAPI,
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}
}

// ParseRepositoryList reads a CSV file and returns repository pairs
func (mv *MigrationValidator) ParseRepositoryList(filePath string) ([]RepositoryPair, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository list file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV file: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file must have at least a header row and one data row")
	}

	// Validate header
	header := records[0]
	if len(header) != 2 || strings.ToLower(header[0]) != "source" || strings.ToLower(header[1]) != "target" {
		return nil, fmt.Errorf("CSV file must have header: source,target")
	}

	var pairs []RepositoryPair
	for i, record := range records[1:] {
		if len(record) != 2 {
			return nil, fmt.Errorf("row %d: expected 2 columns, got %d", i+2, len(record))
		}

		sourceRepo := strings.TrimSpace(record[0])
		targetRepo := strings.TrimSpace(record[1])

		if sourceRepo == "" || targetRepo == "" {
			return nil, fmt.Errorf("row %d: source and target repository names cannot be empty", i+2)
		}

		pairs = append(pairs, RepositoryPair{
			SourceRepo: sourceRepo,
			TargetRepo: targetRepo,
		})
	}

	return pairs, nil
}

// DetermineRepositoryStatus analyzes ValidationResult array and returns overall status and failure reason
func DetermineRepositoryStatus(validationResults []ValidationResult) (status string, failureReason string) {
	hasFailures := false
	hasWarnings := false
	var issues []string

	for _, vr := range validationResults {
		switch vr.Status {
		case "❌ FAIL":
			hasFailures = true
			issues = append(issues, fmt.Sprintf("Missing %s", vr.Metric))
		case "⚠️ WARN":
			hasWarnings = true
			issues = append(issues, fmt.Sprintf("Extra %s", vr.Metric))
		}
	}

	if hasFailures {
		return "❌ FAIL", strings.Join(issues, ", ")
	} else if hasWarnings {
		return "⚠️ WARN", strings.Join(issues, ", ")
	} else {
		return "✅ PASS", ""
	}
}

// ValidateBatch performs validation on multiple repository pairs
func (mv *MigrationValidator) ValidateBatch(sourceOwner, targetOwner string, pairs []RepositoryPair) (*BatchValidationResult, error) {
	batchStartTime := time.Now()
	fmt.Printf("Starting batch migration validation for %d repositories...\n", len(pairs))

	// Initialize batch result with enhanced summary tracking
	result := &BatchValidationResult{
		Timestamp:    batchStartTime,
		SourceOrg:    sourceOwner,
		TargetOrg:    targetOwner,
		Repositories: make([]RepositoryValidationResult, 0, len(pairs)),
		Summary: BatchSummary{
			Total:        len(pairs),
			ErrorSummary: make(map[string]int),
			FailedRepos:  make([]string, 0),
		},
	}

	// Progress bar for batch processing with enhanced status display
	progressBar, _ := pterm.DefaultProgressbar.WithTotal(len(pairs)).WithTitle("Validating repositories").Start()

	// Track progress counts for real-time feedback
	var processed, passed, failed, warnings int

	for i, pair := range pairs {
		repoStartTime := time.Now()

		// Enhanced progress display with current stats
		progressTitle := fmt.Sprintf("Validating %s → %s (%d/%d | ✅ %d ❌ %d ⚠️ %d)",
			pair.SourceRepo, pair.TargetRepo, i+1, len(pairs), passed, failed, warnings)
		progressBar.UpdateTitle(progressTitle)

		// Initialize repository result structure
		repoResult := RepositoryValidationResult{
			SourceRepo:  pair.SourceRepo,
			TargetRepo:  pair.TargetRepo,
			SourceOwner: sourceOwner,
			TargetOwner: targetOwner,
		}

		// Call ValidateMigration for this repository pair
		validationResults, err := mv.ValidateMigration(sourceOwner, pair.SourceRepo, targetOwner, pair.TargetRepo)

		// Record processing time for this repository
		repoResult.ProcessingTime = time.Since(repoStartTime)

		if err != nil {
			// Handle validation error - log actual error without categorization
			repoResult.OverallStatus = "❌ FAIL"
			repoResult.FailureReason = fmt.Sprintf("Validation failed: %v", err)

			// Create detailed error information with actual error
			repoResult.ValidationError = &ValidationError{
				ErrorType:    "validation_error", // Simple, consistent error type
				ErrorMessage: err.Error(),
				Timestamp:    time.Now().Format(time.RFC3339),
			}

			// Update batch summary tracking
			result.Summary.Failed++
			result.Summary.ErrorSummary["validation_error"]++
			result.Summary.FailedRepos = append(result.Summary.FailedRepos, fmt.Sprintf("%s→%s", pair.SourceRepo, pair.TargetRepo))
			failed++

			// Log the actual error for debugging while continuing processing
			pterm.Warning.Printf("Repository %s/%s → %s/%s failed: %v\n",
				sourceOwner, pair.SourceRepo, targetOwner, pair.TargetRepo, err)
		} else {
			// Process successful validation results
			repoResult.Results = validationResults

			// Use shared function to determine overall status
			repoResult.OverallStatus, repoResult.FailureReason = DetermineRepositoryStatus(validationResults)

			// Update summary counters based on status
			switch repoResult.OverallStatus {
			case "❌ FAIL":
				result.Summary.Failed++
				result.Summary.FailedRepos = append(result.Summary.FailedRepos, fmt.Sprintf("%s→%s", pair.SourceRepo, pair.TargetRepo))
				failed++
			case "⚠️ WARN":
				result.Summary.Warnings++
				warnings++
			case "✅ PASS":
				result.Summary.Passed++
				passed++
			}
		}

		// Add repository result to batch
		result.Repositories = append(result.Repositories, repoResult)
		processed++

		// Update progress bar
		progressBar.Increment()
	}

	// Complete batch processing
	result.Summary.ProcessingTime = time.Since(batchStartTime)
	progressBar.Stop()

	// Final summary display
	pterm.Success.Printf("Batch validation completed in %v\n", result.Summary.ProcessingTime)
	pterm.Info.Printf("Results: ✅ %d passed, ❌ %d failed, ⚠️ %d warnings out of %d repositories\n",
		result.Summary.Passed, result.Summary.Failed, result.Summary.Warnings, result.Summary.Total)

	// Store result for session management
	mv.BatchResult = result

	return result, nil
}

// ValidateMigration performs the migration validation logic and returns results
func (mv *MigrationValidator) ValidateMigration(sourceOwner, sourceRepo, targetOwner, targetRepo string) ([]ValidationResult, error) {
	fmt.Println("Starting migration validation...")

	// Retrieve source repository data
	if err := mv.RetrieveSource(sourceOwner, sourceRepo); err != nil {
		return nil, fmt.Errorf("failed to retrieve source data: %w", err)
	}

	// Retrieve target repository data
	if err := mv.RetrieveTarget(targetOwner, targetRepo); err != nil {
		return nil, fmt.Errorf("failed to retrieve target data: %w", err)
	}

	// Compare and validate the data
	fmt.Println("\nValidating migration data...")
	results := mv.ValidateRepositoryData()

	fmt.Println("Migration validation completed!")
	return results, nil
}

// RetrieveSource retrieves all repository data from the source repository
func (mv *MigrationValidator) RetrieveSource(owner, name string) error {
	fmt.Printf("Retrieving data from source repository: %s/%s\n", owner, name)

	mv.SourceData.Owner = owner
	mv.SourceData.Name = name

	// Get issue count
	spinner, _ := pterm.DefaultSpinner.Start("Fetching issues...")
	issues, err := mv.api.GetIssueCount(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch issues")
		return fmt.Errorf("failed to get source issue count: %w", err)
	}
	mv.SourceData.Issues = issues
	spinner.Success("Issues fetched successfully")

	// Get PR counts
	spinner, _ = pterm.DefaultSpinner.Start("Fetching pull requests...")
	prCounts, err := mv.api.GetPRCounts(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch pull requests")
		return fmt.Errorf("failed to get source PR counts: %w", err)
	}
	mv.SourceData.PRs = prCounts
	spinner.Success("Pull requests fetched successfully")

	// Get tag count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching tags...")
	tags, err := mv.api.GetTagCount(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch tags")
		return fmt.Errorf("failed to get source tag count: %w", err)
	}
	mv.SourceData.Tags = tags
	spinner.Success("Tags fetched successfully")

	// Get release count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching releases...")
	releases, err := mv.api.GetReleaseCount(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch releases")
		return fmt.Errorf("failed to get source release count: %w", err)
	}
	mv.SourceData.Releases = releases
	spinner.Success("Releases fetched successfully")

	// Get commit count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching commit count...")
	commitCount, err := mv.api.GetCommitCount(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch commit count")
		return fmt.Errorf("failed to get source commit count: %w", err)
	}
	mv.SourceData.CommitCount = commitCount
	spinner.Success("Commit count fetched successfully")

	// Get latest commit hash
	spinner, _ = pterm.DefaultSpinner.Start("Fetching latest commit hash...")
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.SourceClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch latest commit hash")
		return fmt.Errorf("failed to get source latest commit hash: %w", err)
	}
	mv.SourceData.LatestCommitSHA = latestCommitSHA
	spinner.Success("Latest commit hash fetched successfully")

	fmt.Printf("Source data retrieved successfully!\n")
	return nil
}

// RetrieveTarget retrieves all repository data from the target repository
func (mv *MigrationValidator) RetrieveTarget(owner, name string) error {
	fmt.Printf("Retrieving data from target repository: %s/%s\n", owner, name)

	mv.TargetData.Owner = owner
	mv.TargetData.Name = name

	// Get issue count
	spinner, _ := pterm.DefaultSpinner.Start("Fetching issues...")
	issues, err := mv.api.GetIssueCount(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch issues")
		return fmt.Errorf("failed to get target issue count: %w", err)
	}
	mv.TargetData.Issues = issues
	spinner.Success("Issues fetched successfully")

	// Get PR counts
	spinner, _ = pterm.DefaultSpinner.Start("Fetching pull requests...")
	prCounts, err := mv.api.GetPRCounts(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch pull requests")
		return fmt.Errorf("failed to get target PR counts: %w", err)
	}
	mv.TargetData.PRs = prCounts
	spinner.Success("Pull requests fetched successfully")

	// Get tag count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching tags...")
	tags, err := mv.api.GetTagCount(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch tags")
		return fmt.Errorf("failed to get target tag count: %w", err)
	}
	mv.TargetData.Tags = tags
	spinner.Success("Tags fetched successfully")

	// Get release count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching releases...")
	releases, err := mv.api.GetReleaseCount(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch releases")
		return fmt.Errorf("failed to get target release count: %w", err)
	}
	mv.TargetData.Releases = releases
	spinner.Success("Releases fetched successfully")

	// Get commit count
	spinner, _ = pterm.DefaultSpinner.Start("Fetching commit count...")
	commitCount, err := mv.api.GetCommitCount(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch commit count")
		return fmt.Errorf("failed to get target commit count: %w", err)
	}
	mv.TargetData.CommitCount = commitCount
	spinner.Success("Commit count fetched successfully")

	// Get latest commit hash
	spinner, _ = pterm.DefaultSpinner.Start("Fetching latest commit hash...")
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.TargetClient, owner, name)
	if err != nil {
		spinner.Fail("Failed to fetch latest commit hash")
		return fmt.Errorf("failed to get target latest commit hash: %w", err)
	}
	mv.TargetData.LatestCommitSHA = latestCommitSHA
	spinner.Success("Latest commit hash fetched successfully")

	fmt.Printf("Target data retrieved successfully!\n")
	return nil
}

// ValidateRepositoryData compares source and target repository data and returns validation results
func (mv *MigrationValidator) ValidateRepositoryData() []ValidationResult {
	fmt.Println("Comparing repository data...")

	var results []ValidationResult

	// Compare Issues (target should have source issues + 1 for migration logging issue)
	expectedTargetIssues := mv.SourceData.Issues + 1
	issueDiff := expectedTargetIssues - mv.TargetData.Issues
	issueStatus := "✅ PASS"
	if issueDiff > 0 {
		issueStatus = "❌ FAIL"
	} else if issueDiff < 0 {
		issueStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Issues (expected +1 for migration log)",
		SourceVal:  fmt.Sprintf("%d (expected target: %d)", mv.SourceData.Issues, expectedTargetIssues),
		TargetVal:  mv.TargetData.Issues,
		Status:     issueStatus,
		Difference: issueDiff,
	})

	// Compare Total PRs
	prDiff := mv.SourceData.PRs.Total - mv.TargetData.PRs.Total
	prStatus := "✅ PASS"
	if prDiff > 0 {
		prStatus = "❌ FAIL"
	} else if prDiff < 0 {
		prStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Total)",
		SourceVal:  mv.SourceData.PRs.Total,
		TargetVal:  mv.TargetData.PRs.Total,
		Status:     prStatus,
		Difference: prDiff,
	})

	// Compare Open PRs
	openPRDiff := mv.SourceData.PRs.Open - mv.TargetData.PRs.Open
	openPRStatus := "✅ PASS"
	if openPRDiff > 0 {
		openPRStatus = "❌ FAIL"
	} else if openPRDiff < 0 {
		openPRStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Open)",
		SourceVal:  mv.SourceData.PRs.Open,
		TargetVal:  mv.TargetData.PRs.Open,
		Status:     openPRStatus,
		Difference: openPRDiff,
	})

	// Compare Merged PRs
	mergedPRDiff := mv.SourceData.PRs.Merged - mv.TargetData.PRs.Merged
	mergedPRStatus := "✅ PASS"
	if mergedPRDiff > 0 {
		mergedPRStatus = "❌ FAIL"
	} else if mergedPRDiff < 0 {
		mergedPRStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Merged)",
		SourceVal:  mv.SourceData.PRs.Merged,
		TargetVal:  mv.TargetData.PRs.Merged,
		Status:     mergedPRStatus,
		Difference: mergedPRDiff,
	})

	// Compare Tags
	tagDiff := mv.SourceData.Tags - mv.TargetData.Tags
	tagStatus := "✅ PASS"
	if tagDiff > 0 {
		tagStatus = "❌ FAIL"
	} else if tagDiff < 0 {
		tagStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Tags",
		SourceVal:  mv.SourceData.Tags,
		TargetVal:  mv.TargetData.Tags,
		Status:     tagStatus,
		Difference: tagDiff,
	})

	// Compare Releases
	releaseDiff := mv.SourceData.Releases - mv.TargetData.Releases
	releaseStatus := "✅ PASS"
	if releaseDiff > 0 {
		releaseStatus = "❌ FAIL"
	} else if releaseDiff < 0 {
		releaseStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Releases",
		SourceVal:  mv.SourceData.Releases,
		TargetVal:  mv.TargetData.Releases,
		Status:     releaseStatus,
		Difference: releaseDiff,
	})

	// Compare Commit Count
	commitDiff := mv.SourceData.CommitCount - mv.TargetData.CommitCount
	commitStatus := "✅ PASS"
	if commitDiff > 0 {
		commitStatus = "❌ FAIL"
	} else if commitDiff < 0 {
		commitStatus = "⚠️ WARN"
	}

	results = append(results, ValidationResult{
		Metric:     "Commits",
		SourceVal:  mv.SourceData.CommitCount,
		TargetVal:  mv.TargetData.CommitCount,
		Status:     commitStatus,
		Difference: commitDiff,
	})

	// Compare Latest Commit SHA
	latestCommitStatus := "✅ PASS"
	if mv.SourceData.LatestCommitSHA != mv.TargetData.LatestCommitSHA {
		latestCommitStatus = "❌ FAIL"
	}

	results = append(results, ValidationResult{
		Metric:     "Latest Commit SHA",
		SourceVal:  mv.SourceData.LatestCommitSHA,
		TargetVal:  mv.TargetData.LatestCommitSHA,
		Status:     latestCommitStatus,
		Difference: 0, // Not applicable for SHA comparison
	})

	return results
}

// PrintBatchResults prints results for multiple repository validation (executive summary style)
func (mv *MigrationValidator) PrintBatchResults(result *BatchValidationResult) {
	// Executive Summary Header
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).WithTextStyle(pterm.NewStyle(pterm.FgWhite)).Println("📊 Migration Validation Report")

	// Organization info
	sourceOrgInfo := pterm.DefaultBox.WithTitle("Source Organization").WithTitleTopLeft().Sprint(fmt.Sprintf("Organization: %s", result.SourceOrg))
	targetOrgInfo := pterm.DefaultBox.WithTitle("Target Organization").WithTitleTopLeft().Sprint(fmt.Sprintf("Organization: %s", result.TargetOrg))

	pterm.DefaultPanel.WithPanels([][]pterm.Panel{
		{{Data: sourceOrgInfo}, {Data: targetOrgInfo}},
	}).Render()

	fmt.Println() // Add spacing

	// Executive Summary with enhanced timing information
	pterm.DefaultSection.Println("🎯 EXECUTIVE SUMMARY")

	summaryData := [][]string{
		{"Metric", "Count", "Percentage"},
		{"Total Repositories", fmt.Sprintf("%d", result.Summary.Total), "100%"},
		{"✅ Passed", fmt.Sprintf("%d", result.Summary.Passed), fmt.Sprintf("%.1f%%", float64(result.Summary.Passed)/float64(result.Summary.Total)*100)},
		{"❌ Failed", fmt.Sprintf("%d", result.Summary.Failed), fmt.Sprintf("%.1f%%", float64(result.Summary.Failed)/float64(result.Summary.Total)*100)},
		{"⚠️ Warnings", fmt.Sprintf("%d", result.Summary.Warnings), fmt.Sprintf("%.1f%%", float64(result.Summary.Warnings)/float64(result.Summary.Total)*100)},
		{"Processing Time", result.Summary.ProcessingTime.Round(time.Second).String(), ""},
	}

	summaryTable := pterm.DefaultTable.WithHasHeader().WithData(summaryData)
	summaryTable.Render()

	// Error Summary Section (if there are errors)
	if len(result.Summary.ErrorSummary) > 0 {
		fmt.Println() // Add spacing
		pterm.DefaultSection.Println("📋 ERROR BREAKDOWN")

		errorTableData := [][]string{
			{"Error Type", "Count", "Percentage of Failures"},
		}

		for errorType, count := range result.Summary.ErrorSummary {
			percentage := "N/A"
			if result.Summary.Failed > 0 {
				percentage = fmt.Sprintf("%.1f%%", float64(count)/float64(result.Summary.Failed)*100)
			}
			errorTableData = append(errorTableData, []string{
				strings.ReplaceAll(strings.Title(strings.ReplaceAll(errorType, "_", " ")), " ", " "),
				fmt.Sprintf("%d", count),
				percentage,
			})
		}

		errorTable := pterm.DefaultTable.WithHasHeader().WithData(errorTableData)
		errorTable.Render()
	}

	fmt.Println() // Add spacing

	// Attention Required Section (Failed and Warning repositories)
	attentionRepos := make([]RepositoryValidationResult, 0)
	for _, repo := range result.Repositories {
		if repo.OverallStatus == "❌ FAIL" || repo.OverallStatus == "⚠️ WARN" {
			attentionRepos = append(attentionRepos, repo)
		}
	}

	if len(attentionRepos) > 0 {
		pterm.DefaultSection.Println("🚨 ATTENTION REQUIRED")

		// Failed Repository Details Table with enhanced error information
		failedTableData := [][]string{
			{"Repository", "Status", "Issues Found", "Error Type", "Processing Time"},
		}

		for _, repo := range attentionRepos {
			errorType := ""
			if repo.ValidationError != nil {
				errorType = strings.Title(strings.ReplaceAll(repo.ValidationError.ErrorType, "_", " "))
			}

			failedTableData = append(failedTableData, []string{
				fmt.Sprintf("%s → %s", repo.SourceRepo, repo.TargetRepo),
				repo.OverallStatus,
				repo.FailureReason,
				errorType,
				repo.ProcessingTime.Round(time.Second).String(),
			})
		}

		failedTable := pterm.DefaultTable.WithHasHeader().WithData(failedTableData)
		failedTable.Render()

		fmt.Println() // Add spacing
	}

	// Successful Migrations (Collapsed List)
	passedRepos := make([]string, 0)
	for _, repo := range result.Repositories {
		if repo.OverallStatus == "✅ PASS" {
			passedRepos = append(passedRepos, fmt.Sprintf("%s→%s", repo.SourceRepo, repo.TargetRepo))
		}
	}

	if len(passedRepos) > 0 {
		pterm.DefaultSection.Println("✅ SUCCESSFUL MIGRATIONS")

		// Show first few, then collapse if too many
		const maxShow = 10
		if len(passedRepos) <= maxShow {
			pterm.Info.Println(strings.Join(passedRepos, ", "))
		} else {
			displayed := strings.Join(passedRepos[:maxShow], ", ")
			pterm.Info.Printf("%s... and %d more\n", displayed, len(passedRepos)-maxShow)
		}
		fmt.Println() // Add spacing
	}

	// Final Status
	if result.Summary.Failed > 0 {
		pterm.Error.Printf("❌ Batch validation FAILED - %d repositories have missing data\n", result.Summary.Failed)
	} else if result.Summary.Warnings > 0 {
		pterm.Warning.Printf("⚠️ Batch validation completed with WARNINGS - %d repositories have extra data\n", result.Summary.Warnings)
	} else {
		pterm.Success.Println("✅ Batch validation PASSED - All repositories match!")
	}

	fmt.Println() // Add spacing

	// Tip for detailed analysis
	if len(attentionRepos) > 0 {
		pterm.Info.Println("💡 Tip: Use 'gh migration-validator inspect <repo-name>' for detailed analysis")
	}
}

// PrintValidationResults prints a formatted report for single repository validation
func (mv *MigrationValidator) PrintValidationResults(results []ValidationResult) {
	// Print header
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).WithTextStyle(pterm.NewStyle(pterm.FgWhite)).Println("📊 Migration Validation Report")

	// Print source/target info
	sourceInfo := pterm.DefaultBox.WithTitle("Source Repository").WithTitleTopLeft().Sprint(fmt.Sprintf("Repository: %s/%s", mv.SourceData.Owner, mv.SourceData.Name))
	targetInfo := pterm.DefaultBox.WithTitle("Target Repository").WithTitleTopLeft().Sprint(fmt.Sprintf("Repository: %s/%s", mv.TargetData.Owner, mv.TargetData.Name))

	pterm.DefaultPanel.WithPanels([][]pterm.Panel{
		{{Data: sourceInfo}, {Data: targetInfo}},
	}).Render()

	fmt.Println() // Add spacing

	// Create table data
	tableData := [][]string{
		{"Metric", "Status", "Source Value", "Target Value", "Difference"},
	}

	for _, result := range results {
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

	// Create and display the table
	table := pterm.DefaultTable.WithHasHeader().WithData(tableData)
	table.Render()

	fmt.Println() // Add spacing

	// Calculate summary
	passCount := 0
	failCount := 0
	warnCount := 0

	for _, result := range results {
		switch result.Status {
		case "✅ PASS":
			passCount++
		case "❌ FAIL":
			failCount++
		case "⚠️ WARN":
			warnCount++
		}
	}

	// Print summary with colored boxes
	summaryData := []pterm.BulletListItem{
		{Level: 0, Text: fmt.Sprintf("Passed: %d", passCount), TextStyle: pterm.NewStyle(pterm.FgGreen)},
		{Level: 0, Text: fmt.Sprintf("Failed: %d", failCount), TextStyle: pterm.NewStyle(pterm.FgRed)},
		{Level: 0, Text: fmt.Sprintf("Warnings: %d", warnCount), TextStyle: pterm.NewStyle(pterm.FgYellow)},
	}

	pterm.DefaultBulletList.WithItems(summaryData).WithBullet("📊").Render()

	fmt.Println() // Add spacing

	// Final status with prominent styling
	if failCount > 0 {
		pterm.Error.Println("❌ Migration validation FAILED - Some data is missing in target")
	} else if warnCount > 0 {
		pterm.Warning.Println("⚠️ Migration validation completed with WARNINGS - Target has more data than source")
	} else {
		pterm.Success.Println("✅ Migration validation PASSED - All data matches!")
	}

	fmt.Println() // Add spacing
	if viper.GetBool("MARKDOWN_TABLE") {
		mv.PrintMarkdownTable(results)
	}
}

// PrintMarkdownTable prints a markdown-formatted table for easy copy/paste
func (mv *MigrationValidator) PrintMarkdownTable(results []ValidationResult) {
	pterm.DefaultSection.Println("📋 Markdown Table (Copy-Paste Ready)")

	fmt.Println("```markdown")
	fmt.Printf("# Migration Validation Report\n\n")
	fmt.Printf("**Source:** `%s/%s`  \n", mv.SourceData.Owner, mv.SourceData.Name)
	fmt.Printf("**Target:** `%s/%s`  \n\n", mv.TargetData.Owner, mv.TargetData.Name)

	fmt.Println("| Metric | Status | Source Value | Target Value | Difference |")
	fmt.Println("|--------|--------|--------------|--------------|------------|")

	for _, result := range results {
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

	// Calculate summary for markdown
	passCount := 0
	failCount := 0
	warnCount := 0

	for _, result := range results {
		switch result.Status {
		case "✅ PASS":
			passCount++
		case "❌ FAIL":
			failCount++
		case "⚠️ WARN":
			warnCount++
		}
	}

	fmt.Printf("\n## Summary\n\n")
	fmt.Printf("- **Passed:** %d  \n", passCount)
	fmt.Printf("- **Failed:** %d  \n", failCount)
	fmt.Printf("- **Warnings:** %d  \n\n", warnCount)

	if failCount > 0 {
		fmt.Printf("**Result:** ❌ Migration validation FAILED - Some data is missing in target\n")
	} else if warnCount > 0 {
		fmt.Printf("**Result:** ⚠️ Migration validation completed with WARNINGS - Target has more data than source\n")
	} else {
		fmt.Printf("**Result:** ✅ Migration validation PASSED - All data matches!\n")
	}

	fmt.Println("```")

	pterm.Info.Println("💡 Tip: You can select and copy the entire markdown section above to paste into documentation, issues, or pull requests!")
}

// GetSessionDir returns the directory where sessions are stored
func GetSessionDir() (string, error) {
	// Use current working directory instead of user home directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	sessionDir := filepath.Join(currentDir, ".ghmv", "sessions")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	return sessionDir, nil
}

// SaveSession saves the batch results to a session file
func (mv *MigrationValidator) SaveSession() (string, error) {
	if mv.BatchResult == nil {
		return "", fmt.Errorf("no batch result to save")
	}

	sessionDir, err := GetSessionDir()
	if err != nil {
		return "", err
	}

	// Create timestamp-based filename
	timestamp := mv.BatchResult.Timestamp.Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s.json", timestamp)
	sessionPath := filepath.Join(sessionDir, filename)

	// Save as JSON
	data, err := json.MarshalIndent(mv.BatchResult, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch result: %w", err)
	}

	if err := os.WriteFile(sessionPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write session file: %w", err)
	}

	// Also save as "latest.json" for easy access
	latestPath := filepath.Join(sessionDir, "latest.json")
	if err := os.WriteFile(latestPath, data, 0644); err != nil {
		// Don't fail if we can't write latest, just warn
		pterm.Warning.Printf("Could not update latest session: %v\n", err)
	}

	return sessionPath, nil
}

// LoadSession loads a batch result from a session file
func LoadSession(sessionPath string) (*BatchValidationResult, error) {
	// Handle special cases
	if sessionPath == "latest" {
		sessionDir, err := GetSessionDir()
		if err != nil {
			return nil, err
		}
		sessionPath = filepath.Join(sessionDir, "latest.json")
	}

	// Check if file exists
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session file does not exist: %s", sessionPath)
	}

	// Read and unmarshal
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var result BatchValidationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &result, nil
}

// ShouldSaveSession determines if a session should be automatically saved
func (mv *MigrationValidator) ShouldSaveSession() bool {
	if mv.BatchResult == nil {
		return false
	}

	// Always save in CI or non-interactive environments
	if os.Getenv("CI") != "" || !isInteractiveTerminal() {
		return true
	}

	// Save if there are any issues (failures or warnings)
	return mv.BatchResult.Summary.Failed > 0 || mv.BatchResult.Summary.Warnings > 0
}

// isInteractiveTerminal checks if we're running in an interactive terminal
func isInteractiveTerminal() bool {
	// Simple check - in a real implementation you might want more sophisticated detection
	return os.Getenv("TERM") != "" && os.Getenv("CI") == ""
}

// AutoSaveSessionIfNeeded saves the session automatically based on smart rules
func (mv *MigrationValidator) AutoSaveSessionIfNeeded() {
	if !mv.ShouldSaveSession() {
		return
	}

	sessionPath, err := mv.SaveSession()
	if err != nil {
		pterm.Warning.Printf("Failed to save session: %v\n", err)
		return
	}

	if mv.BatchResult.Summary.Failed > 0 || mv.BatchResult.Summary.Warnings > 0 {
		pterm.Info.Printf("💾 Session saved: %s\n", sessionPath)
		pterm.Info.Println("💡 Use 'gh migration-validator inspect <repo-name>' for detailed analysis")
	}
}

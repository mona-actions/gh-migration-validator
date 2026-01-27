package validator

import (
	"bytes"
	"fmt"
	"io"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/migrationarchive"
	"mona-actions/gh-migration-validator/internal/output"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

// ValidationStatus represents the logical status of a validation result
type ValidationStatus int

const (
	ValidationStatusMessagePass = "✅ PASS"
	ValidationStatusMessageFail = "❌ FAIL"
	ValidationStatusMessageWarn = "⚠️ WARN"
)

const (
	ValidationStatusPass ValidationStatus = iota
	ValidationStatusFail
	ValidationStatusWarn
)

// MigrationLogIssueOffset represents the additional issue created during migration
const MigrationLogIssueOffset = 1

// getValidationStatus returns both display string and enum value based on difference
// diff > 0: target has fewer items than source (FAIL)
// diff < 0: target has more items than source (WARN)
// diff = 0: perfect match (PASS)
func getValidationStatus(diff int) (string, ValidationStatus) {
	switch {
	case diff > 0:
		return ValidationStatusMessageFail, ValidationStatusFail
	case diff < 0:
		return ValidationStatusMessageWarn, ValidationStatusWarn
	default:
		return ValidationStatusMessagePass, ValidationStatusPass
	}
}

// RepositoryData holds all the metrics for a repository
type RepositoryData struct {
	Owner                 string
	Name                  string
	Issues                int
	PRs                   *api.PRCounts
	Tags                  int
	Releases              int
	CommitCount           int
	LatestCommitSHA       string
	BranchProtectionRules int
	Webhooks              int
	MigrationArchive      *migrationarchive.MigrationArchiveMetrics `json:"migration_archive,omitempty"`
}

// ValidationResult represents the comparison between source and target
type ValidationResult struct {
	Metric     string
	SourceVal  interface{}
	TargetVal  interface{}
	Status     string           // "✅ PASS", "❌ FAIL", "⚠️ WARN" - for display
	StatusType ValidationStatus // Pass, Fail, Warn - for logic/testing
	Difference int              // How many items are missing in target (negative if target has more)
}

// MigrationValidator handles the validation of GitHub organization migrations
type MigrationValidator struct {
	api        *api.GitHubAPI
	SourceData *RepositoryData
	TargetData *RepositoryData
}

// New creates a new MigrationValidator instance
func New(githubAPI *api.GitHubAPI) *MigrationValidator {
	return &MigrationValidator{
		api:        githubAPI,
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}
}

// ValidateMigration performs the migration validation logic and returns results
func (mv *MigrationValidator) ValidateMigration(sourceOwner, sourceRepo, targetOwner, targetRepo string) ([]ValidationResult, error) {
	// Validate access to both repositories before starting expensive operations
	fmt.Println("Validating repository access...")
	if err := mv.api.ValidateRepoAccess(api.SourceClient, sourceOwner, sourceRepo); err != nil {
		return nil, fmt.Errorf("cannot access source repository %s/%s: %w", sourceOwner, sourceRepo, err)
	}
	if err := mv.api.ValidateRepoAccess(api.TargetClient, targetOwner, targetRepo); err != nil {
		return nil, fmt.Errorf("cannot access target repository %s/%s: %w", targetOwner, targetRepo, err)
	}

	// Check rate limits before starting - warn if low
	mv.checkAndWarnRateLimits()

	fmt.Println("Starting migration validation...")
	fmt.Printf("Source: %s/%s | Target: %s/%s\n", sourceOwner, sourceRepo, targetOwner, targetRepo)

	// Create a multi printer. This allows multiple spinners to print simultaneously.
	multi := pterm.DefaultMultiPrinter

	// Create spinners for source and target with separate writers from the multi printer
	sourceSpinner, _ := pterm.DefaultSpinner.WithWriter(multi.NewWriter()).Start(fmt.Sprintf("Preparing to retrieve data from %s/%s...", sourceOwner, sourceRepo))
	targetSpinner, _ := pterm.DefaultSpinner.WithWriter(multi.NewWriter()).Start(fmt.Sprintf("Preparing to retrieve data from %s/%s...", targetOwner, targetRepo))

	// Start the multi printer
	multi.Start()

	// Use WaitGroup to wait for both goroutines to complete
	var wg sync.WaitGroup
	var sourceErr, targetErr error
	var sourceErrorMsgs, targetErrorMsgs []string

	// Channel to synchronize goroutines
	wg.Add(2)

	// Retrieve source repository data in a goroutine
	go func() {
		defer wg.Done()
		sourceErrorMsgs, sourceErr = mv.retrieveSource(sourceOwner, sourceRepo, sourceSpinner)
	}()

	// Retrieve target repository data in a goroutine
	go func() {
		defer wg.Done()
		targetErrorMsgs, targetErr = mv.retrieveTarget(targetOwner, targetRepo, targetSpinner)
	}()

	// Wait for both goroutines to complete
	wg.Wait()

	// Stop the multi printer
	multi.Stop()

	// Log any API errors (safe to call after spinners finish)
	output.LogAPIErrors(sourceErrorMsgs, sourceOwner, sourceRepo, sourceErr)
	output.LogAPIErrors(targetErrorMsgs, targetOwner, targetRepo, targetErr)

	// Check for errors from both operations
	if sourceErr != nil {
		return nil, fmt.Errorf("failed to retrieve source data: %w", sourceErr)
	}
	if targetErr != nil {
		return nil, fmt.Errorf("failed to retrieve target data: %w", targetErr)
	}

	// Compare and validate the data
	fmt.Println("\nValidating migration data...")
	results := mv.validateRepositoryData()

	fmt.Println("Migration validation completed!")
	return results, nil
}

// checks rate limits for both source and target clients (configurable via RATE_LIMIT_THRESHOLD env var, default 50).
// Set threshold to 0 to disable rate limit warnings.
func (mv *MigrationValidator) checkAndWarnRateLimits() {
	viper.SetDefault("RATE_LIMIT_THRESHOLD", 50)
	threshold := viper.GetInt("RATE_LIMIT_THRESHOLD")

	sourceRL, sourceErr := mv.api.GetRateLimitStatus(api.SourceClient)
	targetRL, targetErr := mv.api.GetRateLimitStatus(api.TargetClient)

	if sourceErr != nil {
		pterm.DefaultLogger.Warn("Source API rate limit check failed", pterm.DefaultLogger.Args("error", sourceErr.Error()))
	} else {
		output.LogRateLimitWarning("Source", sourceRL.Remaining, sourceRL.ResetAt, threshold)
	}

	if targetErr != nil {
		pterm.DefaultLogger.Warn("Target API rate limit check failed", pterm.DefaultLogger.Args("error", targetErr.Error()))
	} else {
		output.LogRateLimitWarning("Target", targetRL.Remaining, targetRL.ResetAt, threshold)
	}
}

// retrieveSource retrieves all repository data from the source repository.
// Returns a slice of error messages for display after spinners finish, and an error if all requests failed.
// An empty slice indicates all requests succeeded; callers should only expect error messages when
// partial failures occur (some requests succeeded, some failed).
func (mv *MigrationValidator) retrieveSource(owner, name string, spinner *pterm.SpinnerPrinter) ([]string, error) {
	startTime := time.Now()
	var failedRequests []string
	var errorMessages []string
	var successfulRequests int

	mv.SourceData.Owner = owner
	mv.SourceData.Name = name

	// Get issue count
	spinner.UpdateText(fmt.Sprintf("Fetching issues from %s/%s...", owner, name))
	issues, err := mv.api.GetIssueCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "issues")
		errorMessages = append(errorMessages, fmt.Sprintf("issues: %v", err))
		mv.SourceData.Issues = 0
	} else {
		mv.SourceData.Issues = issues
		successfulRequests++
	}

	// Get PR counts
	spinner.UpdateText(fmt.Sprintf("Fetching pull requests from %s/%s...", owner, name))
	prCounts, err := mv.api.GetPRCounts(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "pull requests")
		errorMessages = append(errorMessages, fmt.Sprintf("pull requests: %v", err))
		mv.SourceData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	} else {
		mv.SourceData.PRs = prCounts
		successfulRequests++
	}

	// Get tag count
	spinner.UpdateText(fmt.Sprintf("Fetching tags from %s/%s...", owner, name))
	tags, err := mv.api.GetTagCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "tags")
		errorMessages = append(errorMessages, fmt.Sprintf("tags: %v", err))
		mv.SourceData.Tags = 0
	} else {
		mv.SourceData.Tags = tags
		successfulRequests++
	}

	// Get release count
	spinner.UpdateText(fmt.Sprintf("Fetching releases from %s/%s...", owner, name))
	releases, err := mv.api.GetReleaseCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "releases")
		errorMessages = append(errorMessages, fmt.Sprintf("releases: %v", err))
		mv.SourceData.Releases = 0
	} else {
		mv.SourceData.Releases = releases
		successfulRequests++
	}

	// Get commit count
	spinner.UpdateText(fmt.Sprintf("Fetching commit count from %s/%s...", owner, name))
	commitCount, err := mv.api.GetCommitCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "commits")
		errorMessages = append(errorMessages, fmt.Sprintf("commits: %v", err))
		mv.SourceData.CommitCount = 0
	} else {
		mv.SourceData.CommitCount = commitCount
		successfulRequests++
	}

	// Get latest commit hash
	spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", owner, name))
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "latest commit hash")
		errorMessages = append(errorMessages, fmt.Sprintf("latest commit hash: %v", err))
		mv.SourceData.LatestCommitSHA = ""
	} else {
		mv.SourceData.LatestCommitSHA = latestCommitSHA
		successfulRequests++
	}

	// Get branch protection rules count
	spinner.UpdateText(fmt.Sprintf("Fetching branch protection rules from %s/%s...", owner, name))
	branchProtectionRules, err := mv.api.GetBranchProtectionRulesCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "branch protection rules")
		errorMessages = append(errorMessages, fmt.Sprintf("branch protection rules: %v", err))
		mv.SourceData.BranchProtectionRules = 0
	} else {
		mv.SourceData.BranchProtectionRules = branchProtectionRules
		successfulRequests++
	}

	// Get webhook count
	spinner.UpdateText(fmt.Sprintf("Fetching webhooks from %s/%s...", owner, name))
	webhooks, err := mv.api.GetWebhookCount(api.SourceClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "webhooks")
		errorMessages = append(errorMessages, fmt.Sprintf("webhooks: %v", err))
		mv.SourceData.Webhooks = 0
	} else {
		mv.SourceData.Webhooks = webhooks
		successfulRequests++
	}

	duration := time.Since(startTime)

	// Determine success/failure status
	if successfulRequests == 0 {
		spinner.Fail(fmt.Sprintf("Failed to retrieve any data from %s/%s", owner, name))
		return errorMessages, fmt.Errorf("all API requests failed for %s/%s", owner, name)
	}

	if len(failedRequests) > 0 {
		spinner.Warning(fmt.Sprintf("%s/%s: %d OK, %d failed (%v) - missing: %v",
			owner, name, successfulRequests, len(failedRequests), duration, failedRequests))
	} else {
		spinner.Success(fmt.Sprintf("%s/%s retrieved successfully (%v)", owner, name, duration))
	}

	return errorMessages, nil
}

// RetrieveSourceData is a public wrapper for retrieveSource for use by the export package
func (mv *MigrationValidator) RetrieveSourceData(owner, name string, spinner *pterm.SpinnerPrinter) ([]string, error) {
	return mv.retrieveSource(owner, name, spinner)
}

// SetSourceDataFromExport sets the source data from an export instead of fetching from API
func (mv *MigrationValidator) SetSourceDataFromExport(exportData *RepositoryData) {
	// Create a deep copy to prevent external mutation
	sourceDataCopy := *exportData

	// Clone the PRCounts struct if it exists to achieve true isolation
	if exportData.PRs != nil {
		prCountsCopy := *exportData.PRs
		sourceDataCopy.PRs = &prCountsCopy
	}

	mv.SourceData = &sourceDataCopy
}

// ValidateFromExport performs validation against target using pre-loaded source data from export
func (mv *MigrationValidator) ValidateFromExport(targetOwner, targetRepo string) ([]ValidationResult, error) {
	// Validate that source data is already loaded
	if mv.SourceData == nil || mv.SourceData.Owner == "" || mv.SourceData.Name == "" {
		return nil, fmt.Errorf("source data not properly loaded - call SetSourceDataFromExport with valid data first")
	}

	// Normalize source data to prevent nil pointer dereferences
	if mv.SourceData.PRs == nil {
		mv.SourceData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	}

	// Validate access to target repository before starting
	fmt.Println("Validating repository access...")
	if err := mv.api.ValidateRepoAccess(api.TargetClient, targetOwner, targetRepo); err != nil {
		return nil, fmt.Errorf("cannot access target repository %s/%s: %w", targetOwner, targetRepo, err)
	}

	// Check rate limits before starting - warn if low
	mv.checkAndWarnRateLimits()

	fmt.Println("Starting migration validation from export...")
	fmt.Printf("Source: %s/%s (from export) | Target: %s/%s\n",
		mv.SourceData.Owner, mv.SourceData.Name, targetOwner, targetRepo)

	// Create a spinner for target data retrieval
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching target data from %s/%s...", targetOwner, targetRepo))

	// Retrieve target data using existing functionality
	errorMsgs, err := mv.retrieveTarget(targetOwner, targetRepo, spinner)

	// Log any API errors (safe to call after spinner finishes)
	output.LogAPIErrors(errorMsgs, targetOwner, targetRepo, err)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve target data: %w", err)
	}

	// Compare and validate the data (same as ValidateMigration)
	fmt.Println("\nValidating migration data...")
	results := mv.validateRepositoryData()

	fmt.Println("Migration validation completed!")
	return results, nil
}

// retrieveTarget retrieves all repository data from the target repository.
// Handles individual API failures gracefully by logging errors and continuing with default values.
// Returns a slice of error messages for display after spinners finish, and an error if all requests failed.
// An empty slice indicates all requests succeeded; callers should only expect error messages when
// partial failures occur (some requests succeeded, some failed).
func (mv *MigrationValidator) retrieveTarget(owner, name string, spinner *pterm.SpinnerPrinter) ([]string, error) {
	startTime := time.Now()
	var failedRequests []string
	var errorMessages []string
	var successfulRequests int

	mv.TargetData.Owner = owner
	mv.TargetData.Name = name

	// Get issue count
	spinner.UpdateText(fmt.Sprintf("Fetching issues from %s/%s...", owner, name))
	issues, err := mv.api.GetIssueCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "issues")
		errorMessages = append(errorMessages, fmt.Sprintf("issues: %v", err))
		mv.TargetData.Issues = 0
	} else {
		mv.TargetData.Issues = issues
		successfulRequests++
	}

	// Get PR counts
	spinner.UpdateText(fmt.Sprintf("Fetching pull requests from %s/%s...", owner, name))
	prCounts, err := mv.api.GetPRCounts(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "pull requests")
		errorMessages = append(errorMessages, fmt.Sprintf("pull requests: %v", err))
		mv.TargetData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	} else {
		mv.TargetData.PRs = prCounts
		successfulRequests++
	}

	// Get tag count
	spinner.UpdateText(fmt.Sprintf("Fetching tags from %s/%s...", owner, name))
	tags, err := mv.api.GetTagCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "tags")
		errorMessages = append(errorMessages, fmt.Sprintf("tags: %v", err))
		mv.TargetData.Tags = 0
	} else {
		mv.TargetData.Tags = tags
		successfulRequests++
	}

	// Get release count
	spinner.UpdateText(fmt.Sprintf("Fetching releases from %s/%s...", owner, name))
	releases, err := mv.api.GetReleaseCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "releases")
		errorMessages = append(errorMessages, fmt.Sprintf("releases: %v", err))
		mv.TargetData.Releases = 0
	} else {
		mv.TargetData.Releases = releases
		successfulRequests++
	}

	// Get commit count
	spinner.UpdateText(fmt.Sprintf("Fetching commit count from %s/%s...", owner, name))
	commitCount, err := mv.api.GetCommitCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "commits")
		errorMessages = append(errorMessages, fmt.Sprintf("commits: %v", err))
		mv.TargetData.CommitCount = 0
	} else {
		mv.TargetData.CommitCount = commitCount
		successfulRequests++
	}

	// Get latest commit hash
	spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", owner, name))
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "latest commit hash")
		errorMessages = append(errorMessages, fmt.Sprintf("latest commit hash: %v", err))
		mv.TargetData.LatestCommitSHA = ""
	} else {
		mv.TargetData.LatestCommitSHA = latestCommitSHA
		successfulRequests++
	}

	// Get branch protection rules count
	spinner.UpdateText(fmt.Sprintf("Fetching branch protection rules from %s/%s...", owner, name))
	branchProtectionRules, err := mv.api.GetBranchProtectionRulesCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "branch protection rules")
		errorMessages = append(errorMessages, fmt.Sprintf("branch protection rules: %v", err))
		mv.TargetData.BranchProtectionRules = 0
	} else {
		mv.TargetData.BranchProtectionRules = branchProtectionRules
		successfulRequests++
	}

	// Get webhook count
	spinner.UpdateText(fmt.Sprintf("Fetching webhooks from %s/%s...", owner, name))
	webhooks, err := mv.api.GetWebhookCount(api.TargetClient, owner, name)
	if err != nil {
		failedRequests = append(failedRequests, "webhooks")
		errorMessages = append(errorMessages, fmt.Sprintf("webhooks: %v", err))
		mv.TargetData.Webhooks = 0
	} else {
		mv.TargetData.Webhooks = webhooks
		successfulRequests++
	}

	duration := time.Since(startTime)

	// Determine success/failure status
	if successfulRequests == 0 {
		spinner.Fail(fmt.Sprintf("Failed to retrieve any data from %s/%s", owner, name))
		return errorMessages, fmt.Errorf("all API requests failed for %s/%s", owner, name)
	}

	if len(failedRequests) > 0 {
		spinner.Warning(fmt.Sprintf("%s/%s: %d OK, %d failed (%v) - missing: %v",
			owner, name, successfulRequests, len(failedRequests), duration, failedRequests))
	} else {
		spinner.Success(fmt.Sprintf("%s/%s retrieved successfully (%v)", owner, name, duration))
	}

	return errorMessages, nil
}

// validateRepositoryData compares source and target repository data and returns validation results
func (mv *MigrationValidator) validateRepositoryData() []ValidationResult {
	fmt.Println("Comparing repository data...")

	var results []ValidationResult

	// Compare Issues (target should have source issues + migration log issue)
	expectedTargetIssues := mv.SourceData.Issues + MigrationLogIssueOffset
	issueDiff := expectedTargetIssues - mv.TargetData.Issues
	issueStatus, issueStatusType := getValidationStatus(issueDiff)

	results = append(results, ValidationResult{
		Metric:     "Issues (expected +1 for migration log)",
		SourceVal:  mv.SourceData.Issues,
		TargetVal:  mv.TargetData.Issues,
		Status:     issueStatus,
		StatusType: issueStatusType,
		Difference: issueDiff,
	})

	// Compare Total PRs
	prDiff := mv.SourceData.PRs.Total - mv.TargetData.PRs.Total
	prStatus, prStatusType := getValidationStatus(prDiff)

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Total)",
		SourceVal:  mv.SourceData.PRs.Total,
		TargetVal:  mv.TargetData.PRs.Total,
		Status:     prStatus,
		StatusType: prStatusType,
		Difference: prDiff,
	})

	// Compare Open PRs
	openPRDiff := mv.SourceData.PRs.Open - mv.TargetData.PRs.Open
	openPRStatus, openPRStatusType := getValidationStatus(openPRDiff)

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Open)",
		SourceVal:  mv.SourceData.PRs.Open,
		TargetVal:  mv.TargetData.PRs.Open,
		Status:     openPRStatus,
		StatusType: openPRStatusType,
		Difference: openPRDiff,
	})

	// Compare Merged PRs
	mergedPRDiff := mv.SourceData.PRs.Merged - mv.TargetData.PRs.Merged
	mergedPRStatus, mergedPRStatusType := getValidationStatus(mergedPRDiff)

	results = append(results, ValidationResult{
		Metric:     "Pull Requests (Merged)",
		SourceVal:  mv.SourceData.PRs.Merged,
		TargetVal:  mv.TargetData.PRs.Merged,
		Status:     mergedPRStatus,
		StatusType: mergedPRStatusType,
		Difference: mergedPRDiff,
	})

	// Compare Tags
	tagDiff := mv.SourceData.Tags - mv.TargetData.Tags
	tagStatus, tagStatusType := getValidationStatus(tagDiff)

	results = append(results, ValidationResult{
		Metric:     "Tags",
		SourceVal:  mv.SourceData.Tags,
		TargetVal:  mv.TargetData.Tags,
		Status:     tagStatus,
		StatusType: tagStatusType,
		Difference: tagDiff,
	})

	// Compare Releases
	releaseDiff := mv.SourceData.Releases - mv.TargetData.Releases
	releaseStatus, releaseStatusType := getValidationStatus(releaseDiff)

	results = append(results, ValidationResult{
		Metric:     "Releases",
		SourceVal:  mv.SourceData.Releases,
		TargetVal:  mv.TargetData.Releases,
		Status:     releaseStatus,
		StatusType: releaseStatusType,
		Difference: releaseDiff,
	})

	// Compare Commit Count
	commitDiff := mv.SourceData.CommitCount - mv.TargetData.CommitCount
	commitStatus, commitStatusType := getValidationStatus(commitDiff)

	results = append(results, ValidationResult{
		Metric:     "Commits",
		SourceVal:  mv.SourceData.CommitCount,
		TargetVal:  mv.TargetData.CommitCount,
		Status:     commitStatus,
		StatusType: commitStatusType,
		Difference: commitDiff,
	})

	// Compare Branch Protection Rules
	branchProtectionDiff := mv.SourceData.BranchProtectionRules - mv.TargetData.BranchProtectionRules
	branchProtectionStatus, branchProtectionStatusType := getValidationStatus(branchProtectionDiff)

	results = append(results, ValidationResult{
		Metric:     "Branch Protection Rules",
		SourceVal:  mv.SourceData.BranchProtectionRules,
		TargetVal:  mv.TargetData.BranchProtectionRules,
		Status:     branchProtectionStatus,
		StatusType: branchProtectionStatusType,
		Difference: branchProtectionDiff,
	})

	// Compare Webhooks
	webhooksDiff := mv.SourceData.Webhooks - mv.TargetData.Webhooks
	webhooksStatus, webhooksStatusType := getValidationStatus(webhooksDiff)

	results = append(results, ValidationResult{
		Metric:     "Webhooks",
		SourceVal:  mv.SourceData.Webhooks,
		TargetVal:  mv.TargetData.Webhooks,
		Status:     webhooksStatus,
		StatusType: webhooksStatusType,
		Difference: webhooksDiff,
	})

	// Compare Latest Commit SHA
	latestCommitStatus := ValidationStatusMessagePass
	latestCommitStatusType := ValidationStatusPass

	if mv.SourceData.LatestCommitSHA != mv.TargetData.LatestCommitSHA {
		latestCommitStatus = ValidationStatusMessageFail
		latestCommitStatusType = ValidationStatusFail
	}

	results = append(results, ValidationResult{
		Metric:     "Latest Commit SHA",
		SourceVal:  mv.SourceData.LatestCommitSHA,
		TargetVal:  mv.TargetData.LatestCommitSHA,
		Status:     latestCommitStatus,
		StatusType: latestCommitStatusType,
		Difference: 0, // Not applicable for SHA comparison
	})

	// Add migration archive validation if available
	if mv.SourceData.MigrationArchive != nil {
		// First, compare migration archive with source API data to check migration completeness
		archiveVsSourceIssuesDiff := mv.SourceData.MigrationArchive.Issues - mv.SourceData.Issues
		archiveVsSourceIssuesStatus, archiveVsSourceIssuesStatusType := getValidationStatus(archiveVsSourceIssuesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Source Issues",
			SourceVal:  mv.SourceData.Issues,
			TargetVal:  mv.SourceData.MigrationArchive.Issues,
			Status:     archiveVsSourceIssuesStatus,
			StatusType: archiveVsSourceIssuesStatusType,
			Difference: archiveVsSourceIssuesDiff,
		})

		archiveVsSourcePRsDiff := mv.SourceData.MigrationArchive.PullRequests - mv.SourceData.PRs.Total
		archiveVsSourcePRsStatus, archiveVsSourcePRsStatusType := getValidationStatus(archiveVsSourcePRsDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Source Pull Requests",
			SourceVal:  mv.SourceData.PRs.Total,
			TargetVal:  mv.SourceData.MigrationArchive.PullRequests,
			Status:     archiveVsSourcePRsStatus,
			StatusType: archiveVsSourcePRsStatusType,
			Difference: archiveVsSourcePRsDiff,
		})

		archiveVsSourceBranchesDiff := mv.SourceData.MigrationArchive.ProtectedBranches - mv.SourceData.BranchProtectionRules
		archiveVsSourceBranchesStatus, archiveVsSourceBranchesStatusType := getValidationStatus(archiveVsSourceBranchesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Source Protected Branches",
			SourceVal:  mv.SourceData.BranchProtectionRules,
			TargetVal:  mv.SourceData.MigrationArchive.ProtectedBranches,
			Status:     archiveVsSourceBranchesStatus,
			StatusType: archiveVsSourceBranchesStatusType,
			Difference: archiveVsSourceBranchesDiff,
		})

		archiveVsSourceReleasesDiff := mv.SourceData.MigrationArchive.Releases - mv.SourceData.Releases
		archiveVsSourceReleasesStatus, archiveVsSourceReleasesStatusType := getValidationStatus(archiveVsSourceReleasesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Source Releases",
			SourceVal:  mv.SourceData.Releases,
			TargetVal:  mv.SourceData.MigrationArchive.Releases,
			Status:     archiveVsSourceReleasesStatus,
			StatusType: archiveVsSourceReleasesStatusType,
			Difference: archiveVsSourceReleasesDiff,
		})

		// Then, compare migration archive with target data to check migration success
		expectedTargetFromArchive := mv.SourceData.MigrationArchive.Issues + MigrationLogIssueOffset
		archiveToTargetIssuesDiff := expectedTargetFromArchive - mv.TargetData.Issues
		archiveToTargetIssuesStatus, archiveToTargetIssuesStatusType := getValidationStatus(archiveToTargetIssuesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Target Issues (expected +1 for migration log)",
			SourceVal:  mv.SourceData.MigrationArchive.Issues,
			TargetVal:  mv.TargetData.Issues,
			Status:     archiveToTargetIssuesStatus,
			StatusType: archiveToTargetIssuesStatusType,
			Difference: archiveToTargetIssuesDiff,
		})

		archiveToTargetPRsDiff := mv.SourceData.MigrationArchive.PullRequests - mv.TargetData.PRs.Total
		archiveToTargetPRsStatus, archiveToTargetPRsStatusType := getValidationStatus(archiveToTargetPRsDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Target Pull Requests",
			SourceVal:  mv.SourceData.MigrationArchive.PullRequests,
			TargetVal:  mv.TargetData.PRs.Total,
			Status:     archiveToTargetPRsStatus,
			StatusType: archiveToTargetPRsStatusType,
			Difference: archiveToTargetPRsDiff,
		})

		archiveToTargetBranchesDiff := mv.SourceData.MigrationArchive.ProtectedBranches - mv.TargetData.BranchProtectionRules
		archiveToTargetBranchesStatus, archiveToTargetBranchesStatusType := getValidationStatus(archiveToTargetBranchesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Target Protected Branches",
			SourceVal:  mv.SourceData.MigrationArchive.ProtectedBranches,
			TargetVal:  mv.TargetData.BranchProtectionRules,
			Status:     archiveToTargetBranchesStatus,
			StatusType: archiveToTargetBranchesStatusType,
			Difference: archiveToTargetBranchesDiff,
		})

		archiveToTargetReleasesDiff := mv.SourceData.MigrationArchive.Releases - mv.TargetData.Releases
		archiveToTargetReleasesStatus, archiveToTargetReleasesStatusType := getValidationStatus(archiveToTargetReleasesDiff)

		results = append(results, ValidationResult{
			Metric:     "Archive vs Target Releases",
			SourceVal:  mv.SourceData.MigrationArchive.Releases,
			TargetVal:  mv.TargetData.Releases,
			Status:     archiveToTargetReleasesStatus,
			StatusType: archiveToTargetReleasesStatusType,
			Difference: archiveToTargetReleasesDiff,
		})
	}

	return results
}

// PrintValidationResults prints a formatted report of the validation results
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

	// Separate results into different categories
	var standardResults []ValidationResult
	var archiveVsSourceResults []ValidationResult
	var archiveVsTargetResults []ValidationResult

	for _, result := range results {
		if strings.HasPrefix(result.Metric, "Archive vs Source") {
			archiveVsSourceResults = append(archiveVsSourceResults, result)
		} else if strings.HasPrefix(result.Metric, "Archive vs Target") {
			archiveVsTargetResults = append(archiveVsTargetResults, result)
		} else {
			standardResults = append(standardResults, result)
		}
	}

	// Display standard validation table
	mv.displayValidationTable("🔄 Source vs Target Validation", standardResults)

	// Display migration archive validation tables if available
	if len(archiveVsSourceResults) > 0 {
		fmt.Println()
		mv.displayValidationTable("📦 Migration Archive vs Source Validation", archiveVsSourceResults)
	}

	if len(archiveVsTargetResults) > 0 {
		fmt.Println()
		mv.displayValidationTable("🎯 Migration Archive vs Target Validation", archiveVsTargetResults)
	}

	fmt.Println() // Add spacing

	// Calculate and display summary for all results
	mv.displayValidationSummary(results)
}

// displayValidationTable displays a validation table with the given title and results
func (mv *MigrationValidator) displayValidationTable(title string, results []ValidationResult) {
	if len(results) == 0 {
		return
	}

	// Print section title
	pterm.DefaultSection.Println(title)

	// Determine appropriate headers based on the validation type
	var headers []string
	if strings.Contains(title, "Archive vs Source") {
		headers = []string{"Metric", "Status", "Source API Value", "Archive Value", "Difference"}
	} else if strings.Contains(title, "Archive vs Target") {
		headers = []string{"Metric", "Status", "Archive Value", "Target Value", "Difference"}
	} else {
		headers = []string{"Metric", "Status", "Source Value", "Target Value", "Difference"}
	}

	// Create table data
	tableData := [][]string{headers}

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
}

// displayValidationSummary calculates and displays the overall validation summary
func (mv *MigrationValidator) displayValidationSummary(results []ValidationResult) {
	// Calculate summary
	passCount := 0
	failCount := 0
	warnCount := 0

	for _, result := range results {
		switch result.StatusType {
		case ValidationStatusPass:
			passCount++
		case ValidationStatusFail:
			failCount++
		case ValidationStatusWarn:
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
	mv.outputMarkdownResults(results)
}

type markdownOutputOptions struct {
	writer           io.Writer
	includeCodeFence bool
	announce         bool
}

// printMarkdownTable prints a markdown-formatted table for easy copy/paste
func (mv *MigrationValidator) printMarkdownTable(results []ValidationResult, opts markdownOutputOptions) {
	writer := opts.writer
	if writer == nil {
		writer = os.Stdout
	}

	if opts.announce {
		pterm.DefaultSection.Println("📋 Markdown Table (Copy-Paste Ready)")
	}

	if opts.includeCodeFence {
		fmt.Fprintln(writer, "```markdown")
	}

	fmt.Fprintln(writer, "# Migration Validation Report\n")
	fmt.Fprintf(writer, "**Source:** `%s/%s`  \n", mv.SourceData.Owner, mv.SourceData.Name)
	fmt.Fprintf(writer, "**Target:** `%s/%s`  \n\n", mv.TargetData.Owner, mv.TargetData.Name)

	fmt.Fprintln(writer, "| Metric | Status | Source Value | Target Value | Difference |")
	fmt.Fprintln(writer, "|--------|--------|--------------|--------------|------------|")

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

		fmt.Fprintf(writer, "| %s | %s | %v | %v | %s |\n",
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
		switch result.StatusType {
		case ValidationStatusPass:
			passCount++
		case ValidationStatusFail:
			failCount++
		case ValidationStatusWarn:
			warnCount++
		}
	}

	fmt.Fprintln(writer, "\n## Summary\n")
	fmt.Fprintf(writer, "- **Passed:** %d  \n", passCount)
	fmt.Fprintf(writer, "- **Failed:** %d  \n", failCount)
	fmt.Fprintf(writer, "- **Warnings:** %d  \n\n", warnCount)

	if failCount > 0 {
		fmt.Fprintln(writer, "**Result:** ❌ Migration validation FAILED - Some data is missing in target")
	} else if warnCount > 0 {
		fmt.Fprintln(writer, "**Result:** ⚠️ Migration validation completed with WARNINGS - Target has more data than source")
	} else {
		fmt.Fprintln(writer, "**Result:** ✅ Migration validation PASSED - All data matches!")
	}

	if opts.includeCodeFence {
		fmt.Fprintln(writer, "```")
		if opts.announce {
			pterm.Info.Println("💡 Tip: You can select and copy the entire markdown section above to paste into documentation, issues, or pull requests!")
		}
	}
}

func (mv *MigrationValidator) writeMarkdownToFile(results []ValidationResult, path string) error {
	var buffer bytes.Buffer
	mv.printMarkdownTable(results, markdownOutputOptions{writer: &buffer, includeCodeFence: false, announce: false})
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func (mv *MigrationValidator) outputMarkdownResults(results []ValidationResult) {
	markdownTable := viper.GetBool("MARKDOWN_TABLE")
	markdownFile := viper.GetString("MARKDOWN_FILE")

	if markdownTable {
		mv.printMarkdownTable(results, markdownOutputOptions{writer: os.Stdout, includeCodeFence: true, announce: true})
	}

	if markdownFile == "" {
		return
	}

	if err := mv.writeMarkdownToFile(results, markdownFile); err != nil {
		pterm.Error.Printf("Failed to write markdown file %s: %v\n", markdownFile, err)
		return
	}

	pterm.Success.Printf("📁 Markdown report saved to %s\n", markdownFile)
}

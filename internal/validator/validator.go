package validator

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/migrationarchive"
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

	// Channel to synchronize goroutines
	wg.Add(2)

	// Retrieve source repository data in a goroutine
	go func() {
		defer wg.Done()
		sourceErr = mv.retrieveSource(sourceOwner, sourceRepo, sourceSpinner)
	}()

	// Retrieve target repository data in a goroutine
	go func() {
		defer wg.Done()
		targetErr = mv.retrieveTarget(targetOwner, targetRepo, targetSpinner)
	}()

	// Wait for both goroutines to complete
	wg.Wait()

	// Stop the multi printer
	multi.Stop()

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

// retrieveSource retrieves all repository data from the source repository
// Handles individual API failures gracefully by logging errors and continuing with default values
func (mv *MigrationValidator) retrieveSource(owner, name string, spinner *pterm.SpinnerPrinter) error {
	startTime := time.Now()
	var failedRequests []string
	var successfulRequests int

	mv.SourceData.Owner = owner
	mv.SourceData.Name = name

	// Get issue count
	spinner.UpdateText(fmt.Sprintf("Fetching issues from %s/%s...", owner, name))
	issues, err := mv.api.GetIssueCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch issues from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "issues")
		mv.SourceData.Issues = 0
	} else {
		mv.SourceData.Issues = issues
		successfulRequests++
	}

	// Get PR counts
	spinner.UpdateText(fmt.Sprintf("Fetching pull requests from %s/%s...", owner, name))
	prCounts, err := mv.api.GetPRCounts(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch pull requests from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "pull requests")
		mv.SourceData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	} else {
		mv.SourceData.PRs = prCounts
		successfulRequests++
	}

	// Get tag count
	spinner.UpdateText(fmt.Sprintf("Fetching tags from %s/%s...", owner, name))
	tags, err := mv.api.GetTagCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch tags from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "tags")
		mv.SourceData.Tags = 0
	} else {
		mv.SourceData.Tags = tags
		successfulRequests++
	}

	// Get release count
	spinner.UpdateText(fmt.Sprintf("Fetching releases from %s/%s...", owner, name))
	releases, err := mv.api.GetReleaseCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch releases from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "releases")
		mv.SourceData.Releases = 0
	} else {
		mv.SourceData.Releases = releases
		successfulRequests++
	}

	// Get commit count
	spinner.UpdateText(fmt.Sprintf("Fetching commit count from %s/%s...", owner, name))
	commitCount, err := mv.api.GetCommitCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch commit count from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "commits")
		mv.SourceData.CommitCount = 0
	} else {
		mv.SourceData.CommitCount = commitCount
		successfulRequests++
	}

	// Get latest commit hash
	spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", owner, name))
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch latest commit hash from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "latest commit hash")
		mv.SourceData.LatestCommitSHA = ""
	} else {
		mv.SourceData.LatestCommitSHA = latestCommitSHA
		successfulRequests++
	}

	// Get branch protection rules count
	spinner.UpdateText(fmt.Sprintf("Fetching branch protection rules from %s/%s...", owner, name))
	branchProtectionRules, err := mv.api.GetBranchProtectionRulesCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch branch protection rules from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "branch protection rules")
		mv.SourceData.BranchProtectionRules = 0
	} else {
		mv.SourceData.BranchProtectionRules = branchProtectionRules
		successfulRequests++
	}

	// Get webhook count
	spinner.UpdateText(fmt.Sprintf("Fetching webhooks from %s/%s...", owner, name))
	webhooks, err := mv.api.GetWebhookCount(api.SourceClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch webhooks from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "webhooks")
		mv.SourceData.Webhooks = 0
	} else {
		mv.SourceData.Webhooks = webhooks
		successfulRequests++
	}

	duration := time.Since(startTime)

	// Determine success/failure status
	if successfulRequests == 0 {
		spinner.Fail(fmt.Sprintf("Failed to retrieve any data from %s/%s", owner, name))
		return fmt.Errorf("all API requests failed for %s/%s", owner, name)
	}

	if len(failedRequests) > 0 {
		spinner.Warning(fmt.Sprintf("%s/%s retrieved with %d successful and %d failed requests (%v)",
			owner, name, successfulRequests, len(failedRequests), duration))
		pterm.Warning.Printf("Failed to retrieve: %v\n", failedRequests)
		pterm.Info.Println("Export will continue with available data (failed requests will have default values)")
	} else {
		spinner.Success(fmt.Sprintf("%s/%s retrieved successfully (%v)", owner, name, duration))
	}

	return nil
}

// RetrieveSourceData is a public wrapper for retrieveSource for use by the export package
func (mv *MigrationValidator) RetrieveSourceData(owner, name string, spinner *pterm.SpinnerPrinter) error {
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

	fmt.Println("Starting migration validation from export...")
	fmt.Printf("Source: %s/%s (from export) | Target: %s/%s\n",
		mv.SourceData.Owner, mv.SourceData.Name, targetOwner, targetRepo)

	// Create a spinner for target data retrieval
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching target data from %s/%s...", targetOwner, targetRepo))

	// Retrieve target data using existing functionality
	err := mv.retrieveTarget(targetOwner, targetRepo, spinner)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Failed to fetch target data from %s/%s", targetOwner, targetRepo))
		return nil, fmt.Errorf("failed to retrieve target data: %w", err)
		//fail spinner
	}

	// Compare and validate the data (same as ValidateMigration)
	fmt.Println("\nValidating migration data...")
	results := mv.validateRepositoryData()

	fmt.Println("Migration validation completed!")
	return results, nil
}

// retrieveTarget retrieves all repository data from the target repository
// Handles individual API failures gracefully by logging errors and continuing with default values
func (mv *MigrationValidator) retrieveTarget(owner, name string, spinner *pterm.SpinnerPrinter) error {
	startTime := time.Now()
	var failedRequests []string
	var successfulRequests int

	mv.TargetData.Owner = owner
	mv.TargetData.Name = name

	// Get issue count
	spinner.UpdateText(fmt.Sprintf("Fetching issues from %s/%s...", owner, name))
	issues, err := mv.api.GetIssueCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch issues from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "issues")
		mv.TargetData.Issues = 0
	} else {
		mv.TargetData.Issues = issues
		successfulRequests++
	}

	// Get PR counts
	spinner.UpdateText(fmt.Sprintf("Fetching pull requests from %s/%s...", owner, name))
	prCounts, err := mv.api.GetPRCounts(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch pull requests from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "pull requests")
		mv.TargetData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	} else {
		mv.TargetData.PRs = prCounts
		successfulRequests++
	}

	// Get tag count
	spinner.UpdateText(fmt.Sprintf("Fetching tags from %s/%s...", owner, name))
	tags, err := mv.api.GetTagCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch tags from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "tags")
		mv.TargetData.Tags = 0
	} else {
		mv.TargetData.Tags = tags
		successfulRequests++
	}

	// Get release count
	spinner.UpdateText(fmt.Sprintf("Fetching releases from %s/%s...", owner, name))
	releases, err := mv.api.GetReleaseCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch releases from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "releases")
		mv.TargetData.Releases = 0
	} else {
		mv.TargetData.Releases = releases
		successfulRequests++
	}

	// Get commit count
	spinner.UpdateText(fmt.Sprintf("Fetching commit count from %s/%s...", owner, name))
	commitCount, err := mv.api.GetCommitCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch commit count from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "commits")
		mv.TargetData.CommitCount = 0
	} else {
		mv.TargetData.CommitCount = commitCount
		successfulRequests++
	}

	// Get latest commit hash
	spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", owner, name))
	latestCommitSHA, err := mv.api.GetLatestCommitHash(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch latest commit hash from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "latest commit hash")
		mv.TargetData.LatestCommitSHA = ""
	} else {
		mv.TargetData.LatestCommitSHA = latestCommitSHA
		successfulRequests++
	}

	// Get branch protection rules count
	spinner.UpdateText(fmt.Sprintf("Fetching branch protection rules from %s/%s...", owner, name))
	branchProtectionRules, err := mv.api.GetBranchProtectionRulesCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch branch protection rules from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "branch protection rules")
		mv.TargetData.BranchProtectionRules = 0
	} else {
		mv.TargetData.BranchProtectionRules = branchProtectionRules
		successfulRequests++
	}

	// Get webhook count
	spinner.UpdateText(fmt.Sprintf("Fetching webhooks from %s/%s...", owner, name))
	webhooks, err := mv.api.GetWebhookCount(api.TargetClient, owner, name)
	if err != nil {
		pterm.Error.Printf("Failed to fetch webhooks from %s/%s: %v\n", owner, name, err)
		failedRequests = append(failedRequests, "webhooks")
		mv.TargetData.Webhooks = 0
	} else {
		mv.TargetData.Webhooks = webhooks
		successfulRequests++
	}

	duration := time.Since(startTime)

	// Determine success/failure status
	if successfulRequests == 0 {
		spinner.Fail(fmt.Sprintf("Failed to retrieve any data from %s/%s", owner, name))
		return fmt.Errorf("all API requests failed for %s/%s", owner, name)
	}

	if len(failedRequests) > 0 {
		spinner.Warning(fmt.Sprintf("%s/%s retrieved with %d successful and %d failed requests (%v)",
			owner, name, successfulRequests, len(failedRequests), duration))
		pterm.Warning.Printf("Failed to retrieve: %v\n", failedRequests)
		pterm.Info.Println("Validation will continue with available data (failed requests will have default values)")
	} else {
		spinner.Success(fmt.Sprintf("%s/%s retrieved successfully (%v)", owner, name, duration))
	}

	return nil
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
	if viper.GetBool("MARKDOWN_TABLE") {
		mv.printMarkdownTable(results)
	}
}

// printMarkdownTable prints a markdown-formatted table for easy copy/paste
func (mv *MigrationValidator) printMarkdownTable(results []ValidationResult) {
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
		switch result.StatusType {
		case ValidationStatusPass:
			passCount++
		case ValidationStatusFail:
			failCount++
		case ValidationStatusWarn:
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

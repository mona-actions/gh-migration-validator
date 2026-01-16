package validator

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"

	"mona-actions/gh-migration-validator/internal/api"
)

// expectedValidationMetrics defines all the metrics that should be validated
// This eliminates magic numbers in tests and ensures consistency when validation dimensions change
var expectedValidationMetrics = []string{
	"Issues (expected +1 for migration log)",
	"Pull Requests (Total)",
	"Pull Requests (Open)",
	"Pull Requests (Merged)",
	"Tags",
	"Releases",
	"Commits",
	"Branch Protection Rules",
	"Webhooks",
	"Latest Commit SHA",
}

// setupTestValidator creates a validator with test data for validation testing
func setupTestValidator(sourceData, targetData *RepositoryData) *MigrationValidator {
	return &MigrationValidator{
		api:        nil, // Not needed for validation logic tests
		SourceData: sourceData,
		TargetData: targetData,
	}
}

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// validateMetricNames is a helper function to verify that validation results contain expected metrics
func validateMetricNames(t *testing.T, results []ValidationResult) {
	t.Helper() // Mark this as a test helper

	// Should have expected number of results
	assert.Equal(t, len(expectedValidationMetrics), len(results),
		"Should return expected number of validation results")

	// Verify metrics are in expected order with expected names
	for i, expectedMetric := range expectedValidationMetrics {
		assert.Equal(t, expectedMetric, results[i].Metric,
			"Metric at index %d should be '%s'", i, expectedMetric)
	}
}

func TestValidateRepositoryData_PerfectMatch(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:                 "source-org",
		Name:                  "test-repo",
		Issues:                10,
		PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:                  3,
		Releases:              2,
		CommitCount:           100,
		LatestCommitSHA:       "abc123",
		BranchProtectionRules: 4,
		Webhooks:              2,
	}

	targetData := &RepositoryData{
		Owner:                 "target-org",
		Name:                  "test-repo",
		Issues:                11, // Expected: source + 1 for migration log
		PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:                  3,
		Releases:              2,
		CommitCount:           100,
		LatestCommitSHA:       "abc123",
		BranchProtectionRules: 4,
		Webhooks:              2,
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Validate metric names and count
	validateMetricNames(t, results)

	// Check that all results pass
	passCount := 0
	for _, result := range results {
		if result.StatusType == ValidationStatusPass {
			passCount++
		}
	}
	assert.Equal(t, len(expectedValidationMetrics), passCount, "All validations should pass for perfect match") // Verify specific results
	issueResult := results[0]
	assert.Equal(t, "Issues (expected +1 for migration log)", issueResult.Metric)
	assert.Equal(t, ValidationStatusPass, issueResult.StatusType)
	assert.Equal(t, 0, issueResult.Difference)

	// Verify PR results
	prResult := results[1]
	assert.Equal(t, "Pull Requests (Total)", prResult.Metric)
	assert.Equal(t, ValidationStatusPass, prResult.StatusType)
	assert.Equal(t, 5, prResult.SourceVal)
	assert.Equal(t, 5, prResult.TargetVal)

	// Verify commit SHA result
	var shaResult *ValidationResult
	for i := range results {
		if results[i].Metric == "Latest Commit SHA" {
			shaResult = &results[i]
			break
		}
	}
	if assert.NotNil(t, shaResult, "Should find commit SHA validation result") {
		assert.Equal(t, "Latest Commit SHA", shaResult.Metric)
		assert.Equal(t, ValidationStatusPass, shaResult.StatusType)
		assert.Equal(t, "abc123", shaResult.SourceVal)
		assert.Equal(t, "abc123", shaResult.TargetVal)
	}
}

func TestValidateRepositoryData_MissingData(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:                 "source-org",
		Name:                  "test-repo",
		Issues:                10,
		PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:                  3,
		Releases:              2,
		CommitCount:           100,
		LatestCommitSHA:       "abc123",
		BranchProtectionRules: 4,
		Webhooks:              3,
	}

	targetData := &RepositoryData{
		Owner:                 "target-org",
		Name:                  "test-repo",
		Issues:                8,                                                      // Missing 3 (should be 11, but is 8)
		PRs:                   &api.PRCounts{Total: 3, Open: 1, Merged: 1, Closed: 1}, // Missing 2 total PRs
		Tags:                  2,                                                      // Missing 1 tag
		Releases:              1,                                                      // Missing 1 release
		CommitCount:           90,                                                     // Missing 10 commits
		LatestCommitSHA:       "def456",                                               // Different commit SHA
		BranchProtectionRules: 3,                                                      // Missing 1 rule
		Webhooks:              1,                                                      // Missing 2 webhooks
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Count statuses
	failCount := 0
	for _, result := range results {
		if result.StatusType == ValidationStatusFail {
			failCount++
		}
	}
	assert.Equal(t, len(expectedValidationMetrics), failCount, "Should have expected number of failures for missing data")

	// Check issues validation
	issueResult := results[0]
	assert.Equal(t, ValidationStatusFail, issueResult.StatusType)
	assert.Equal(t, 3, issueResult.Difference) // Expected 11, got 8

	// Check PR validation
	prResult := results[1]
	assert.Equal(t, ValidationStatusFail, prResult.StatusType)
	assert.Equal(t, 2, prResult.Difference) // Expected 5, got 3

	// Check commit SHA validation
	var shaResult *ValidationResult
	for i := range results {
		if results[i].Metric == "Latest Commit SHA" {
			shaResult = &results[i]
			break
		}
	}
	if assert.NotNil(t, shaResult, "Should find commit SHA validation result") {
		assert.Equal(t, ValidationStatusFail, shaResult.StatusType)
		assert.Equal(t, "abc123", shaResult.SourceVal)
		assert.Equal(t, "def456", shaResult.TargetVal)
	}
}

func TestValidateRepositoryData_ExtraData(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:                 "source-org",
		Name:                  "test-repo",
		Issues:                10,
		PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:                  3,
		Releases:              2,
		CommitCount:           100,
		LatestCommitSHA:       "abc123",
		BranchProtectionRules: 4,
		Webhooks:              2,
	}

	targetData := &RepositoryData{
		Owner:                 "target-org",
		Name:                  "test-repo",
		Issues:                13,                                                     // 2 extra (should be 11, but is 13)
		PRs:                   &api.PRCounts{Total: 7, Open: 3, Merged: 3, Closed: 1}, // 2 extra PRs
		Tags:                  5,                                                      // 2 extra tags
		Releases:              4,                                                      // 2 extra releases
		CommitCount:           110,                                                    // 10 extra commits
		LatestCommitSHA:       "abc123",                                               // Same commit SHA
		BranchProtectionRules: 6,                                                      // 2 extra rules
		Webhooks:              5,                                                      // 3 extra webhooks
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Count warnings and passes
	warnCount := 0
	passCount := 0
	for _, result := range results {
		if result.StatusType == ValidationStatusWarn {
			warnCount++
		} else if result.StatusType == ValidationStatusPass {
			passCount++
		}
	}
	assert.Equal(t, len(expectedValidationMetrics)-1, warnCount, "Should have warnings for extra data (except commit SHA)")
	assert.Equal(t, 1, passCount, "Should have 1 pass (commit SHA)")

	// Check issues validation (extra data)
	issueResult := results[0]
	assert.Equal(t, ValidationStatusWarn, issueResult.StatusType)
	assert.Equal(t, -2, issueResult.Difference) // Expected 11, got 13

	// Check commit SHA validation (should still pass)
	var shaResult *ValidationResult
	for i := range results {
		if results[i].Metric == "Latest Commit SHA" {
			shaResult = &results[i]
			break
		}
	}
	if assert.NotNil(t, shaResult, "Should find commit SHA validation result") {
		assert.Equal(t, ValidationStatusPass, shaResult.StatusType)
	}
}

func TestPrintValidationResults(t *testing.T) {
	// Disable pterm output for testing to avoid cluttering test output
	pterm.DisableOutput()
	defer pterm.EnableOutput()

	validator := setupTestValidator(
		&RepositoryData{
			Owner: "source-org",
			Name:  "test-repo",
		},
		&RepositoryData{
			Owner: "target-org",
			Name:  "test-repo",
		},
	)

	results := []ValidationResult{
		{
			Metric:     "Issues",
			SourceVal:  10,
			TargetVal:  11,
			Status:     ValidationStatusMessagePass,
			StatusType: ValidationStatusPass,
			Difference: 0,
		},
		{
			Metric:     "PRs",
			SourceVal:  5,
			TargetVal:  3,
			Status:     ValidationStatusMessageFail,
			StatusType: ValidationStatusFail,
			Difference: 2,
		},
		{
			Metric:     "Tags",
			SourceVal:  3,
			TargetVal:  5,
			Status:     ValidationStatusMessageWarn,
			StatusType: ValidationStatusWarn,
			Difference: -2,
		},
	}

	// This should run without panic and output the formatted results
	assert.NotPanics(t, func() {
		validator.PrintValidationResults(results)
	}, "PrintValidationResults should not panic")

	// Test that the function processes results correctly
	// We can't easily test the exact output due to pterm formatting,
	// but we can ensure it doesn't crash with various result combinations
}

func TestPrintMarkdownTable(t *testing.T) {
	validator := setupTestValidator(
		&RepositoryData{
			Owner: "source-org",
			Name:  "test-repo",
		},
		&RepositoryData{
			Owner: "target-org",
			Name:  "test-repo",
		},
	)

	results := []ValidationResult{
		{
			Metric:     "Issues",
			SourceVal:  10,
			TargetVal:  11,
			Status:     "✅ PASS",
			StatusType: ValidationStatusPass,
			Difference: 0,
		},
		{
			Metric:     "PRs",
			SourceVal:  5,
			TargetVal:  3,
			Status:     "❌ FAIL",
			StatusType: ValidationStatusFail,
			Difference: 2,
		},
		{
			Metric:     "Tags",
			SourceVal:  3,
			TargetVal:  5,
			Status:     "⚠️ WARN",
			StatusType: ValidationStatusWarn,
			Difference: -2,
		},
		{
			Metric:     "Latest Commit SHA",
			SourceVal:  "abc123",
			TargetVal:  "def456",
			Status:     "❌ FAIL",
			StatusType: ValidationStatusFail,
			Difference: 0,
		},
	}

	// Capture the markdown output
	output := captureOutput(func() {
		validator.printMarkdownTable(results)
	})

	// Verify the output contains expected markdown elements
	assert.Contains(t, output, "# Migration Validation Report", "Should contain report header")
	assert.Contains(t, output, "**Source:** `source-org/test-repo`", "Should contain source info")
	assert.Contains(t, output, "**Target:** `target-org/test-repo`", "Should contain target info")
	assert.Contains(t, output, "| Metric | Status | Source Value | Target Value | Difference |", "Should contain table header")
	assert.Contains(t, output, "|--------|--------|--------------|--------------|------------|", "Should contain table separator")

	// Check for specific result rows
	assert.Contains(t, output, "| Issues | ✅ PASS | 10 | 11 | Perfect match |", "Should contain issues row")
	assert.Contains(t, output, "| PRs | ❌ FAIL | 5 | 3 | Missing: 2 |", "Should contain PRs row")
	assert.Contains(t, output, "| Tags | ⚠️ WARN | 3 | 5 | Extra: 2 |", "Should contain tags row")
	assert.Contains(t, output, "| Latest Commit SHA | ❌ FAIL | abc123 | def456 | N/A |", "Should contain SHA row")

	// Check for summary section
	assert.Contains(t, output, "## Summary", "Should contain summary section")
	assert.Contains(t, output, "- **Passed:** 1", "Should count passed items")
	assert.Contains(t, output, "- **Failed:** 2", "Should count failed items")
	assert.Contains(t, output, "- **Warnings:** 1", "Should count warning items")
}

func TestValidationResult_DifferenceCalculation(t *testing.T) {
	tests := []struct {
		name               string
		sourceIssues       int
		targetIssues       int
		expectedStatusType ValidationStatus
		expectedDiff       int
	}{
		{
			name:               "Perfect issues match (+1 expected)",
			sourceIssues:       10,
			targetIssues:       11,
			expectedStatusType: ValidationStatusPass,
			expectedDiff:       0,
		},
		{
			name:               "Missing issues in target",
			sourceIssues:       10,
			targetIssues:       9,
			expectedStatusType: ValidationStatusFail,
			expectedDiff:       2, // Expected 11, got 9
		},
		{
			name:               "Extra issues in target",
			sourceIssues:       10,
			targetIssues:       13,
			expectedStatusType: ValidationStatusWarn,
			expectedDiff:       -2, // Expected 11, got 13
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupTestValidator(
				&RepositoryData{
					Issues: tt.sourceIssues,
					PRs:    &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
				},
				&RepositoryData{
					Issues: tt.targetIssues,
					PRs:    &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
				},
			)

			results := validator.validateRepositoryData()

			// Find the issues result
			var issueResult ValidationResult
			for _, result := range results {
				if result.Metric == "Issues (expected +1 for migration log)" {
					issueResult = result
					break
				}
			}

			assert.Equal(t, tt.expectedStatusType, issueResult.StatusType, "StatusType should match expected")
			assert.Equal(t, tt.expectedDiff, issueResult.Difference, "Difference should match expected")
		})
	}
}

func TestValidationResult_CommitSHAComparison(t *testing.T) {
	tests := []struct {
		name               string
		sourceSHA          string
		targetSHA          string
		expectedStatusType ValidationStatus
	}{
		{
			name:               "Matching commit SHAs",
			sourceSHA:          "abc123def456",
			targetSHA:          "abc123def456",
			expectedStatusType: ValidationStatusPass,
		},
		{
			name:               "Different commit SHAs",
			sourceSHA:          "abc123def456",
			targetSHA:          "xyz789uvw012",
			expectedStatusType: ValidationStatusFail,
		},
		{
			name:               "Empty source SHA",
			sourceSHA:          "",
			targetSHA:          "abc123def456",
			expectedStatusType: ValidationStatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupTestValidator(
				&RepositoryData{
					LatestCommitSHA: tt.sourceSHA,
					PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
				},
				&RepositoryData{
					LatestCommitSHA: tt.targetSHA,
					PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
				},
			)

			results := validator.validateRepositoryData()

			// Find the commit SHA result by metric name
			var shaResult *ValidationResult
			for i := range results {
				if results[i].Metric == "Latest Commit SHA" {
					shaResult = &results[i]
					break
				}
			}
			if assert.NotNil(t, shaResult, "Should find commit SHA validation result") {
				assert.Equal(t, "Latest Commit SHA", shaResult.Metric)
				assert.Equal(t, tt.expectedStatusType, shaResult.StatusType)
				assert.Equal(t, tt.sourceSHA, shaResult.SourceVal)
				assert.Equal(t, tt.targetSHA, shaResult.TargetVal)
				assert.Equal(t, 0, shaResult.Difference) // Always 0 for SHA comparison
			}
		})
	}
}

func TestValidateRepositoryData_MetricNames(t *testing.T) {
	// Test that validateRepositoryData returns exactly the expected metrics with correct names
	validator := setupTestValidator(
		&RepositoryData{
			Owner:           "source-org",
			Name:            "test-repo",
			Issues:          10,
			PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 3, Closed: 0},
			Tags:            3,
			Releases:        2,
			CommitCount:     100,
			LatestCommitSHA: "abc123",
		},
		&RepositoryData{
			Owner:           "target-org",
			Name:            "test-repo",
			Issues:          11,
			PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 3, Closed: 0},
			Tags:            3,
			Releases:        2,
			CommitCount:     100,
			LatestCommitSHA: "abc123",
		},
	)

	results := validator.validateRepositoryData()

	// Use the helper to validate metric names and presence
	validateMetricNames(t, results)
}

func TestMarkdownTable_DifferentScenarios(t *testing.T) {
	scenarios := []struct {
		name     string
		results  []ValidationResult
		expected []string
	}{
		{
			name: "All passing",
			results: []ValidationResult{
				{Metric: "Issues", SourceVal: 10, TargetVal: 11, Status: "✅ PASS", StatusType: ValidationStatusPass, Difference: 0},
				{Metric: "PRs", SourceVal: 5, TargetVal: 5, Status: "✅ PASS", StatusType: ValidationStatusPass, Difference: 0},
			},
			expected: []string{
				"- **Passed:** 2",
				"- **Failed:** 0",
				"- **Warnings:** 0",
				"**Result:** ✅ Migration validation PASSED",
			},
		},
		{
			name: "Mixed results",
			results: []ValidationResult{
				{Metric: "Issues", SourceVal: 10, TargetVal: 9, Status: "❌ FAIL", StatusType: ValidationStatusFail, Difference: 2},
				{Metric: "PRs", SourceVal: 5, TargetVal: 6, Status: "⚠️ WARN", StatusType: ValidationStatusWarn, Difference: -1},
			},
			expected: []string{
				"- **Passed:** 0",
				"- **Failed:** 1",
				"- **Warnings:** 1",
				"**Result:** ❌ Migration validation FAILED",
			},
		},
		{
			name: "Only warnings",
			results: []ValidationResult{
				{Metric: "Issues", SourceVal: 10, TargetVal: 12, Status: "⚠️ WARN", StatusType: ValidationStatusWarn, Difference: -1},
				{Metric: "PRs", SourceVal: 5, TargetVal: 6, Status: "⚠️ WARN", StatusType: ValidationStatusWarn, Difference: -1},
			},
			expected: []string{
				"- **Passed:** 0",
				"- **Failed:** 0",
				"- **Warnings:** 2",
				"**Result:** ⚠️ Migration validation completed with WARNINGS",
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			validator := setupTestValidator(
				&RepositoryData{Owner: "source-org", Name: "test-repo"},
				&RepositoryData{Owner: "target-org", Name: "test-repo"},
			)

			output := captureOutput(func() {
				validator.printMarkdownTable(scenario.results)
			})

			for _, expected := range scenario.expected {
				assert.Contains(t, output, expected, "Output should contain expected text")
			}
		})
	}
}

func TestSetSourceDataFromExport(t *testing.T) {
	// Create a validator instance
	validator := &MigrationValidator{
		api:        nil,
		SourceData: &RepositoryData{}, // Empty initially
		TargetData: &RepositoryData{},
	}

	// Create test export data
	exportData := &RepositoryData{
		Owner:           "exported-org",
		Name:            "exported-repo",
		Issues:          25,
		PRs:             &api.PRCounts{Total: 15, Open: 3, Merged: 10, Closed: 2},
		Tags:            8,
		Releases:        4,
		CommitCount:     200,
		LatestCommitSHA: "export123abc",
	}

	// Set source data from export
	validator.SetSourceDataFromExport(exportData)

	// Verify that source data was set correctly
	assert.Equal(t, "exported-org", validator.SourceData.Owner)
	assert.Equal(t, "exported-repo", validator.SourceData.Name)
	assert.Equal(t, 25, validator.SourceData.Issues)
	assert.Equal(t, 15, validator.SourceData.PRs.Total)
	assert.Equal(t, 3, validator.SourceData.PRs.Open)
	assert.Equal(t, 10, validator.SourceData.PRs.Merged)
	assert.Equal(t, 2, validator.SourceData.PRs.Closed)
	assert.Equal(t, 8, validator.SourceData.Tags)
	assert.Equal(t, 4, validator.SourceData.Releases)
	assert.Equal(t, 200, validator.SourceData.CommitCount)
	assert.Equal(t, "export123abc", validator.SourceData.LatestCommitSHA)
}

func TestSetSourceDataFromExport_NilPRCounts(t *testing.T) {
	validator := &MigrationValidator{
		api:        nil,
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}

	// Test with nil PR counts
	exportData := &RepositoryData{
		Owner:           "exported-org",
		Name:            "exported-repo",
		Issues:          10,
		PRs:             nil, // Nil PR counts
		Tags:            5,
		Releases:        2,
		CommitCount:     100,
		LatestCommitSHA: "test123",
	}

	validator.SetSourceDataFromExport(exportData)

	// Should handle nil PR counts gracefully
	assert.Equal(t, "exported-org", validator.SourceData.Owner)
	assert.Equal(t, "exported-repo", validator.SourceData.Name)
	assert.Equal(t, 10, validator.SourceData.Issues)
	assert.Nil(t, validator.SourceData.PRs)
}

func TestValidateFromExport_NoSourceData(t *testing.T) {
	// Disable pterm output for testing
	pterm.DisableOutput()
	defer pterm.EnableOutput()

	validator := &MigrationValidator{
		api:        nil,
		SourceData: nil, // No source data loaded
		TargetData: &RepositoryData{},
	}

	// Should return error when no source data is loaded
	results, err := validator.ValidateFromExport("target-org", "target-repo")

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "source data not properly loaded")
	assert.Contains(t, err.Error(), "call SetSourceDataFromExport with valid data first")
}

func TestValidateFromExport_CompleteWorkflow(t *testing.T) {
	// Test the complete workflow by simulating both source and target data
	// This tests the validation logic without requiring API calls

	validator := &MigrationValidator{
		api:        nil,               // Not needed for this test
		SourceData: &RepositoryData{}, // Will be set via SetSourceDataFromExport
		TargetData: &RepositoryData{
			// Simulate target data as if it was retrieved successfully
			Owner:           "target-org",
			Name:            "target-repo",
			Issues:          16, // Source has 15, expect 16 (15+1 for migration log)
			PRs:             &api.PRCounts{Total: 8, Open: 2, Merged: 5, Closed: 1},
			Tags:            4,
			Releases:        2,
			CommitCount:     120,
			LatestCommitSHA: "abc123export",
		},
	}

	// Set up source data from export
	exportSourceData := &RepositoryData{
		Owner:           "source-org",
		Name:            "source-repo",
		Issues:          15,
		PRs:             &api.PRCounts{Total: 8, Open: 2, Merged: 5, Closed: 1},
		Tags:            4,
		Releases:        2,
		CommitCount:     120,
		LatestCommitSHA: "abc123export",
	}

	validator.SetSourceDataFromExport(exportSourceData)

	// Since we can't test the API call part easily, we can test the validation
	// logic that would run after successful target data retrieval
	results := validator.validateRepositoryData()

	// Should get validation results
	assert.NotNil(t, results)
	assert.Equal(t, len(expectedValidationMetrics), len(results)) // Should have expected validation metrics

	// Check that validation logic works correctly with export data
	// All should pass since target data matches expected values
	passCount := 0
	for _, result := range results {
		if result.StatusType == ValidationStatusPass {
			passCount++
		}
	}
	assert.Equal(t, len(expectedValidationMetrics), passCount, "All validations should pass with perfect match")
}

func TestValidateFromExport_SourceDataValidation(t *testing.T) {
	// Test ONLY the source data validation part of ValidateFromExport
	// We should not test the full function since it makes API calls

	t.Run("Nil source data returns error immediately", func(t *testing.T) {
		validator := &MigrationValidator{
			api:        nil,
			SourceData: nil, // This should trigger immediate error
			TargetData: &RepositoryData{},
		}

		results, err := validator.ValidateFromExport("target-org", "target-repo")

		// Should fail immediately with source data error, before any API calls
		assert.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "source data not properly loaded")
		assert.Contains(t, err.Error(), "call SetSourceDataFromExport with valid data first")
	})

	t.Run("Valid source data structure", func(t *testing.T) {
		// For this test, we only verify that source data validation passes
		// We don't actually call ValidateFromExport since it makes API calls

		sourceData := &RepositoryData{
			Owner:  "test-org",
			Name:   "test-repo",
			Issues: 10,
			PRs:    &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
		}

		validator := &MigrationValidator{
			api:        nil,
			SourceData: sourceData,
			TargetData: &RepositoryData{},
		}

		// Just verify source data was set correctly
		assert.NotNil(t, validator.SourceData)
		assert.Equal(t, "test-org", validator.SourceData.Owner)
		assert.Equal(t, "test-repo", validator.SourceData.Name)

		// We don't call ValidateFromExport here because it would make API calls
		// That's tested in the integration workflow test instead
	})
}

func TestRetrieveSourceData_PublicMethodExists(t *testing.T) {
	// This test verifies that RetrieveSourceData is properly exposed as a public method
	// We don't call it since it makes API calls, but we verify it exists at compile time

	validator := &MigrationValidator{
		api:        nil,
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}

	// If this compiles, it means the public method exists with the correct signature
	// We use a type assertion to verify the method signature without calling it
	var method func(string, string, *pterm.SpinnerPrinter) ([]string, error) = validator.RetrieveSourceData

	assert.NotNil(t, method, "RetrieveSourceData method should exist")

	// This test serves as a compile-time verification that:
	// 1. The method is public (capitalized)
	// 2. It has the expected signature (returns []string for error messages, error)
	// 3. It's callable from external packages
}

func TestExportValidationWorkflow_Integration(t *testing.T) {
	// Integration test showing the full workflow for export-based validation
	// This demonstrates how the new methods work together without API dependencies

	// Step 1: Create validator
	validator := &MigrationValidator{
		api:        nil, // API not needed for this workflow test
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}

	// Step 2: Simulate loaded export data
	exportSourceData := &RepositoryData{
		Owner:           "source-org",
		Name:            "source-repo",
		Issues:          15,
		PRs:             &api.PRCounts{Total: 8, Open: 2, Merged: 5, Closed: 1},
		Tags:            4,
		Releases:        2,
		CommitCount:     120,
		LatestCommitSHA: "abc123export",
	}

	// Step 3: Set source data from export
	validator.SetSourceDataFromExport(exportSourceData)

	// Verify source data is set correctly
	assert.Equal(t, "source-org", validator.SourceData.Owner)
	assert.Equal(t, "source-repo", validator.SourceData.Name)
	assert.Equal(t, 15, validator.SourceData.Issues)
	assert.Equal(t, 8, validator.SourceData.PRs.Total)

	// Step 4: Simulate target data (as if retrieved from API)
	validator.TargetData = &RepositoryData{
		Owner:           "target-org",
		Name:            "target-repo",
		Issues:          16, // Expected: 15+1 for migration log
		PRs:             &api.PRCounts{Total: 8, Open: 2, Merged: 5, Closed: 1},
		Tags:            4,
		Releases:        2,
		CommitCount:     120,
		LatestCommitSHA: "abc123export",
	}

	// Step 5: Test validation logic (bypass API call by calling validateRepositoryData directly)
	results := validator.validateRepositoryData()

	// Verify validation results
	assert.NotNil(t, results)
	validateMetricNames(t, results)

	// Check that all validations pass with matching data
	passCount := 0
	for _, result := range results {
		if result.StatusType == ValidationStatusPass {
			passCount++
		}
	}
	assert.Equal(t, len(expectedValidationMetrics), passCount, "All validations should pass with perfect match")

	// This demonstrates the complete workflow: export → set source → validate
	// In real usage, the API call would happen in ValidateFromExport()
}

func BenchmarkValidateRepositoryData(b *testing.B) {
	validator := setupTestValidator(
		&RepositoryData{
			Owner:           "source-org",
			Name:            "test-repo",
			Issues:          10,
			PRs:             &api.PRCounts{Total: 20, Open: 5, Merged: 15, Closed: 0},
			Tags:            5,
			Releases:        3,
			CommitCount:     100,
			LatestCommitSHA: "abc123",
		},
		&RepositoryData{
			Owner:           "target-org",
			Name:            "test-repo",
			Issues:          11,
			PRs:             &api.PRCounts{Total: 20, Open: 5, Merged: 15, Closed: 0},
			Tags:            5,
			Releases:        3,
			CommitCount:     100,
			LatestCommitSHA: "abc123",
		},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.validateRepositoryData()
	}
}

func BenchmarkSetSourceDataFromExport(b *testing.B) {
	validator := &MigrationValidator{
		api:        nil,
		SourceData: &RepositoryData{},
		TargetData: &RepositoryData{},
	}

	exportData := &RepositoryData{
		Owner:           "benchmark-org",
		Name:            "benchmark-repo",
		Issues:          100,
		PRs:             &api.PRCounts{Total: 50, Open: 10, Merged: 35, Closed: 5},
		Tags:            20,
		Releases:        10,
		CommitCount:     1000,
		LatestCommitSHA: "benchmark123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.SetSourceDataFromExport(exportData)
	}
}

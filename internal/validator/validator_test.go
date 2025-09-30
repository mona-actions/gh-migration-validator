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

func TestValidateRepositoryData_PerfectMatch(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:           "source-org",
		Name:            "test-repo",
		Issues:          10,
		PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:            3,
		Releases:        2,
		CommitCount:     100,
		LatestCommitSHA: "abc123",
	}

	targetData := &RepositoryData{
		Owner:           "target-org",
		Name:            "test-repo",
		Issues:          11, // Expected: source + 1 for migration log
		PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:            3,
		Releases:        2,
		CommitCount:     100,
		LatestCommitSHA: "abc123",
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Should have 8 validation results
	assert.Equal(t, 8, len(results), "Should have 8 validation results")

	// Check that all results pass
	passCount := 0
	for _, result := range results {
		if result.Status == "✅ PASS" {
			passCount++
		}
	}
	assert.Equal(t, 8, passCount, "All validations should pass for perfect match")

	// Verify specific results
	issueResult := results[0]
	assert.Equal(t, "Issues (expected +1 for migration log)", issueResult.Metric)
	assert.Equal(t, "✅ PASS", issueResult.Status)
	assert.Equal(t, 0, issueResult.Difference)

	// Verify PR results
	prResult := results[1]
	assert.Equal(t, "Pull Requests (Total)", prResult.Metric)
	assert.Equal(t, "✅ PASS", prResult.Status)
	assert.Equal(t, 5, prResult.SourceVal)
	assert.Equal(t, 5, prResult.TargetVal)

	// Verify commit SHA result
	shaResult := results[7]
	assert.Equal(t, "Latest Commit SHA", shaResult.Metric)
	assert.Equal(t, "✅ PASS", shaResult.Status)
	assert.Equal(t, "abc123", shaResult.SourceVal)
	assert.Equal(t, "abc123", shaResult.TargetVal)
}

func TestValidateRepositoryData_MissingData(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:           "source-org",
		Name:            "test-repo",
		Issues:          10,
		PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:            3,
		Releases:        2,
		CommitCount:     100,
		LatestCommitSHA: "abc123",
	}

	targetData := &RepositoryData{
		Owner:           "target-org",
		Name:            "test-repo",
		Issues:          8,                                                      // Missing 3 (should be 11, but is 8)
		PRs:             &api.PRCounts{Total: 3, Open: 1, Merged: 1, Closed: 1}, // Missing 2 total PRs
		Tags:            2,                                                      // Missing 1 tag
		Releases:        1,                                                      // Missing 1 release
		CommitCount:     90,                                                     // Missing 10 commits
		LatestCommitSHA: "def456",                                               // Different commit SHA
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Count statuses
	failCount := 0
	for _, result := range results {
		if result.Status == "❌ FAIL" {
			failCount++
		}
	}
	assert.Equal(t, 8, failCount, "Should have 8 failures for missing data")

	// Check issues validation
	issueResult := results[0]
	assert.Equal(t, "❌ FAIL", issueResult.Status)
	assert.Equal(t, 3, issueResult.Difference) // Expected 11, got 8

	// Check PR validation
	prResult := results[1]
	assert.Equal(t, "❌ FAIL", prResult.Status)
	assert.Equal(t, 2, prResult.Difference) // Expected 5, got 3

	// Check commit SHA validation
	shaResult := results[7]
	assert.Equal(t, "❌ FAIL", shaResult.Status)
	assert.Equal(t, "abc123", shaResult.SourceVal)
	assert.Equal(t, "def456", shaResult.TargetVal)
}

func TestValidateRepositoryData_ExtraData(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:           "source-org",
		Name:            "test-repo",
		Issues:          10,
		PRs:             &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
		Tags:            3,
		Releases:        2,
		CommitCount:     100,
		LatestCommitSHA: "abc123",
	}

	targetData := &RepositoryData{
		Owner:           "target-org",
		Name:            "test-repo",
		Issues:          13,                                                     // 2 extra (should be 11, but is 13)
		PRs:             &api.PRCounts{Total: 7, Open: 3, Merged: 3, Closed: 1}, // 2 extra PRs
		Tags:            5,                                                      // 2 extra tags
		Releases:        4,                                                      // 2 extra releases
		CommitCount:     110,                                                    // 10 extra commits
		LatestCommitSHA: "abc123",                                               // Same commit SHA
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Count warnings and passes
	warnCount := 0
	passCount := 0
	for _, result := range results {
		if result.Status == "⚠️ WARN" {
			warnCount++
		} else if result.Status == "✅ PASS" {
			passCount++
		}
	}
	assert.Equal(t, 7, warnCount, "Should have 7 warnings for extra data")
	assert.Equal(t, 1, passCount, "Should have 1 pass (commit SHA)")

	// Check issues validation (extra data)
	issueResult := results[0]
	assert.Equal(t, "⚠️ WARN", issueResult.Status)
	assert.Equal(t, -2, issueResult.Difference) // Expected 11, got 13

	// Check commit SHA validation (should still pass)
	shaResult := results[7]
	assert.Equal(t, "✅ PASS", shaResult.Status)
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
			Status:     "✅ PASS",
			Difference: 0,
		},
		{
			Metric:     "PRs",
			SourceVal:  5,
			TargetVal:  3,
			Status:     "❌ FAIL",
			Difference: 2,
		},
		{
			Metric:     "Tags",
			SourceVal:  3,
			TargetVal:  5,
			Status:     "⚠️ WARN",
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
			Difference: 0,
		},
		{
			Metric:     "PRs",
			SourceVal:  5,
			TargetVal:  3,
			Status:     "❌ FAIL",
			Difference: 2,
		},
		{
			Metric:     "Tags",
			SourceVal:  3,
			TargetVal:  5,
			Status:     "⚠️ WARN",
			Difference: -2,
		},
		{
			Metric:     "Latest Commit SHA",
			SourceVal:  "abc123",
			TargetVal:  "def456",
			Status:     "❌ FAIL",
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
		name           string
		sourceIssues   int
		targetIssues   int
		expectedStatus string
		expectedDiff   int
	}{
		{
			name:           "Perfect issues match (+1 expected)",
			sourceIssues:   10,
			targetIssues:   11,
			expectedStatus: "✅ PASS",
			expectedDiff:   0,
		},
		{
			name:           "Missing issues in target",
			sourceIssues:   10,
			targetIssues:   9,
			expectedStatus: "❌ FAIL",
			expectedDiff:   2, // Expected 11, got 9
		},
		{
			name:           "Extra issues in target",
			sourceIssues:   10,
			targetIssues:   13,
			expectedStatus: "⚠️ WARN",
			expectedDiff:   -2, // Expected 11, got 13
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

			assert.Equal(t, tt.expectedStatus, issueResult.Status, "Status should match expected")
			assert.Equal(t, tt.expectedDiff, issueResult.Difference, "Difference should match expected")
		})
	}
}

func TestValidationResult_CommitSHAComparison(t *testing.T) {
	tests := []struct {
		name           string
		sourceSHA      string
		targetSHA      string
		expectedStatus string
	}{
		{
			name:           "Matching commit SHAs",
			sourceSHA:      "abc123def456",
			targetSHA:      "abc123def456",
			expectedStatus: "✅ PASS",
		},
		{
			name:           "Different commit SHAs",
			sourceSHA:      "abc123def456",
			targetSHA:      "xyz789uvw012",
			expectedStatus: "❌ FAIL",
		},
		{
			name:           "Empty source SHA",
			sourceSHA:      "",
			targetSHA:      "abc123def456",
			expectedStatus: "❌ FAIL",
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

			// Find the commit SHA result (should be last)
			shaResult := results[len(results)-1]
			assert.Equal(t, "Latest Commit SHA", shaResult.Metric)
			assert.Equal(t, tt.expectedStatus, shaResult.Status)
			assert.Equal(t, tt.sourceSHA, shaResult.SourceVal)
			assert.Equal(t, tt.targetSHA, shaResult.TargetVal)
			assert.Equal(t, 0, shaResult.Difference) // Always 0 for SHA comparison
		})
	}
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
				{Metric: "Issues", SourceVal: 10, TargetVal: 11, Status: "✅ PASS", Difference: 0},
				{Metric: "PRs", SourceVal: 5, TargetVal: 5, Status: "✅ PASS", Difference: 0},
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
				{Metric: "Issues", SourceVal: 10, TargetVal: 9, Status: "❌ FAIL", Difference: 2},
				{Metric: "PRs", SourceVal: 5, TargetVal: 6, Status: "⚠️ WARN", Difference: -1},
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
				{Metric: "Issues", SourceVal: 10, TargetVal: 12, Status: "⚠️ WARN", Difference: -1},
				{Metric: "PRs", SourceVal: 5, TargetVal: 6, Status: "⚠️ WARN", Difference: -1},
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

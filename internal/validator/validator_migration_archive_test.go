package validator

import (
	"testing"

	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/migrationarchive"

	"github.com/stretchr/testify/assert"
)

// Test migration archive functionality
func TestSetSourceDataFromExport_WithMigrationArchive(t *testing.T) {
	validator := New(nil)

	// Test data with migration archive
	repositoryData := &RepositoryData{
		Owner:           "test-owner",
		Name:            "test-repo",
		Issues:          5,
		PRs:             &api.PRCounts{Open: 1, Closed: 2, Merged: 3, Total: 6},
		Tags:            10,
		Releases:        8,
		CommitCount:     100,
		LatestCommitSHA: "sha123",
		MigrationArchive: &migrationarchive.MigrationArchiveMetrics{
			Issues:            6,
			PullRequests:      29,
			ProtectedBranches: 1,
			Releases:          25,
		},
	}

	validator.SetSourceDataFromExport(repositoryData)

	// Verify migration archive data was set
	assert.NotNil(t, validator.SourceData.MigrationArchive, "Migration archive should not be nil")
	assert.Equal(t, 6, validator.SourceData.MigrationArchive.Issues)
	assert.Equal(t, 29, validator.SourceData.MigrationArchive.PullRequests)
	assert.Equal(t, 1, validator.SourceData.MigrationArchive.ProtectedBranches)
	assert.Equal(t, 25, validator.SourceData.MigrationArchive.Releases)
}

func TestValidateRepositoryData_WithMigrationArchive(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:                 "source-org",
		Name:                  "source-repo",
		Issues:                2,
		PRs:                   &api.PRCounts{Total: 29, Open: 0, Merged: 27, Closed: 2},
		Tags:                  25,
		Releases:              25,
		CommitCount:           64,
		LatestCommitSHA:       "source123",
		BranchProtectionRules: 1,
		Webhooks:              0,
		MigrationArchive: &migrationarchive.MigrationArchiveMetrics{
			Issues:            6,
			PullRequests:      29,
			ProtectedBranches: 1,
			Releases:          25,
		},
	}

	targetData := &RepositoryData{
		Owner:                 "target-org",
		Name:                  "target-repo",
		Issues:                7, // 6 from archive + 1 migration log
		PRs:                   &api.PRCounts{Total: 29, Open: 0, Merged: 27, Closed: 2},
		Tags:                  25,
		Releases:              25,
		CommitCount:           64,
		LatestCommitSHA:       "target123",
		BranchProtectionRules: 1,
		Webhooks:              0,
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Check that we have migration archive validation results
	hasArchiveVsSource := false
	hasArchiveVsTarget := false

	for _, result := range results {
		if result.Metric == "Archive vs Source Issues" {
			hasArchiveVsSource = true
			// Archive has 6, source API has 2, difference = 4
			assert.Equal(t, 4, result.Difference)
		}
		if result.Metric == "Archive vs Target Issues (expected +1 for migration log)" {
			hasArchiveVsTarget = true
			// Expected target: 6 + 1 = 7, actual target: 7, difference = 0
			assert.Equal(t, 0, result.Difference)
		}
	}

	assert.True(t, hasArchiveVsSource, "Should have archive vs source validation")
	assert.True(t, hasArchiveVsTarget, "Should have archive vs target validation")
}

func TestValidateRepositoryData_WithoutMigrationArchive(t *testing.T) {
	sourceData := &RepositoryData{
		Owner:                 "source-org",
		Name:                  "source-repo",
		Issues:                2,
		PRs:                   &api.PRCounts{Total: 29, Open: 0, Merged: 27, Closed: 2},
		Tags:                  25,
		Releases:              25,
		CommitCount:           64,
		LatestCommitSHA:       "source123",
		BranchProtectionRules: 1,
		Webhooks:              0,
		MigrationArchive:      nil, // No migration archive
	}

	targetData := &RepositoryData{
		Owner:                 "target-org",
		Name:                  "target-repo",
		Issues:                3,
		PRs:                   &api.PRCounts{Total: 29, Open: 0, Merged: 27, Closed: 2},
		Tags:                  25,
		Releases:              25,
		CommitCount:           64,
		LatestCommitSHA:       "target123",
		BranchProtectionRules: 1,
		Webhooks:              0,
	}

	validator := setupTestValidator(sourceData, targetData)
	results := validator.validateRepositoryData()

	// Check that we don't have migration archive validation results
	for _, result := range results {
		assert.NotContains(t, result.Metric, "Archive vs", "Should not have archive validation without migration archive data")
	}

	// Should only have standard validation metrics
	assert.Equal(t, len(expectedValidationMetrics), len(results), "Should have standard validation metrics count")
}

func TestDisplayValidationTable_Headers(t *testing.T) {
	validator := New(nil)

	// Test different table types have correct headers
	testCases := []struct {
		title           string
		expectedHeaders []string
	}{
		{
			title:           "🔄 Source vs Target Validation",
			expectedHeaders: []string{"Metric", "Status", "Source Value", "Target Value", "Difference"},
		},
		{
			title:           "📦 Migration Archive vs Source Validation",
			expectedHeaders: []string{"Metric", "Status", "Source API Value", "Archive Value", "Difference"},
		},
		{
			title:           "🎯 Migration Archive vs Target Validation",
			expectedHeaders: []string{"Metric", "Status", "Archive Value", "Target Value", "Difference"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			// Capture the table output (this would require more sophisticated mocking)
			// For now, we can at least verify the function doesn't panic
			validator.displayValidationTable(tc.title, []ValidationResult{})

			// TODO: Implement proper output capture and header verification
			// This would require mocking pterm.DefaultTable or capturing its output
		})
	}
}

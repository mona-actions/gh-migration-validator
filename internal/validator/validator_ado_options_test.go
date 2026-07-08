package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mona-actions/gh-migration-validator/internal/api"
)

// findResult returns the ValidationResult for the given metric, or nil if absent.
func findResult(results []ValidationResult, metric string) *ValidationResult {
	for i := range results {
		if results[i].Metric == metric {
			return &results[i]
		}
	}
	return nil
}

// TestValidateRepositoryData_ADOLFSMigrateCommit verifies the Azure DevOps special case
// where `git lfs migrate` rewrites the tip commit: when the source has no LFS objects
// but the target does and the parent commit still matches, the differing latest commit
// SHA is downgraded from FAIL to WARN.
func TestValidateRepositoryData_ADOLFSMigrateCommit(t *testing.T) {
	tests := []struct {
		name               string
		sourceLabel        string
		sourceLFS          int
		targetLFS          int
		sourceParent       string
		targetParent       string
		expectedStatusType ValidationStatus
	}{
		{
			name:               "ADO LFS migrate with matching parent warns",
			sourceLabel:        "Azure DevOps",
			sourceLFS:          0,
			targetLFS:          1,
			sourceParent:       "parent-abc",
			targetParent:       "parent-abc",
			expectedStatusType: ValidationStatusWarn,
		},
		{
			name:               "ADO LFS migrate with mismatched parent fails",
			sourceLabel:        "Azure DevOps",
			sourceLFS:          0,
			targetLFS:          1,
			sourceParent:       "parent-abc",
			targetParent:       "parent-xyz",
			expectedStatusType: ValidationStatusFail,
		},
		{
			name:               "ADO but source also has LFS fails",
			sourceLabel:        "Azure DevOps",
			sourceLFS:          1,
			targetLFS:          1,
			sourceParent:       "parent-abc",
			targetParent:       "parent-abc",
			expectedStatusType: ValidationStatusFail,
		},
		{
			name:               "Non-ADO source with matching parent still fails",
			sourceLabel:        "Bitbucket Server",
			sourceLFS:          0,
			targetLFS:          1,
			sourceParent:       "parent-abc",
			targetParent:       "parent-abc",
			expectedStatusType: ValidationStatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := setupTestValidator(
				&RepositoryData{
					Owner:                 "source",
					Name:                  "repo",
					PRs:                   &api.PRCounts{},
					LatestCommitSHA:       "source-tip",
					LatestCommitParentSHA: tt.sourceParent,
					LFSObjects:            tt.sourceLFS,
				},
				&RepositoryData{
					Owner:                 "target",
					Name:                  "repo",
					PRs:                   &api.PRCounts{},
					LatestCommitSHA:       "target-tip",
					LatestCommitParentSHA: tt.targetParent,
					LFSObjects:            tt.targetLFS,
				},
			)

			results := validator.validateRepositoryData(ValidationOptions{
				SkipIssues:             true,
				SkipReleases:           true,
				SkipMigrationLogOffset: true,
				SkipMigrationArchive:   true,
				SourceLabel:            tt.sourceLabel,
			})

			shaResult := findResult(results, "Latest Commit SHA")
			require.NotNil(t, shaResult, "Latest Commit SHA result must be present")
			assert.Equal(t, tt.expectedStatusType, shaResult.StatusType)

			// When downgraded to WARN, the Difference column must explain why.
			if tt.expectedStatusType == ValidationStatusWarn {
				assert.NotEmpty(t, shaResult.DifferenceNote,
					"a warned SHA mismatch must include an explanatory DifferenceNote")
				assert.Equal(t, shaResult.DifferenceNote, formatDifference(*shaResult),
					"DifferenceNote should be surfaced by formatDifference")
			} else {
				assert.Empty(t, shaResult.DifferenceNote,
					"only warned mismatches should carry a DifferenceNote")
			}
		})
	}
}

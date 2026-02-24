package validator

import (
"testing"

"mona-actions/gh-migration-validator/internal/api"

"github.com/pterm/pterm"
"github.com/stretchr/testify/assert"
)

// --- ValidationStatusInfo constant tests ---

func TestValidationStatusInfo_IotaValue(t *testing.T) {
// Verify iota ordering is preserved: Pass=0, Fail=1, Warn=2, Info=3
assert.Equal(t, ValidationStatus(0), ValidationStatusPass, "Pass should be 0")
assert.Equal(t, ValidationStatus(1), ValidationStatusFail, "Fail should be 1")
assert.Equal(t, ValidationStatus(2), ValidationStatusWarn, "Warn should be 2")
assert.Equal(t, ValidationStatus(3), ValidationStatusInfo, "Info should be 3")
}

func TestValidationStatusMessageInfo_Value(t *testing.T) {
assert.Equal(t, "ℹ️ INFO", ValidationStatusMessageInfo)
}

// --- HasFailures does NOT count INFO as failure ---

func TestHasFailures_InfoDoesNotCountAsFailure(t *testing.T) {
results := []ValidationResult{
{StatusType: ValidationStatusPass},
{StatusType: ValidationStatusInfo},
{StatusType: ValidationStatusWarn},
}

assert.False(t, HasFailures(results), "INFO results should not be counted as failures")
}

func TestHasFailures_InfoWithFailure(t *testing.T) {
results := []ValidationResult{
{StatusType: ValidationStatusInfo},
{StatusType: ValidationStatusFail},
}

assert.True(t, HasFailures(results), "Should still detect failures when INFO is present")
}

// --- SetSourceData alias tests ---

func TestSetSourceData_AliasForSetSourceDataFromExport(t *testing.T) {
validator := New(nil)

data := &RepositoryData{
Owner:           "bbs-org",
Name:            "bbs-repo",
Issues:          0,
PRs:             &api.PRCounts{Total: 10, Open: 2, Merged: 7, Closed: 1},
Tags:            5,
CommitCount:     200,
LatestCommitSHA: "sha256abc",
}

validator.SetSourceData(data)

assert.Equal(t, "bbs-org", validator.SourceData.Owner)
assert.Equal(t, "bbs-repo", validator.SourceData.Name)
assert.Equal(t, 10, validator.SourceData.PRs.Total)
assert.Equal(t, 200, validator.SourceData.CommitCount)
}

func TestSetSourceData_DeepCopiesPRs(t *testing.T) {
validator := New(nil)

original := &RepositoryData{
Owner: "org",
Name:  "repo",
PRs:   &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
}

validator.SetSourceData(original)

// Mutate original - should not affect validator's copy
original.PRs.Total = 999

assert.Equal(t, 5, validator.SourceData.PRs.Total,
"SetSourceData should deep copy PR data")
}

func TestSetSourceData_NilPRs(t *testing.T) {
validator := New(nil)

data := &RepositoryData{
Owner: "org",
Name:  "repo",
PRs:   nil,
}

validator.SetSourceData(data)
assert.Nil(t, validator.SourceData.PRs, "Should handle nil PRs gracefully")
}

// --- validateRepositoryDataWithOptions tests ---

func TestValidateWithOptions_SkipIssues(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
Issues:          10,
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        2,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
Issues:          0, // Completely different, but should be skipped
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        2,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
)

opts := ValidationOptions{SkipIssues: true}
results := validator.validateRepositoryDataWithOptions(opts)

for _, result := range results {
assert.NotContains(t, result.Metric, "Issues",
"Issues metric should be skipped when SkipIssues is true")
}
}

func TestValidateWithOptions_SkipReleases(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
Issues:          10,
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        5,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
Issues:          11,
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        0, // Completely different, but should be skipped
CommitCount:     100,
LatestCommitSHA: "abc123",
},
)

opts := ValidationOptions{SkipReleases: true}
results := validator.validateRepositoryDataWithOptions(opts)

for _, result := range results {
assert.NotEqual(t, "Releases", result.Metric,
"Releases metric should be skipped when SkipReleases is true")
}
}

func TestValidateWithOptions_SkipLFS(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LFSObjects:      50,
LatestCommitSHA: "abc",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LFSObjects:      0,
LatestCommitSHA: "abc",
},
)

opts := ValidationOptions{SkipLFS: true}
results := validator.validateRepositoryDataWithOptions(opts)

for _, result := range results {
assert.NotEqual(t, "LFS Objects", result.Metric,
"LFS Objects metric should be skipped when SkipLFS is true")
}
}

func TestValidateWithOptions_SkipMigrationLogOffset(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
Issues:          10,
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LatestCommitSHA: "abc",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
Issues:          10, // Exact match without +1 offset
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LatestCommitSHA: "abc",
},
)

opts := ValidationOptions{SkipMigrationLogOffset: true}
results := validator.validateRepositoryDataWithOptions(opts)

// Find the issues result
var issueResult *ValidationResult
for i := range results {
if results[i].Metric == "Issues" {
issueResult = &results[i]
break
}
}

if assert.NotNil(t, issueResult, "Should find Issues metric") {
assert.Equal(t, ValidationStatusPass, issueResult.StatusType,
"Issues should pass when source==target and migration log offset is skipped")
assert.Equal(t, 0, issueResult.Difference)
assert.Equal(t, "Issues", issueResult.Metric,
"Metric label should be 'Issues' without offset note")
}
}

func TestValidateWithOptions_MigrationLogOffsetDefault(t *testing.T) {
// When SkipMigrationLogOffset is false (default), the +1 offset applies
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
Issues:          10,
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LatestCommitSHA: "abc",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
Issues:          11, // source + 1
PRs:             &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
LatestCommitSHA: "abc",
},
)

opts := ValidationOptions{} // Default: SkipMigrationLogOffset = false
results := validator.validateRepositoryDataWithOptions(opts)

var issueResult *ValidationResult
for i := range results {
if results[i].Metric == "Issues (expected +1 for migration log)" {
issueResult = &results[i]
break
}
}

if assert.NotNil(t, issueResult, "Should find Issues metric with +1 label") {
assert.Equal(t, ValidationStatusPass, issueResult.StatusType)
assert.Equal(t, 0, issueResult.Difference)
}
}

func TestValidateWithOptions_BranchPermissionsAdvisory(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:                 "source",
Name:                  "repo",
Issues:                10,
PRs:                   &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
BranchProtectionRules: 3,
LatestCommitSHA:       "abc",
},
&RepositoryData{
Owner:                 "target",
Name:                  "repo",
Issues:                11, // source + 1 for migration log
PRs:                   &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
BranchProtectionRules: 1, // Different — would normally be FAIL
LatestCommitSHA:       "abc",
},
)

opts := ValidationOptions{BranchPermissionsAdvisory: true}
results := validator.validateRepositoryDataWithOptions(opts)

var branchResult *ValidationResult
for i := range results {
if results[i].Metric == "Branch Protection Rules (advisory)" {
branchResult = &results[i]
break
}
}

if assert.NotNil(t, branchResult, "Should find advisory branch protection result") {
assert.Equal(t, ValidationStatusInfo, branchResult.StatusType,
"Branch protection with advisory flag should have INFO status")
assert.Equal(t, ValidationStatusMessageInfo, branchResult.Status,
"Branch protection display status should be INFO message")
assert.Equal(t, 2, branchResult.Difference,
"Difference should still be calculated")
}

// Verify INFO doesn't count as failure
assert.False(t, HasFailures(results),
"Advisory branch protection should not cause failure")
}

func TestValidateWithOptions_BranchPermissionsNonAdvisory(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{
Owner:                 "source",
Name:                  "repo",
Issues:                10,
PRs:                   &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
BranchProtectionRules: 3,
LatestCommitSHA:       "abc",
},
&RepositoryData{
Owner:                 "target",
Name:                  "repo",
Issues:                11,
PRs:                   &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
BranchProtectionRules: 1,
LatestCommitSHA:       "abc",
},
)

opts := ValidationOptions{BranchPermissionsAdvisory: false}
results := validator.validateRepositoryDataWithOptions(opts)

var branchResult *ValidationResult
for i := range results {
if results[i].Metric == "Branch Protection Rules" {
branchResult = &results[i]
break
}
}

if assert.NotNil(t, branchResult, "Should find standard branch protection result") {
assert.Equal(t, ValidationStatusFail, branchResult.StatusType,
"Non-advisory branch protection mismatch should FAIL")
}
}

func TestValidateWithOptions_BBSFullScenario(t *testing.T) {
// Simulate a full BBS migration validation scenario
validator := setupTestValidator(
&RepositoryData{
Owner:                 "bbs-project",
Name:                  "bbs-repo",
Issues:                0, // BBS has no issues
PRs:                   &api.PRCounts{Total: 25, Open: 3, Merged: 20, Closed: 2},
Tags:                  10,
Releases:              0, // BBS has no releases
CommitCount:           500,
LatestCommitSHA:       "bbs123abc",
BranchProtectionRules: 5,
Webhooks:              0,
LFSObjects:            0,
},
&RepositoryData{
Owner:                 "gh-org",
Name:                  "migrated-repo",
Issues:                1, // Just the migration log issue
PRs:                   &api.PRCounts{Total: 25, Open: 3, Merged: 20, Closed: 2},
Tags:                  10,
Releases:              0,
CommitCount:           500,
LatestCommitSHA:       "bbs123abc",
BranchProtectionRules: 2,
Webhooks:              0,
LFSObjects:            0,
},
)

opts := ValidationOptions{
SkipIssues:                true,
SkipReleases:              true,
SkipLFS:                   true,
SkipMigrationLogOffset:    true,
BranchPermissionsAdvisory: true,
SourceLabel:               "Bitbucket",
}

results := validator.validateRepositoryDataWithOptions(opts)

// Count by status type
statusCounts := map[ValidationStatus]int{}
for _, r := range results {
statusCounts[r.StatusType]++
}

// Should have no failures (branch protection is advisory INFO, not FAIL)
assert.False(t, HasFailures(results), "BBS scenario should not have failures")

// Verify skipped metrics are absent
for _, r := range results {
assert.NotContains(t, r.Metric, "Issues", "Issues should be skipped for BBS")
assert.NotEqual(t, "Releases", r.Metric, "Releases should be skipped for BBS")
assert.NotEqual(t, "LFS Objects", r.Metric, "LFS should be skipped for BBS")
}

// Verify branch protection is INFO
assert.Equal(t, 1, statusCounts[ValidationStatusInfo],
"Should have exactly 1 INFO result (branch protection)")

// Verify expected metrics are present
metricNames := make([]string, 0, len(results))
for _, r := range results {
metricNames = append(metricNames, r.Metric)
}
assert.Contains(t, metricNames, "Pull Requests (Total)")
assert.Contains(t, metricNames, "Pull Requests (Open)")
assert.Contains(t, metricNames, "Pull Requests (Merged)")
assert.Contains(t, metricNames, "Tags")
assert.Contains(t, metricNames, "Commits")
assert.Contains(t, metricNames, "Branch Protection Rules (advisory)")
assert.Contains(t, metricNames, "Webhooks")
assert.Contains(t, metricNames, "Latest Commit SHA")
}

func TestValidateWithOptions_NoSkips_MatchesDefault(t *testing.T) {
// With no options set, validateRepositoryDataWithOptions should produce
// the same results as validateRepositoryData for the standard metrics
sourceData := &RepositoryData{
Owner:                 "source",
Name:                  "repo",
Issues:                10,
PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
Tags:                  3,
Releases:              2,
CommitCount:           100,
LatestCommitSHA:       "abc123",
BranchProtectionRules: 4,
Webhooks:              2,
LFSObjects:            5,
}

targetData := &RepositoryData{
Owner:                 "target",
Name:                  "repo",
Issues:                11,
PRs:                   &api.PRCounts{Total: 5, Open: 2, Merged: 2, Closed: 1},
Tags:                  3,
Releases:              2,
CommitCount:           100,
LatestCommitSHA:       "abc123",
BranchProtectionRules: 4,
Webhooks:              2,
LFSObjects:            5,
}

validator := setupTestValidator(sourceData, targetData)

defaultResults := validator.validateRepositoryData()
optResults := validator.validateRepositoryDataWithOptions(ValidationOptions{})

// Both should produce same number of standard results (before migration archive)
// Note: validateRepositoryData includes migration archive if present; with no archive, they match
assert.Equal(t, len(defaultResults), len(optResults),
"With no options, result count should match default validateRepositoryData")

for i := range defaultResults {
assert.Equal(t, defaultResults[i].Metric, optResults[i].Metric,
"Metric names should match at index %d", i)
assert.Equal(t, defaultResults[i].StatusType, optResults[i].StatusType,
"Status types should match for %s", defaultResults[i].Metric)
assert.Equal(t, defaultResults[i].Difference, optResults[i].Difference,
"Differences should match for %s", defaultResults[i].Metric)
}
}

func TestValidateWithOptions_SkipsMigrationArchive(t *testing.T) {
// Even if source has migration archive data, validateRepositoryDataWithOptions
// should NOT include archive comparisons
validator := setupTestValidator(
&RepositoryData{
Owner:           "source",
Name:            "repo",
Issues:          10,
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        2,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
&RepositoryData{
Owner:           "target",
Name:            "repo",
Issues:          11,
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
Releases:        2,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
)

results := validator.validateRepositoryDataWithOptions(ValidationOptions{})

for _, r := range results {
assert.NotContains(t, r.Metric, "Archive",
"Options-based validation should never include Archive comparisons")
}
}

// --- ValidateWithOptions source validation tests ---

func TestValidateWithOptions_NilSourceData(t *testing.T) {
validator := &MigrationValidator{
api:        nil,
SourceData: nil,
TargetData: &RepositoryData{},
}

results, err := validator.ValidateWithOptions("target-org", "target-repo", ValidationOptions{})

assert.Error(t, err)
assert.Nil(t, results)
assert.Contains(t, err.Error(), "source data not properly loaded")
}

func TestValidateWithOptions_EmptySourceOwner(t *testing.T) {
validator := &MigrationValidator{
api:        nil,
SourceData: &RepositoryData{Owner: "", Name: "repo"},
TargetData: &RepositoryData{},
}

results, err := validator.ValidateWithOptions("target-org", "target-repo", ValidationOptions{})

assert.Error(t, err)
assert.Nil(t, results)
assert.Contains(t, err.Error(), "source data not properly loaded")
}

func TestValidateWithOptions_NormalizesNilPRs(t *testing.T) {
// Verify that validateRepositoryDataWithOptions handles nil PRs gracefully
// by testing the normalization that ValidateWithOptions performs before calling it.
validator := &MigrationValidator{
api: nil,
SourceData: &RepositoryData{
Owner: "source",
Name:  "repo",
PRs:   nil, // Will be normalized
},
TargetData: &RepositoryData{
Owner: "target",
Name:  "repo",
PRs:   &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0},
},
}

// Normalize as ValidateWithOptions would
if validator.SourceData.PRs == nil {
validator.SourceData.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
}

// This should not panic after normalization
assert.NotPanics(t, func() {
validator.validateRepositoryDataWithOptions(ValidationOptions{SkipLFS: true})
}, "Should not panic with normalized nil PRs")

assert.NotNil(t, validator.SourceData.PRs)
assert.Equal(t, 0, validator.SourceData.PRs.Total)
}

// --- displayValidationSummary INFO counter tests ---

func TestDisplayValidationSummary_WithInfoResults(t *testing.T) {
pterm.DisableOutput()
defer pterm.EnableOutput()

validator := setupTestValidator(
&RepositoryData{Owner: "src", Name: "repo"},
&RepositoryData{Owner: "tgt", Name: "repo"},
)

results := []ValidationResult{
{Metric: "PRs", StatusType: ValidationStatusPass, Status: ValidationStatusMessagePass},
{Metric: "Branch Protection", StatusType: ValidationStatusInfo, Status: ValidationStatusMessageInfo},
}

// Should not panic
assert.NotPanics(t, func() {
validator.displayValidationSummary(results)
})
}

// --- printMarkdownTable INFO counter tests ---

func TestPrintMarkdownTable_WithInfoResults(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{Owner: "source-org", Name: "test-repo"},
&RepositoryData{Owner: "target-org", Name: "test-repo"},
)

results := []ValidationResult{
{Metric: "PRs", SourceVal: 5, TargetVal: 5, Status: ValidationStatusMessagePass, StatusType: ValidationStatusPass, Difference: 0},
{Metric: "Branch Protection Rules (advisory)", SourceVal: 3, TargetVal: 1, Status: ValidationStatusMessageInfo, StatusType: ValidationStatusInfo, Difference: 2},
{Metric: "Tags", SourceVal: 3, TargetVal: 2, Status: ValidationStatusMessageFail, StatusType: ValidationStatusFail, Difference: 1},
}

output := captureOutput(func() {
validator.printMarkdownTable(results)
})

assert.Contains(t, output, "- **Passed:** 1", "Should count 1 pass")
assert.Contains(t, output, "- **Failed:** 1", "Should count 1 failure")
assert.Contains(t, output, "- **Warnings:** 0", "Should count 0 warnings")
assert.Contains(t, output, "- **Info:** 1", "Should count 1 info")
assert.Contains(t, output, "ℹ️ INFO", "Should contain INFO status in table")
}

func TestPrintMarkdownTable_NoInfoOmitsInfoLine(t *testing.T) {
validator := setupTestValidator(
&RepositoryData{Owner: "source-org", Name: "test-repo"},
&RepositoryData{Owner: "target-org", Name: "test-repo"},
)

results := []ValidationResult{
{Metric: "PRs", SourceVal: 5, TargetVal: 5, Status: ValidationStatusMessagePass, StatusType: ValidationStatusPass, Difference: 0},
}

output := captureOutput(func() {
validator.printMarkdownTable(results)
})

assert.NotContains(t, output, "- **Info:**",
"Should not include Info line when there are no INFO results")
}

// --- ValidationOptions struct tests ---

func TestValidationOptions_ZeroValueIsNoOp(t *testing.T) {
// Default zero-value options should not skip anything
opts := ValidationOptions{}

assert.False(t, opts.SkipIssues)
assert.False(t, opts.SkipReleases)
assert.False(t, opts.SkipLFS)
assert.False(t, opts.SkipMigrationLogOffset)
assert.False(t, opts.BranchPermissionsAdvisory)
assert.Empty(t, opts.SourceLabel)
}

func TestValidateWithOptions_AllSkips_MinimalResults(t *testing.T) {
// When everything possible is skipped, verify we still get the core metrics
validator := setupTestValidator(
&RepositoryData{
Owner:           "src",
Name:            "repo",
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
&RepositoryData{
Owner:           "tgt",
Name:            "repo",
PRs:             &api.PRCounts{Total: 5, Open: 1, Merged: 3, Closed: 1},
Tags:            3,
CommitCount:     100,
LatestCommitSHA: "abc123",
},
)

opts := ValidationOptions{
SkipIssues:                true,
SkipReleases:              true,
SkipLFS:                   true,
SkipMigrationLogOffset:    true,
BranchPermissionsAdvisory: true,
}

results := validator.validateRepositoryDataWithOptions(opts)

// Should have: PRs (Total, Open, Merged), Tags, Commits, Branch Protection (advisory), Webhooks, Latest Commit SHA
expectedMetrics := []string{
"Pull Requests (Total)",
"Pull Requests (Open)",
"Pull Requests (Merged)",
"Tags",
"Commits",
"Branch Protection Rules (advisory)",
"Webhooks",
"Latest Commit SHA",
}

assert.Equal(t, len(expectedMetrics), len(results),
"Should have exactly %d metrics with all skips enabled", len(expectedMetrics))

for i, expected := range expectedMetrics {
assert.Equal(t, expected, results[i].Metric,
"Metric at index %d should be %s", i, expected)
}
}

// --- Nil source client safety tests ---
// These tests verify that the BBS flow (target-only API) doesn't panic
// when source GitHub clients are not initialized.

func TestCheckAndWarnRateLimits_NilSourceClient_NoPanic(t *testing.T) {
	// Simulate BBS flow: API with nil source clients
	ghAPI := &api.GitHubAPI{}

	validator := New(ghAPI)

	// checkAndWarnRateLimits(false) should not attempt source rate limit check
	assert.NotPanics(t, func() {
		validator.checkAndWarnRateLimits(false)
	}, "checkAndWarnRateLimits(false) should not panic with nil source client")
}

func TestCheckAndWarnRateLimits_NilSourceClient_CheckSourceTrue_NoPanic(t *testing.T) {
	// Even with checkSource=true and nil clients, should not panic
	// (nil guards in getGraphQLClient return error instead of nil dereference)
	ghAPI := &api.GitHubAPI{}

	validator := New(ghAPI)

	assert.NotPanics(t, func() {
		validator.checkAndWarnRateLimits(true)
	}, "checkAndWarnRateLimits(true) should not panic even with nil clients (nil guard)")
}

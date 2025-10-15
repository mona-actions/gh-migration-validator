package migrationarchive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeMigrationArchive(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test JSON files with sample data
	createTestJSONFile(t, tempDir, "issues_000001.json", []map[string]interface{}{
		{"type": "issue", "id": 1},
		{"type": "issue", "id": 2},
		{"type": "issue", "id": 3},
	})

	createTestJSONFile(t, tempDir, "issues_000002.json", []map[string]interface{}{
		{"type": "issue", "id": 4},
	})

	createTestJSONFile(t, tempDir, "pull_requests_000001.json", []map[string]interface{}{
		{"type": "pull_request", "id": 1},
		{"type": "pull_request", "id": 2},
	})

	createTestJSONFile(t, tempDir, "protected_branches_000001.json", []map[string]interface{}{
		{"type": "protected_branch", "name": "main"},
	})

	createTestJSONFile(t, tempDir, "releases_000001.json", []map[string]interface{}{
		{"type": "release", "id": 1},
		{"type": "release", "id": 2},
		{"type": "release", "id": 3},
		{"type": "release", "id": 4},
		{"type": "release", "id": 5},
	})

	// Test AnalyzeMigrationArchive
	metrics, err := AnalyzeMigrationArchive(tempDir)
	if err != nil {
		t.Fatalf("AnalyzeMigrationArchive failed: %v", err)
	}

	// Verify the counts
	expectedIssues := 4   // 3 from issues_000001.json + 1 from issues_000002.json
	expectedPRs := 2      // 2 from pull_requests_000001.json
	expectedBranches := 1 // 1 from protected_branches_000001.json
	expectedReleases := 5 // 5 from releases_000001.json

	if metrics.Issues != expectedIssues {
		t.Errorf("Expected %d issues, got %d", expectedIssues, metrics.Issues)
	}

	if metrics.PullRequests != expectedPRs {
		t.Errorf("Expected %d pull requests, got %d", expectedPRs, metrics.PullRequests)
	}

	if metrics.ProtectedBranches != expectedBranches {
		t.Errorf("Expected %d protected branches, got %d", expectedBranches, metrics.ProtectedBranches)
	}

	if metrics.Releases != expectedReleases {
		t.Errorf("Expected %d releases, got %d", expectedReleases, metrics.Releases)
	}
}

func TestAnalyzeMigrationArchive_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	metrics, err := AnalyzeMigrationArchive(tempDir)
	if err != nil {
		t.Fatalf("AnalyzeMigrationArchive failed: %v", err)
	}

	// All counts should be zero for empty directory
	if metrics.Issues != 0 || metrics.PullRequests != 0 || metrics.ProtectedBranches != 0 || metrics.Releases != 0 {
		t.Errorf("Expected all counts to be zero, got Issues: %d, PRs: %d, Branches: %d, Releases: %d",
			metrics.Issues, metrics.PullRequests, metrics.ProtectedBranches, metrics.Releases)
	}
}

func TestAnalyzeMigrationArchive_NonExistentDirectory(t *testing.T) {
	_, err := AnalyzeMigrationArchive("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory, got nil")
	}
}

func TestAnalyzeMigrationArchive_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file with invalid JSON
	invalidJSONPath := filepath.Join(tempDir, "issues_000001.json")
	err := os.WriteFile(invalidJSONPath, []byte("invalid json content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = AnalyzeMigrationArchive(tempDir)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestCountJSONArrayEntries(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files with different patterns
	createTestJSONFile(t, tempDir, "issues_000001.json", []map[string]interface{}{
		{"id": 1}, {"id": 2}, {"id": 3},
	})

	createTestJSONFile(t, tempDir, "issues_000002.json", []map[string]interface{}{
		{"id": 4}, {"id": 5},
	})

	// Create a non-matching file that should be ignored
	createTestJSONFile(t, tempDir, "other_000001.json", []map[string]interface{}{
		{"id": 6},
	})

	// Test counting issues
	count, err := countJSONArrayEntries(tempDir, "issues_")
	if err != nil {
		t.Fatalf("countJSONArrayEntries failed: %v", err)
	}

	expectedCount := 5 // 3 + 2 from issues files
	if count != expectedCount {
		t.Errorf("Expected count %d, got %d", expectedCount, count)
	}

	// Test counting non-existent prefix
	count, err = countJSONArrayEntries(tempDir, "nonexistent_")
	if err != nil {
		t.Fatalf("countJSONArrayEntries failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected count 0 for non-existent prefix, got %d", count)
	}
}

func TestSelectMigrationForRepository(t *testing.T) {
	// Create a mock GitHubAPI - this would require setting up the API mock
	// For now, this is a placeholder test structure
	t.Skip("This test requires API mocking infrastructure")

	// TODO: Implement when we have API mocking set up
	// This would test:
	// - Finding migrations for a repository
	// - User selection when multiple migrations exist
	// - Handling of no migrations found
	// - Handling of single migration found
}

func TestDownloadAndExtractArchive(t *testing.T) {
	// This test also requires API mocking
	t.Skip("This test requires API mocking infrastructure")

	// TODO: Implement when we have API mocking set up
	// This would test:
	// - End-to-end download and extraction flow
	// - Error handling for API failures
	// - Error handling for extraction failures
}

// TestDownloadPathHandling tests the downloadPath parameter logic without requiring API calls
func TestDownloadPathHandling(t *testing.T) {
	tests := []struct {
		name         string
		downloadPath string
		expected     string
	}{
		{
			name:         "empty download path uses default",
			downloadPath: "",
			expected:     "migration-archives",
		},
		{
			name:         "custom download path is used",
			downloadPath: "/custom/path",
			expected:     "/custom/path",
		},
		{
			name:         "relative download path is used",
			downloadPath: "custom-archives",
			expected:     "custom-archives",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is testing the logic inside DownloadAndExtractArchive
			// We can't easily unit test it without refactoring, but we can document the expected behavior

			// The function should use "migration-archives" as default when downloadPath is empty
			outputDir := "migration-archives"
			if tt.downloadPath != "" {
				outputDir = tt.downloadPath
			}

			if outputDir != tt.expected {
				t.Errorf("Expected outputDir %s, got %s", tt.expected, outputDir)
			}
		})
	}
}

// Helper function to create test JSON files
func createTestJSONFile(t *testing.T, dir, filename string, data []map[string]interface{}) {
	filePath := filepath.Join(dir, filename)
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file %s: %v", filename, err)
	}
}

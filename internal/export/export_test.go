package export

import (
	"encoding/csv"
	"encoding/json"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestExportData creates sample export data for testing
func createTestExportData() ExportData {
	return ExportData{
		ExportTimestamp: time.Date(2025, 10, 2, 14, 30, 0, 0, time.UTC),
		Repository: validator.RepositoryData{
			Owner:           "test-owner",
			Name:            "test-repo",
			Issues:          42,
			PRs:             &api.PRCounts{Open: 5, Closed: 10, Merged: 15, Total: 30},
			Tags:            8,
			Releases:        3,
			CommitCount:     150,
			LatestCommitSHA: "abc123def456",
		},
	}
}

// Note: Full integration tests for ExportSourceData would require API mocking
// The following tests focus on the individual components that can be tested in isolation

func TestGenerateExportFileName(t *testing.T) {
	testCases := []struct {
		owner     string
		repo      string
		format    string
		timestamp time.Time
		expected  string
	}{
		{
			owner:     "myorg",
			repo:      "myrepo",
			format:    "json",
			timestamp: time.Date(2025, 10, 2, 14, 30, 45, 0, time.UTC),
			expected:  filepath.Join(".exports", "myorg_myrepo_export_20251002_143045.json"),
		},
		{
			owner:     "github",
			repo:      "docs",
			format:    "csv",
			timestamp: time.Date(2025, 12, 25, 9, 15, 30, 0, time.UTC),
			expected:  filepath.Join(".exports", "github_docs_export_20251225_091530.csv"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := generateExportFileName(tc.owner, tc.repo, tc.format, tc.timestamp)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestExportToJSON(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.json")

	// Create test data
	exportData := createTestExportData()

	// Test the function
	err := exportToJSON(exportData, filename)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file exists and content is valid JSON
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var parsed ExportData
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify data integrity
	if parsed.Repository.Owner != exportData.Repository.Owner {
		t.Errorf("Expected owner %s, got %s", exportData.Repository.Owner, parsed.Repository.Owner)
	}
	if parsed.Repository.Name != exportData.Repository.Name {
		t.Errorf("Expected name %s, got %s", exportData.Repository.Name, parsed.Repository.Name)
	}
	if parsed.Repository.Issues != exportData.Repository.Issues {
		t.Errorf("Expected %d issues, got %d", exportData.Repository.Issues, parsed.Repository.Issues)
	}
}

func TestExportToCSV(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.csv")

	// Create test data
	exportData := createTestExportData()

	// Test the function
	err := exportToCSV(exportData, filename)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file exists and content is valid CSV
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read CSV: %v", err)
	}

	// Should have header + 1 data row
	if len(records) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(records))
	}

	// Verify some key data points
	dataRow := records[1]
	if dataRow[1] != "test-owner" {
		t.Errorf("Expected owner test-owner, got %s", dataRow[1])
	}
	if dataRow[2] != "test-repo" {
		t.Errorf("Expected repo test-repo, got %s", dataRow[2])
	}
	if dataRow[3] != "42" {
		t.Errorf("Expected 42 issues, got %s", dataRow[3])
	}
}

func TestExportToJSON_CreateDirectory(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Use a nested path that doesn't exist
	filename := filepath.Join(tmpDir, "nested", "dir", "test.json")

	exportData := ExportData{
		ExportTimestamp: time.Now(),
		Repository: validator.RepositoryData{
			Owner:           "test",
			Name:            "repo",
			Issues:          5,
			PRs:             &api.PRCounts{Open: 1, Closed: 2, Merged: 3, Total: 6},
			Tags:            2,
			Releases:        1,
			CommitCount:     50,
			LatestCommitSHA: "test123",
		},
	}

	// Test the function
	err := exportToJSON(exportData, filename)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file was created in the nested directory
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Expected file to be created at %s", filename)
	}
}

func TestExportToCSV_CreateDirectory(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Use a nested path that doesn't exist
	filename := filepath.Join(tmpDir, "nested", "dir", "test.csv")

	exportData := ExportData{
		ExportTimestamp: time.Now(),
		Repository: validator.RepositoryData{
			Owner:           "test",
			Name:            "repo",
			Issues:          5,
			PRs:             &api.PRCounts{Open: 1, Closed: 2, Merged: 3, Total: 6},
			Tags:            2,
			Releases:        1,
			CommitCount:     50,
			LatestCommitSHA: "test123",
		},
	}

	// Test the function
	err := exportToCSV(exportData, filename)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file was created in the nested directory
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Expected file to be created at %s", filename)
	}
}

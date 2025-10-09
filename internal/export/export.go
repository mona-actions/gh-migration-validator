package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// ExportData represents the exported repository data with metadata
type ExportData struct {
	ExportTimestamp time.Time                `json:"export_timestamp"`
	Repository      validator.RepositoryData `json:"repository_data"`
}

// ExportSourceData exports source repository data at a point in time
// Takes a validator instance to leverage existing data retrieval functionality
func ExportSourceData(mv *validator.MigrationValidator, owner, repoName, format, outputFile string, timestamp time.Time) error {
	fmt.Println("Starting source repository data export...")
	fmt.Printf("Repository: %s/%s\n", owner, repoName)

	// Create a spinner for the export process
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Preparing to export data from %s/%s...", owner, repoName))

	// Use the validator to retrieve source repository data
	err := mv.RetrieveSourceData(owner, repoName, spinner)
	if err != nil {
		return fmt.Errorf("failed to retrieve source data for export: %w", err)
	}

	// Prepare export data
	exportData := ExportData{
		ExportTimestamp: timestamp,
		Repository:      *mv.SourceData,
	}

	// Generate output filename if not provided
	if outputFile == "" {
		outputFile = generateExportFileName(owner, repoName, format, timestamp)
	}

	// Export based on format
	switch strings.ToLower(format) {
	case "json":
		err = exportToJSON(exportData, outputFile)
	case "csv":
		err = exportToCSV(exportData, outputFile)
	default:
		return fmt.Errorf("unsupported format: %s. Supported formats: json, csv", format)
	}

	if err != nil {
		return fmt.Errorf("failed to export data: %w", err)
	}

	spinner.Success(fmt.Sprintf("Export completed successfully: %s", outputFile))
	fmt.Println()
	return nil
}

// generateExportFileName creates a default filename for the export in a .exports directory
func generateExportFileName(owner, repo, format string, timestamp time.Time) string {
	timestampStr := timestamp.Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_export_%s.%s", owner, repo, timestampStr, format)
	return filepath.Join(".exports", filename)
}

// exportToJSON exports data to JSON format
func exportToJSON(data ExportData, filename string) error {
	// Create directory if it doesn't exist
	if dir := filepath.Dir(filename); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// exportToCSV exports data to CSV format
func exportToCSV(data ExportData, filename string) error {
	// Create directory if it doesn't exist
	if dir := filepath.Dir(filename); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	header := []string{
		"export_timestamp",
		"owner",
		"repository_name",
		"issues_count",
		"pull_requests_open",
		"pull_requests_closed",
		"pull_requests_merged",
		"pull_requests_total",
		"tags_count",
		"releases_count",
		"commits_count",
		"latest_commit_sha",
		"branch_protection_rules_count",
		"rulesets_count",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data row
	prOpen, prClosed, prMerged, prTotal := "0", "0", "0", "0"
	if data.Repository.PRs != nil {
		prOpen = fmt.Sprintf("%d", data.Repository.PRs.Open)
		prClosed = fmt.Sprintf("%d", data.Repository.PRs.Closed)
		prMerged = fmt.Sprintf("%d", data.Repository.PRs.Merged)
		prTotal = fmt.Sprintf("%d", data.Repository.PRs.Total)
	}

	record := []string{
		data.ExportTimestamp.Format(time.RFC3339),
		data.Repository.Owner,
		data.Repository.Name,
		fmt.Sprintf("%d", data.Repository.Issues),
		prOpen,
		prClosed,
		prMerged,
		prTotal,
		fmt.Sprintf("%d", data.Repository.Tags),
		fmt.Sprintf("%d", data.Repository.Releases),
		fmt.Sprintf("%d", data.Repository.CommitCount),
		data.Repository.LatestCommitSHA,
		fmt.Sprintf("%d", data.Repository.BranchProtectionRules),
		fmt.Sprintf("%d", data.Repository.Rulesets),
	}
	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write CSV record: %w", err)
	}

	return nil
}

// LoadExportData loads and validates export data from a JSON file
func LoadExportData(filename string) (*ExportData, error) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, fmt.Errorf("export file does not exist: %s", filename)
	}

	// Read file content
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read export file: %w", err)
	}

	// Parse JSON
	var exportData ExportData
	if err := json.Unmarshal(content, &exportData); err != nil {
		return nil, fmt.Errorf("failed to parse export JSON: %w", err)
	}

	// Validate required fields
	if err := validateExportData(&exportData); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}

	return &exportData, nil
}

// validateExportData ensures the export data has all required fields
func validateExportData(data *ExportData) error {
	if data.ExportTimestamp.IsZero() {
		return fmt.Errorf("export timestamp is missing or invalid")
	}

	if data.Repository.Owner == "" {
		return fmt.Errorf("repository owner is missing")
	}

	if data.Repository.Name == "" {
		return fmt.Errorf("repository name is missing")
	}

	// Note: We don't validate PRs for nil here since we handle that gracefully in the export functions
	// This allows for flexibility in case the export format evolves

	return nil
}

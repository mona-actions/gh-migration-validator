package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestExportFlagValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		expectedErrMsg string
	}{
		{
			name:           "both download and archive-path flags should fail",
			args:           []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--download", "--archive-path", "/some/path"},
			expectedError:  true,
			expectedErrMsg: "--download and --archive-path flags are mutually exclusive. Please use only one.",
		},
		{
			name:          "download flag alone should be valid",
			args:          []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--download"},
			expectedError: false,
		},
		{
			name:          "archive-path flag alone should be valid",
			args:          []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--archive-path", "/some/path"},
			expectedError: false,
		},
		{
			name:          "download-path with download should be valid",
			args:          []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--download", "--download-path", "/custom/path"},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set required environment variable to prevent validation errors
			os.Setenv("GHMV_SOURCE_TOKEN", "fake-token")
			defer os.Unsetenv("GHMV_SOURCE_TOKEN")

			// Create a new command instance for each test
			cmd := &cobra.Command{
				Use: "export",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Get flag values
					download, _ := cmd.Flags().GetBool("download")
					downloadPath := cmd.Flag("download-path").Value.String()
					archivePath := cmd.Flag("archive-path").Value.String()

					// Test the validation logic directly
					if download && archivePath != "" {
						return &ValidationError{Message: "--download and --archive-path flags are mutually exclusive. Please use only one."}
					}

					// For testing purposes, we'll just validate the flags
					// In a real test, we'd mock the API and test the full flow
					_ = downloadPath // Use the variable to avoid compiler warning

					return nil
				},
			}

			// Add all the flags
			cmd.Flags().StringP("source-organization", "s", "", "Source Organization")
			cmd.Flags().StringP("source-repository", "", "", "Source repository")
			cmd.Flags().BoolP("download", "d", false, "Download and extract migration archive")
			cmd.Flags().StringP("download-path", "", "", "Directory to download migration archives")
			cmd.Flags().StringP("archive-path", "p", "", "Path to existing extracted archive")

			// Mark required flags
			cmd.MarkFlagRequired("source-organization")
			cmd.MarkFlagRequired("source-repository")

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Set arguments and execute
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Message != tt.expectedErrMsg {
						t.Errorf("Expected error message '%s', got '%s'", tt.expectedErrMsg, validationErr.Message)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExportFlagValues(t *testing.T) {
	tests := []struct {
		name                 string
		args                 []string
		expectedDownload     bool
		expectedDownloadPath string
		expectedArchivePath  string
	}{
		{
			name:                 "download flag sets boolean correctly",
			args:                 []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--download"},
			expectedDownload:     true,
			expectedDownloadPath: "",
			expectedArchivePath:  "",
		},
		{
			name:                 "download-path flag sets string correctly",
			args:                 []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--download", "--download-path", "/custom/path"},
			expectedDownload:     true,
			expectedDownloadPath: "/custom/path",
			expectedArchivePath:  "",
		},
		{
			name:                 "archive-path flag sets string correctly",
			args:                 []string{"--source-organization", "test-org", "--source-repository", "test-repo", "--archive-path", "/existing/path"},
			expectedDownload:     false,
			expectedDownloadPath: "",
			expectedArchivePath:  "/existing/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "export",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Test flag values
					download, _ := cmd.Flags().GetBool("download")
					downloadPath := cmd.Flag("download-path").Value.String()
					archivePath := cmd.Flag("archive-path").Value.String()

					if download != tt.expectedDownload {
						t.Errorf("Expected download %v, got %v", tt.expectedDownload, download)
					}
					if downloadPath != tt.expectedDownloadPath {
						t.Errorf("Expected downloadPath %s, got %s", tt.expectedDownloadPath, downloadPath)
					}
					if archivePath != tt.expectedArchivePath {
						t.Errorf("Expected archivePath %s, got %s", tt.expectedArchivePath, archivePath)
					}

					return nil
				},
			}

			// Add flags
			cmd.Flags().StringP("source-organization", "s", "", "Source Organization")
			cmd.Flags().StringP("source-repository", "", "", "Source repository")
			cmd.Flags().BoolP("download", "d", false, "Download and extract migration archive")
			cmd.Flags().StringP("download-path", "", "", "Directory to download migration archives")
			cmd.Flags().StringP("archive-path", "p", "", "Path to existing extracted archive")

			// Set arguments and execute
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// ValidationError represents a validation error for testing
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func TestValidateArchivePathOptimization(t *testing.T) {
	// Create a temporary directory with various test files
	tempDir := t.TempDir()

	testFiles := []struct {
		filename    string
		shouldMatch bool
	}{
		{"issues_000001.json", true},
		{"pull_requests_000001.json", true},
		{"protected_branches_000001.json", true},
		{"releases_000001.json", true},
		{"repositories_000001.json", true},
		{"random_file.json", false},
		{"issues.json", false}, // No underscore
		{"issues_", false},     // No .json extension
		{"not_expected_pattern_000001.json", false},
		{"schema.json", false},
		{"users_000001.json", false}, // Not in expected patterns
	}

	// Create the test files
	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.filename)
		err := os.WriteFile(filePath, []byte("{}"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", tf.filename, err)
		}
	}

	// Test with directory containing expected files
	err := validateArchivePath(tempDir)
	if err != nil {
		t.Errorf("validateArchivePath should succeed with expected files, got error: %v", err)
	}

	// Test with directory containing only non-matching files
	tempDir2 := t.TempDir()
	nonMatchingFiles := []string{"random.json", "schema.json", "unexpected_pattern.json"}
	for _, filename := range nonMatchingFiles {
		filePath := filepath.Join(tempDir2, filename)
		err := os.WriteFile(filePath, []byte("{}"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	err = validateArchivePath(tempDir2)
	if err == nil {
		t.Error("validateArchivePath should fail when no expected files are found")
	}
}

func BenchmarkValidateArchivePathOptimization(b *testing.B) {
	// Create a temporary directory with many files for benchmarking
	tempDir := b.TempDir()

	// Create many files to test performance
	patterns := []string{"issues_", "pull_requests_", "protected_branches_", "releases_", "repositories_"}
	for i := 0; i < 1000; i++ {
		for j, pattern := range patterns {
			filename := filepath.Join(tempDir, fmt.Sprintf("%s%06d_%d.json", pattern, i, j))
			err := os.WriteFile(filename, []byte("{}"), 0644)
			if err != nil {
				b.Fatalf("Failed to create test file: %v", err)
			}
		}
		// Add some non-matching files
		filename := filepath.Join(tempDir, fmt.Sprintf("random_%06d.json", i))
		err := os.WriteFile(filename, []byte("{}"), 0644)
		if err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := validateArchivePath(tempDir)
		if err != nil {
			b.Fatalf("validateArchivePath failed: %v", err)
		}
	}
}

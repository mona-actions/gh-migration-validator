/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/export"
	"mona-actions/gh-migration-validator/internal/migrationarchive"
	"mona-actions/gh-migration-validator/internal/validator"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export source repository data at a point in time",
	Long: `Export repository data from the source organization at the current point in time.

This command fetches and exports repository metadata including:
- Issues count
- Pull requests count (open, closed, merged)
- Tags count
- Releases count
- Commits count
- Latest commit hash

The data can be exported in JSON or CSV format with a timestamp.

Optionally, you can include migration archive data in the export by either:
- Using --download to automatically download and extract a migration archive
- Using --archive-path to specify an existing extracted migration archive directory

When using --download, you can optionally specify --download-path to choose where 
the archive files are saved (defaults to ./migration-archives).

The tool will automatically search for migrations containing the specified repository
and allow you to select from multiple matches if available when downloading.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get parameters from flags
		sourceOrganization := cmd.Flag("source-organization").Value.String()
		sourceToken := cmd.Flag("source-token").Value.String()
		ghHostname := cmd.Flag("source-hostname").Value.String()
		sourceRepo := cmd.Flag("source-repository").Value.String()
		outputFormat := cmd.Flag("format").Value.String()
		outputFile := cmd.Flag("output").Value.String()
		download, _ := cmd.Flags().GetBool("download")
		downloadPath := cmd.Flag("download-path").Value.String()
		archivePath := cmd.Flag("archive-path").Value.String()

		// Only set ENV variables if flag values are provided (not empty)
		if sourceOrganization != "" {
			os.Setenv("GHMV_SOURCE_ORGANIZATION", sourceOrganization)
		}
		if sourceToken != "" {
			os.Setenv("GHMV_SOURCE_TOKEN", sourceToken)
		}
		if ghHostname != "" {
			os.Setenv("GHMV_SOURCE_HOSTNAME", ghHostname)
		}
		if sourceRepo != "" {
			os.Setenv("GHMV_SOURCE_REPO", sourceRepo)
		}

		// Bind ENV variables in Viper
		viper.BindEnv("SOURCE_ORGANIZATION")
		viper.BindEnv("SOURCE_TOKEN")
		viper.BindEnv("SOURCE_HOSTNAME")
		viper.BindEnv("SOURCE_PRIVATE_KEY")
		viper.BindEnv("SOURCE_APP_ID")
		viper.BindEnv("SOURCE_INSTALLATION_ID")
		viper.BindEnv("SOURCE_REPO")

		// Validate required variables for export
		if err := checkExportVars(); err != nil {
			fmt.Printf("Export configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Initialize API with source-only clients
		ghAPI, err := api.NewSourceOnlyAPI()
		if err != nil {
			fmt.Printf("Failed to initialize source API: %v\n", err)
			os.Exit(1)
		}
		// Validate that --download and --archive-path are mutually exclusive
		if download && archivePath != "" {
			fmt.Printf("Error: --download and --archive-path flags are mutually exclusive. Please use only one.\n")
			os.Exit(1)
		}

		// Create validator
		migrationValidator := validator.New(ghAPI)

		// Handle migration archive (either download or use existing path)
		var archiveDir string
		if download {
			fmt.Println("Searching for migration archives...")
			extractedPath, err := migrationarchive.DownloadAndExtractArchive(ghAPI, sourceOrganization, sourceRepo, downloadPath)
			if err != nil {
				fmt.Printf("Migration archive download failed: %v\n", err)
				os.Exit(1)
			}
			archiveDir = extractedPath
		} else if archivePath != "" {
			// Validate that the specified archive path exists and is a directory
			if err := validateArchivePath(archivePath); err != nil {
				fmt.Printf("Archive path validation failed: %v\n", err)
				os.Exit(1)
			}
			archiveDir = archivePath
			fmt.Printf("Using existing migration archive at: %s\n", archivePath)
		}

		// Export the source repository data (with optional migration archive analysis)
		timestamp := time.Now()
		err = export.ExportSourceData(migrationValidator, sourceOrganization, sourceRepo, outputFormat, outputFile, timestamp, archiveDir)
		if err != nil {
			fmt.Printf("Export failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// Add export command to root
	rootCmd.AddCommand(exportCmd)

	// Define flags specific to export command
	exportCmd.Flags().StringP("source-organization", "s", "", "Source Organization to export data from")
	exportCmd.MarkFlagRequired("source-organization")

	exportCmd.Flags().StringP("source-token", "a", "", "Source Organization GitHub token. Scopes: read:org, read:user, user:email")

	exportCmd.Flags().StringP("source-hostname", "u", "", "GitHub Enterprise source hostname url (optional) Ex. https://github.example.com")

	exportCmd.Flags().StringP("source-repository", "", "", "Source repository name to export (just the repo name, not owner/repo)")
	exportCmd.MarkFlagRequired("source-repository")

	exportCmd.Flags().StringP("format", "f", "json", "Output format: json or csv")

	exportCmd.Flags().StringP("output", "o", "", "Output file path (if not provided, will use default naming)")

	exportCmd.Flags().BoolP("download", "d", false, "Download and extract migration archive for the specified repository")

	exportCmd.Flags().StringP("download-path", "", "", "Directory to download migration archives to (default: ./migration-archives)")

	exportCmd.Flags().StringP("archive-path", "p", "", "Path to an existing extracted migration archive directory (alternative to --download)")
}

// checkExportVars validates the configuration for export command
func checkExportVars() error {
	// Check for source token
	sourceToken := viper.GetString("SOURCE_TOKEN")
	if sourceToken == "" {
		return fmt.Errorf("source token is required. Set it via --source-token flag or GHMV_SOURCE_TOKEN environment variable")
	}

	// Check source repository
	sourceRepo := viper.GetString("SOURCE_REPO")
	if sourceRepo == "" {
		return fmt.Errorf("source repository is required. Set it via --source-repository flag")
	}

	return nil
}

// validateArchivePath validates that the provided archive path exists and contains expected migration archive files
func validateArchivePath(archivePath string) error {
	// Check if the path exists
	info, err := os.Stat(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("archive path does not exist: %s", archivePath)
		}
		return fmt.Errorf("error accessing archive path: %v", err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("archive path must be a directory: %s", archivePath)
	}

	// Check if directory contains expected migration archive files
	entries, err := os.ReadDir(archivePath)
	if err != nil {
		return fmt.Errorf("error reading archive directory: %v", err)
	}

	// Look for at least one expected migration archive file pattern
	// Using a map for O(1) pattern lookups instead of nested O(n*m) loops
	expectedPatterns := map[string]bool{
		"issues_":             true,
		"pull_requests_":      true,
		"protected_branches_": true,
		"releases_":           true,
		"repositories_":       true,
	}
	foundExpectedFile := false

	for _, entry := range entries {
		if entry.IsDir() || foundExpectedFile {
			continue
		}
		fileName := entry.Name()
		if strings.HasSuffix(fileName, ".json") {
			// Find the underscore position to extract the potential prefix
			if underscoreIdx := strings.Index(fileName, "_"); underscoreIdx > 0 {
				prefix := fileName[:underscoreIdx+1] // Include the underscore
				if expectedPatterns[prefix] {
					foundExpectedFile = true
				}
			}
		}
	}

	if !foundExpectedFile {
		return fmt.Errorf("directory does not appear to contain migration archive files (expected files like issues_*.json, pull_requests_*.json, etc.): %s", archivePath)
	}

	return nil
}

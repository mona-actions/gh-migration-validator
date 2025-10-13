package migrationarchive

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/archive"
)

// SelectMigrationForRepository finds and selects a migration containing the specified repository
func SelectMigrationForRepository(githubAPI *api.GitHubAPI, org, repoName string) (int64, error) {
	// Find migrations containing the target repository
	fmt.Printf("Searching for migrations containing repository '%s'...\n", repoName)
	matchingMigrations, err := githubAPI.FindMigrationsByRepository(api.SourceClient, org, repoName)
	if err != nil {
		return 0, fmt.Errorf("failed to search for migrations: %v", err)
	}

	if len(matchingMigrations) == 0 {
		return 0, fmt.Errorf("no migrations found containing repository '%s' in organization %s", repoName, org)
	}

	if len(matchingMigrations) == 1 {
		// Only one migration found, use it automatically
		migrationID := matchingMigrations[0].ID
		fmt.Printf("Found one migration containing '%s': ID %d (created: %s)\n",
			repoName, migrationID, matchingMigrations[0].CreatedAt)
		return migrationID, nil
	}

	// Multiple migrations found, let user choose
	fmt.Printf("Found %d migrations containing repository '%s':\n\n", len(matchingMigrations), repoName)

	for i, migration := range matchingMigrations {
		fmt.Printf("%d. Migration ID: %d\n", i+1, migration.ID)
		fmt.Printf("   Created: %s\n", migration.CreatedAt)
		fmt.Printf("   Updated: %s\n", migration.UpdatedAt)
		fmt.Printf("   State: %s\n", migration.State)
		fmt.Printf("   Repositories (%d): %s\n\n",
			len(migration.Repositories), strings.Join(migration.Repositories, ", "))
	}

	// Get user selection
	fmt.Printf("Please select a migration (1-%d): ", len(matchingMigrations))
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("failed to read user input: %v", err)
	}

	selection, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || selection < 1 || selection > len(matchingMigrations) {
		return 0, fmt.Errorf("invalid selection. Please enter a number between 1 and %d", len(matchingMigrations))
	}

	migrationID := matchingMigrations[selection-1].ID
	fmt.Printf("Selected migration ID: %d\n", migrationID)
	return migrationID, nil
}

// DownloadAndExtractArchive downloads and extracts a migration archive for the specified repository
func DownloadAndExtractArchive(githubAPI *api.GitHubAPI, org, repoName string) error {
	// Select the appropriate migration for this repository
	migrationID, err := SelectMigrationForRepository(githubAPI, org, repoName)
	if err != nil {
		return err
	}

	// Generate output file path
	outputDir := "migration-archive"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	archivePath := filepath.Join(outputDir, fmt.Sprintf("migration-%d.tar.gz", migrationID))

	// Download the archive
	fmt.Printf("Downloading migration archive %d to %s...\n", migrationID, archivePath)
	downloadedPath, err := githubAPI.DownloadMigrationArchive(api.SourceClient, org, migrationID, archivePath)
	if err != nil {
		return fmt.Errorf("failed to download migration archive: %v", err)
	}

	fmt.Printf("Archive downloaded successfully to: %s\n", downloadedPath)

	// Extract the archive
	extractPath := archive.GetArchiveDestination(downloadedPath)
	fmt.Printf("Extracting archive to: %s...\n", extractPath)

	err = archive.ExtractTarGz(downloadedPath, extractPath)
	if err != nil {
		return fmt.Errorf("failed to extract archive: %v", err)
	}

	fmt.Printf("Archive extracted successfully to: %s\n", extractPath)

	return nil
}
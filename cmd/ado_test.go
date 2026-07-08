package cmd

import (
	"os"
	"strings"
	"testing"

	"mona-actions/gh-migration-validator/internal/validator"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// resetADOViperAndEnv clears Viper state and ADO-related environment variables between tests.
func resetADOViperAndEnv() {
	viper.Reset()

	envVars := []string{
		"GHMV_ADO_SERVER_URL",
		"GHMV_ADO_ORG",
		"GHMV_ADO_TEAM_PROJECT",
		"GHMV_ADO_REPO",
		"GHMV_ADO_PAT",
		"GHMV_TARGET_ORGANIZATION",
		"GHMV_TARGET_TOKEN",
		"GHMV_TARGET_HOSTNAME",
		"GHMV_TARGET_REPO",
		"GHMV_MARKDOWN_TABLE",
		"GHMV_MARKDOWN_FILE",
		"GHMV_NO_LFS",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

// setupADOViperWithFlags binds ADO command flags to Viper for testing.
func setupADOViperWithFlags(cmd *cobra.Command) {
	viper.SetEnvPrefix("GHMV")
	viper.AutomaticEnv()

	viper.BindPFlag("ADO_SERVER_URL", cmd.Flags().Lookup("ado-server-url"))
	viper.BindPFlag("ADO_ORG", cmd.Flags().Lookup("ado-org"))
	viper.BindPFlag("ADO_TEAM_PROJECT", cmd.Flags().Lookup("ado-team-project"))
	viper.BindPFlag("ADO_REPO", cmd.Flags().Lookup("ado-repo"))
	viper.BindPFlag("ADO_PAT", cmd.Flags().Lookup("ado-pat"))
	viper.BindPFlag("TARGET_ORGANIZATION", cmd.Flags().Lookup("github-target-org"))
	viper.BindPFlag("TARGET_TOKEN", cmd.Flags().Lookup("github-target-pat"))
	viper.BindPFlag("TARGET_HOSTNAME", cmd.Flags().Lookup("target-hostname"))
	viper.BindPFlag("TARGET_REPO", cmd.Flags().Lookup("target-repo"))
	viper.BindPFlag("MARKDOWN_TABLE", cmd.Flags().Lookup("markdown-table"))
	viper.BindPFlag("MARKDOWN_FILE", cmd.Flags().Lookup("markdown-file"))
}

// createADOTestCommand creates a fresh command with all ADO flags for testing.
func createADOTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test-ado",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkADOVars()
		},
	}

	// ADO-specific flags (aligned with GEI ado2gh)
	cmd.Flags().StringP("ado-server-url", "H", "", "Azure DevOps Server URL")
	cmd.Flags().StringP("ado-org", "o", "", "Azure DevOps organization (collection)")
	cmd.Flags().StringP("ado-team-project", "P", "", "Azure DevOps team project")
	cmd.Flags().StringP("ado-repo", "r", "", "Azure DevOps repository name")
	cmd.Flags().StringP("ado-pat", "k", "", "Azure DevOps personal access token")

	// GitHub target flags
	cmd.Flags().StringP("github-target-org", "t", "", "Target GitHub organization")
	cmd.Flags().StringP("github-target-pat", "b", "", "Target Organization GitHub token")
	cmd.Flags().StringP("target-hostname", "v", "", "GitHub Enterprise target hostname url")
	cmd.Flags().String("target-repo", "", "Target repository name")

	// Output flags
	cmd.Flags().BoolP("markdown-table", "m", false, "Output results in markdown table format")
	cmd.Flags().String("markdown-file", "", "Write markdown output to the specified file")

	return cmd
}

// --- Single-repo mode: all vars provided ---

func TestCheckADOVars_SingleRepoAllProvided(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_REPO", "my-repo")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
	os.Setenv("GHMV_TARGET_REPO", "my-repo")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	if err := checkADOVars(); err != nil {
		t.Errorf("Expected no error when all single-repo vars are provided, got: %v", err)
	}
}

// --- Project mode: all vars provided, no ADO_REPO and no TARGET_REPO ---

func TestCheckADOVars_ProjectModeAllProvided(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	if err := checkADOVars(); err != nil {
		t.Errorf("Expected no error in project mode without TARGET_REPO, got: %v", err)
	}
}

// --- TARGET_REPO required only in single-repo mode ---

func TestCheckADOVars_SingleRepoMissingTargetRepo(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_REPO", "my-repo")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	err := checkADOVars()
	if err == nil {
		t.Error("Expected error when TARGET_REPO is missing in single-repo mode")
	}
	if !strings.Contains(err.Error(), "TARGET_REPO") {
		t.Errorf("Error should mention TARGET_REPO, got: %v", err)
	}
}

// --- Missing individual required fields ---

func TestCheckADOVars_MissingServerURLIsCloud(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	// Omitting the server URL is valid: it targets Azure DevOps Services (cloud).
	os.Setenv("GHMV_ADO_ORG", "my-org")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	if err := checkADOVars(); err != nil {
		t.Errorf("Expected no error when server URL is omitted (cloud), got: %v", err)
	}
}

func TestCheckADOVars_MissingOrg(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	err := checkADOVars()
	if err == nil || !strings.Contains(err.Error(), "ADO_ORG") {
		t.Errorf("Expected ADO_ORG error, got: %v", err)
	}
}

func TestCheckADOVars_MissingTeamProject(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	err := checkADOVars()
	if err == nil || !strings.Contains(err.Error(), "ADO_TEAM_PROJECT") {
		t.Errorf("Expected ADO_TEAM_PROJECT error, got: %v", err)
	}
}

func TestCheckADOVars_MissingPAT(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	err := checkADOVars()
	if err == nil || !strings.Contains(err.Error(), "ADO_PAT") {
		t.Errorf("Expected ADO_PAT error, got: %v", err)
	}
}

func TestCheckADOVars_MissingTargetOrg(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://ado.example.com")
	os.Setenv("GHMV_ADO_ORG", "my-collection")
	os.Setenv("GHMV_ADO_TEAM_PROJECT", "MyProject")
	os.Setenv("GHMV_ADO_PAT", "ado-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	err := checkADOVars()
	if err == nil || !strings.Contains(err.Error(), "TARGET_ORGANIZATION") {
		t.Errorf("Expected TARGET_ORGANIZATION error, got: %v", err)
	}
}

func TestCheckADOVars_NoConfigProvided(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	if err := checkADOVars(); err == nil {
		t.Error("Expected error when no configuration is provided")
	}
}

// --- Viper priority ---

func TestADOViperPriority_FlagTakesPrecedence(t *testing.T) {
	resetADOViperAndEnv()
	defer resetADOViperAndEnv()

	os.Setenv("GHMV_ADO_SERVER_URL", "https://env.example.com")

	cmd := createADOTestCommand()
	setupADOViperWithFlags(cmd)

	cmd.Flags().Set("ado-server-url", "https://flag.example.com")

	if actual := viper.GetString("ADO_SERVER_URL"); actual != "https://flag.example.com" {
		t.Errorf("Expected flag value to take precedence, got %s", actual)
	}
}

// --- Command registration ---

func TestADOCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "ado" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'ado' command to be registered with rootCmd")
	}
}

// --- Repo mapping label ---

func TestRepoMappingLabel(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		target   string
		expected string
	}{
		{
			name:     "same name shows single label",
			source:   "my-repo",
			target:   "my-repo",
			expected: "my-repo",
		},
		{
			name:     "different names show source to target",
			source:   "my-repo",
			target:   "my-repo-migrated",
			expected: "my-repo → my-repo-migrated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := validator.RepoMapping{SourceRepo: tt.source, TargetRepo: tt.target}.Label()
			if label != tt.expected {
				t.Errorf("RepoMapping.Label() = %q, want %q", label, tt.expected)
			}
		})
	}
}

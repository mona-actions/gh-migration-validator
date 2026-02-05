package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// resetViperAndEnv clears Viper state and relevant environment variables between tests
func resetViperAndEnv() {
	// Reset Viper
	viper.Reset()

	// Clear all GHMV_ environment variables
	envVars := []string{
		"GHMV_SOURCE_ORGANIZATION",
		"GHMV_TARGET_ORGANIZATION",
		"GHMV_SOURCE_TOKEN",
		"GHMV_TARGET_TOKEN",
		"GHMV_SOURCE_REPO",
		"GHMV_TARGET_REPO",
		"GHMV_SOURCE_HOSTNAME",
		"GHMV_MARKDOWN_TABLE",
		"GHMV_MARKDOWN_FILE",
		"GHMV_STRICT_EXIT",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

// setupViperWithFlags creates a fresh Viper instance with flag bindings for testing
func setupViperWithFlags(cmd *cobra.Command) {
	viper.SetEnvPrefix("GHMV")
	viper.AutomaticEnv()

	viper.BindPFlag("SOURCE_ORGANIZATION", cmd.Flags().Lookup("github-source-org"))
	viper.BindPFlag("TARGET_ORGANIZATION", cmd.Flags().Lookup("github-target-org"))
	viper.BindPFlag("SOURCE_TOKEN", cmd.Flags().Lookup("github-source-pat"))
	viper.BindPFlag("TARGET_TOKEN", cmd.Flags().Lookup("github-target-pat"))
	viper.BindPFlag("SOURCE_HOSTNAME", cmd.Flags().Lookup("source-hostname"))
	viper.BindPFlag("SOURCE_REPO", cmd.Flags().Lookup("source-repo"))
	viper.BindPFlag("TARGET_REPO", cmd.Flags().Lookup("target-repo"))
	viper.BindPFlag("MARKDOWN_TABLE", cmd.Flags().Lookup("markdown-table"))
	viper.BindPFlag("MARKDOWN_FILE", cmd.Flags().Lookup("markdown-file"))
	viper.BindPFlag("STRICT_EXIT", cmd.Flags().Lookup("strict-exit"))
}

// createTestCommand creates a fresh command with all flags for testing
func createTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkVars()
		},
	}

	cmd.Flags().StringP("github-source-org", "s", "", "Source Organization")
	cmd.Flags().StringP("github-target-org", "t", "", "Target Organization")
	cmd.Flags().StringP("github-source-pat", "a", "", "Source token")
	cmd.Flags().StringP("github-target-pat", "b", "", "Target token")
	cmd.Flags().StringP("source-hostname", "u", "", "Source hostname")
	cmd.Flags().StringP("source-repo", "", "", "Source repo")
	cmd.Flags().StringP("target-repo", "", "", "Target repo")
	cmd.Flags().BoolP("markdown-table", "m", false, "Markdown table")
	cmd.Flags().String("markdown-file", "", "Markdown output file")
	cmd.Flags().Bool("strict-exit", false, "Strict exit")

	return cmd
}

func TestCheckVars_AllEnvVarsProvided(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all required environment variables
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "source-org")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "source-token")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token")
	os.Setenv("GHMV_SOURCE_REPO", "source-repo")
	os.Setenv("GHMV_TARGET_REPO", "target-repo")

	// Setup Viper with a test command
	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// checkVars should pass
	err := checkVars()
	if err != nil {
		t.Errorf("Expected no error when all env vars are provided, got: %v", err)
	}
}

func TestCheckVars_AllFlagsProvided(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Set all flag values
	cmd.SetArgs([]string{
		"--github-source-org", "source-org",
		"--github-target-org", "target-org",
		"--github-source-pat", "source-token",
		"--github-target-pat", "target-token",
		"--source-repo", "source-repo",
		"--target-repo", "target-repo",
	})

	// Parse the flags
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Expected no error when all flags are provided, got: %v", err)
	}
}

func TestCheckVars_MixedFlagsAndEnvVars(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set some via environment variables
	os.Setenv("GHMV_SOURCE_TOKEN", "source-token-from-env")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token-from-env")
	os.Setenv("GHMV_SOURCE_REPO", "source-repo-from-env")
	os.Setenv("GHMV_TARGET_REPO", "target-repo-from-env")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Set remaining via flags
	cmd.SetArgs([]string{
		"--github-source-org", "source-org-from-flag",
		"--github-target-org", "target-org-from-flag",
	})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Expected no error with mixed flags and env vars, got: %v", err)
	}
}

func TestCheckVars_FlagsOverrideEnvVars(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set via environment variable
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "org-from-env")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "token")
	os.Setenv("GHMV_TARGET_TOKEN", "token")
	os.Setenv("GHMV_SOURCE_REPO", "repo")
	os.Setenv("GHMV_TARGET_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Override via flag
	cmd.SetArgs([]string{
		"--github-source-org", "org-from-flag",
	})
	cmd.Execute()

	// Flag should take precedence
	actual := viper.GetString("SOURCE_ORGANIZATION")
	expected := "org-from-flag"
	if actual != expected {
		t.Errorf("Expected flag to override env var. Got %s, want %s", actual, expected)
	}
}

func TestCheckVars_MissingSourceOrg(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all except SOURCE_ORGANIZATION
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "token")
	os.Setenv("GHMV_TARGET_TOKEN", "token")
	os.Setenv("GHMV_SOURCE_REPO", "repo")
	os.Setenv("GHMV_TARGET_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when SOURCE_ORGANIZATION is missing")
	}
	if !strings.Contains(err.Error(), "SOURCE_ORGANIZATION") {
		t.Errorf("Error should mention SOURCE_ORGANIZATION, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--github-source-org") {
		t.Errorf("Error should mention the flag option, got: %v", err)
	}
	if !strings.Contains(err.Error(), "GHMV_SOURCE_ORGANIZATION") {
		t.Errorf("Error should mention the env var option, got: %v", err)
	}
}

func TestCheckVars_MissingSourceToken(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all except SOURCE_TOKEN
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "source-org")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_TARGET_TOKEN", "token")
	os.Setenv("GHMV_SOURCE_REPO", "repo")
	os.Setenv("GHMV_TARGET_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when SOURCE_TOKEN is missing")
	}
	if !strings.Contains(err.Error(), "SOURCE_TOKEN") {
		t.Errorf("Error should mention SOURCE_TOKEN, got: %v", err)
	}
}

func TestCheckVars_MissingTargetToken(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all except TARGET_TOKEN
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "source-org")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "token")
	os.Setenv("GHMV_SOURCE_REPO", "repo")
	os.Setenv("GHMV_TARGET_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when TARGET_TOKEN is missing")
	}
	if !strings.Contains(err.Error(), "TARGET_TOKEN") {
		t.Errorf("Error should mention TARGET_TOKEN, got: %v", err)
	}
}

func TestCheckVars_MissingSourceRepo(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all except SOURCE_REPO
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "source-org")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "token")
	os.Setenv("GHMV_TARGET_TOKEN", "token")
	os.Setenv("GHMV_TARGET_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when SOURCE_REPO is missing")
	}
	if !strings.Contains(err.Error(), "SOURCE_REPO") {
		t.Errorf("Error should mention SOURCE_REPO, got: %v", err)
	}
}

func TestCheckVars_MissingTargetRepo(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set all except TARGET_REPO
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "source-org")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "target-org")
	os.Setenv("GHMV_SOURCE_TOKEN", "token")
	os.Setenv("GHMV_TARGET_TOKEN", "token")
	os.Setenv("GHMV_SOURCE_REPO", "repo")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when TARGET_REPO is missing")
	}
	if !strings.Contains(err.Error(), "TARGET_REPO") {
		t.Errorf("Error should mention TARGET_REPO, got: %v", err)
	}
}

func TestCheckVars_NoConfigProvided(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Error("Expected error when no configuration is provided")
	}
}

func TestViperPriority_FlagTakesPrecedence(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set env var with one value
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "env-value")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Set flag with different value
	cmd.Flags().Set("github-source-org", "flag-value")

	// Viper should return the flag value
	actual := viper.GetString("SOURCE_ORGANIZATION")
	if actual != "flag-value" {
		t.Errorf("Expected flag value to take precedence. Got %s, want 'flag-value'", actual)
	}
}

func TestViperPriority_EnvVarUsedWhenFlagNotSet(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set only env var
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "env-value")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Do not set the flag

	// Viper should return the env var value
	actual := viper.GetString("SOURCE_ORGANIZATION")
	if actual != "env-value" {
		t.Errorf("Expected env var value when flag not set. Got %s, want 'env-value'", actual)
	}
}

func TestMarkdownFile_PriorityFlagBeatsEnv(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	os.Setenv("GHMV_MARKDOWN_FILE", "env.md")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	cmd.Flags().Set("markdown-file", "flag.md")

	actual := viper.GetString("MARKDOWN_FILE")
	if actual != "flag.md" {
		t.Errorf("Expected markdown-file flag to override env. Got %s, want 'flag.md'", actual)
	}
}

func TestMarkdownFile_UsesEnvWhenFlagMissing(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	os.Setenv("GHMV_MARKDOWN_FILE", "env.md")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	actual := viper.GetString("MARKDOWN_FILE")
	if actual != "env.md" {
		t.Errorf("Expected markdown file env to be used when flag missing. Got %s, want 'env.md'", actual)
	}
}

func TestViperPriority_EmptyFlagDoesNotOverrideEnv(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	// Set env var
	os.Setenv("GHMV_SOURCE_ORGANIZATION", "env-value")

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	// Flag is defined but not set (empty default)

	// Viper should return the env var value since flag is empty
	actual := viper.GetString("SOURCE_ORGANIZATION")
	if actual != "env-value" {
		t.Errorf("Expected env var when flag is empty. Got %s, want 'env-value'", actual)
	}
}

func TestErrorMessageFormat(t *testing.T) {
	resetViperAndEnv()
	defer resetViperAndEnv()

	cmd := createTestCommand()
	setupViperWithFlags(cmd)

	err := checkVars()
	if err == nil {
		t.Fatal("Expected an error")
	}

	errMsg := err.Error()

	// Error message should be helpful and indicate both options
	if !strings.Contains(errMsg, "is required") {
		t.Errorf("Error should indicate the field is required: %s", errMsg)
	}
	if !strings.Contains(errMsg, "Set via") {
		t.Errorf("Error should explain how to set the value: %s", errMsg)
	}
	if !strings.Contains(errMsg, "flag or") {
		t.Errorf("Error should mention both flag and env var options: %s", errMsg)
	}
	if !strings.Contains(errMsg, "environment variable") {
		t.Errorf("Error should mention environment variable option: %s", errMsg)
	}
}

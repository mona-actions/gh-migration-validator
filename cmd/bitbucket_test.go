package cmd

import (
"os"
"strings"
"testing"

"github.com/spf13/cobra"
"github.com/spf13/viper"
)

// resetBBSViperAndEnv clears Viper state and BBS-related environment variables between tests.
// This is separate from resetViperAndEnv in root_test.go to avoid modifying shared test helpers.
func resetBBSViperAndEnv() {
viper.Reset()

envVars := []string{
"GHMV_BBS_SERVER_URL",
"GHMV_BBS_PROJECT",
"GHMV_BBS_REPO",
"GHMV_BBS_TOKEN",
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

// setupBBSViperWithFlags binds BBS command flags to Viper for testing.
// This mirrors setupViperWithFlags in root_test.go but for BBS-specific flags.
func setupBBSViperWithFlags(cmd *cobra.Command) {
viper.SetEnvPrefix("GHMV")
viper.AutomaticEnv()

viper.BindPFlag("BBS_SERVER_URL", cmd.Flags().Lookup("bbs-server-url"))
viper.BindPFlag("BBS_PROJECT", cmd.Flags().Lookup("bbs-project"))
viper.BindPFlag("BBS_REPO", cmd.Flags().Lookup("bbs-repo"))
viper.BindPFlag("BBS_TOKEN", cmd.Flags().Lookup("bbs-token"))
viper.BindPFlag("TARGET_ORGANIZATION", cmd.Flags().Lookup("github-target-org"))
viper.BindPFlag("TARGET_TOKEN", cmd.Flags().Lookup("github-target-pat"))
viper.BindPFlag("TARGET_HOSTNAME", cmd.Flags().Lookup("target-hostname"))
viper.BindPFlag("TARGET_REPO", cmd.Flags().Lookup("target-repo"))
viper.BindPFlag("MARKDOWN_TABLE", cmd.Flags().Lookup("markdown-table"))
viper.BindPFlag("MARKDOWN_FILE", cmd.Flags().Lookup("markdown-file"))
}

// createBBSTestCommand creates a fresh command with all BBS flags for testing.
// This mirrors createTestCommand in root_test.go but for the bitbucket subcommand.
func createBBSTestCommand() *cobra.Command {
cmd := &cobra.Command{
Use: "test-bbs",
RunE: func(cmd *cobra.Command, args []string) error {
return checkBBSVars()
},
}

// Bitbucket-specific flags (aligned with GEI bbs2gh)
cmd.Flags().StringP("bbs-server-url", "H", "", "Bitbucket Server URL")
cmd.Flags().StringP("bbs-project", "p", "", "Bitbucket project key")
cmd.Flags().StringP("bbs-repo", "r", "", "Bitbucket repository slug")
cmd.Flags().StringP("bbs-token", "k", "", "Bitbucket personal access token")

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

// --- Test Case 1: All env vars provided → no error ---

func TestCheckBBSVars_AllEnvVarsProvided(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all required BBS environment variables
os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
os.Setenv("GHMV_BBS_PROJECT", "PROJ")
os.Setenv("GHMV_BBS_REPO", "my-repo")
os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err != nil {
t.Errorf("Expected no error when all env vars are provided, got: %v", err)
}
}

// --- Test Cases 2-6: Missing individual required fields ---

func TestCheckBBSVars_MissingBBSHostname(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all except BBS_SERVER_URL
os.Setenv("GHMV_BBS_PROJECT", "PROJ")
os.Setenv("GHMV_BBS_REPO", "my-repo")
os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when BBS_SERVER_URL is missing")
}
if !strings.Contains(err.Error(), "BBS_SERVER_URL") {
t.Errorf("Error should mention BBS_SERVER_URL, got: %v", err)
}
if !strings.Contains(err.Error(), "--bbs-server-url") {
t.Errorf("Error should mention the flag option, got: %v", err)
}
if !strings.Contains(err.Error(), "GHMV_BBS_SERVER_URL") {
t.Errorf("Error should mention the env var option, got: %v", err)
}
}

func TestCheckBBSVars_MissingBBSProjectKey(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all except BBS_PROJECT
os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
os.Setenv("GHMV_BBS_REPO", "my-repo")
os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when BBS_PROJECT is missing")
}
if !strings.Contains(err.Error(), "BBS_PROJECT") {
t.Errorf("Error should mention BBS_PROJECT, got: %v", err)
}
if !strings.Contains(err.Error(), "--bbs-project") {
t.Errorf("Error should mention the flag option, got: %v", err)
}
if !strings.Contains(err.Error(), "GHMV_BBS_PROJECT") {
t.Errorf("Error should mention the env var option, got: %v", err)
}
}

func TestCheckBBSVars_MissingBBSRepoSlug(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all except BBS_REPO
os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
os.Setenv("GHMV_BBS_PROJECT", "PROJ")
os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when BBS_REPO is missing")
}
if !strings.Contains(err.Error(), "BBS_REPO") {
t.Errorf("Error should mention BBS_REPO, got: %v", err)
}
if !strings.Contains(err.Error(), "--bbs-repo") {
t.Errorf("Error should mention the flag option, got: %v", err)
}
if !strings.Contains(err.Error(), "GHMV_BBS_REPO") {
t.Errorf("Error should mention the env var option, got: %v", err)
}
}

func TestCheckBBSVars_MissingBBSToken(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all except BBS_TOKEN
os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
os.Setenv("GHMV_BBS_PROJECT", "PROJ")
os.Setenv("GHMV_BBS_REPO", "my-repo")
os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when BBS_TOKEN is missing")
}
if !strings.Contains(err.Error(), "BBS_TOKEN") {
t.Errorf("Error should mention BBS_TOKEN, got: %v", err)
}
if !strings.Contains(err.Error(), "--bbs-token") {
t.Errorf("Error should mention the flag option, got: %v", err)
}
if !strings.Contains(err.Error(), "GHMV_BBS_TOKEN") {
t.Errorf("Error should mention the env var option, got: %v", err)
}
}

func TestCheckBBSVars_MissingTargetToken(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set all except TARGET_TOKEN
os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
os.Setenv("GHMV_BBS_PROJECT", "PROJ")
os.Setenv("GHMV_BBS_REPO", "my-repo")
os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")
os.Setenv("GHMV_TARGET_REPO", "my-repo")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when TARGET_TOKEN is missing")
}
if !strings.Contains(err.Error(), "TARGET_TOKEN") {
t.Errorf("Error should mention TARGET_TOKEN, got: %v", err)
}
if !strings.Contains(err.Error(), "--github-target-pat") {
t.Errorf("Error should mention the flag option, got: %v", err)
}
if !strings.Contains(err.Error(), "GHMV_TARGET_TOKEN") {
t.Errorf("Error should mention the env var option, got: %v", err)
}
}

// --- Test Case 7: Missing target organization ---

func TestCheckBBSVars_MissingTargetOrganization(t *testing.T) {
	resetBBSViperAndEnv()
	defer resetBBSViperAndEnv()

	// Set all except TARGET_ORGANIZATION
	os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
	os.Setenv("GHMV_BBS_PROJECT", "PROJ")
	os.Setenv("GHMV_BBS_REPO", "my-repo")
	os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
	os.Setenv("GHMV_TARGET_REPO", "target-repo")

	cmd := createBBSTestCommand()
	setupBBSViperWithFlags(cmd)

	err := checkBBSVars()
	if err == nil {
		t.Error("Expected error when TARGET_ORGANIZATION is missing")
	}
	if !strings.Contains(err.Error(), "TARGET_ORGANIZATION") {
		t.Errorf("Error should mention TARGET_ORGANIZATION, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--github-target-org") {
		t.Errorf("Error should mention the flag option, got: %v", err)
	}
	if !strings.Contains(err.Error(), "GHMV_TARGET_ORGANIZATION") {
		t.Errorf("Error should mention the env var option, got: %v", err)
	}
}

// --- Test Case 8: Missing target repo ---

func TestCheckBBSVars_MissingTargetRepo(t *testing.T) {
	resetBBSViperAndEnv()
	defer resetBBSViperAndEnv()

	// Set all except TARGET_REPO
	os.Setenv("GHMV_BBS_SERVER_URL", "https://bitbucket.example.com")
	os.Setenv("GHMV_BBS_PROJECT", "PROJ")
	os.Setenv("GHMV_BBS_REPO", "my-repo")
	os.Setenv("GHMV_BBS_TOKEN", "bbs-token-value")
	os.Setenv("GHMV_TARGET_TOKEN", "target-token-value")
	os.Setenv("GHMV_TARGET_ORGANIZATION", "my-org")

	cmd := createBBSTestCommand()
	setupBBSViperWithFlags(cmd)

	err := checkBBSVars()
	if err == nil {
		t.Error("Expected error when TARGET_REPO is missing")
	}
	if !strings.Contains(err.Error(), "TARGET_REPO") {
		t.Errorf("Error should mention TARGET_REPO, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--target-repo") {
		t.Errorf("Error should mention the flag option, got: %v", err)
	}
	if !strings.Contains(err.Error(), "GHMV_TARGET_REPO") {
		t.Errorf("Error should mention the env var option, got: %v", err)
	}
}

// --- Test Case 9: No configuration provided ---

func TestCheckBBSVars_NoConfigProvided(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

err := checkBBSVars()
if err == nil {
t.Error("Expected error when no configuration is provided")
}
}

// --- Test Cases 8-9: Viper priority (flag vs env var) ---

func TestBBSViperPriority_FlagTakesPrecedence(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set env var with one value
os.Setenv("GHMV_BBS_SERVER_URL", "https://env-hostname.example.com")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

// Set flag with different value
cmd.Flags().Set("bbs-server-url", "https://flag-hostname.example.com")

// Viper should return the flag value (flag takes precedence over env var)
actual := viper.GetString("BBS_SERVER_URL")
if actual != "https://flag-hostname.example.com" {
t.Errorf("Expected flag value to take precedence. Got %s, want 'https://flag-hostname.example.com'", actual)
}
}

func TestBBSViperPriority_EnvVarUsedWhenFlagNotSet(t *testing.T) {
resetBBSViperAndEnv()
defer resetBBSViperAndEnv()

// Set only env var
os.Setenv("GHMV_BBS_SERVER_URL", "https://env-hostname.example.com")

cmd := createBBSTestCommand()
setupBBSViperWithFlags(cmd)

// Do not set the flag

// Viper should return the env var value since flag is not set
actual := viper.GetString("BBS_SERVER_URL")
if actual != "https://env-hostname.example.com" {
t.Errorf("Expected env var value when flag not set. Got %s, want 'https://env-hostname.example.com'", actual)
}
}

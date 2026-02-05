package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v62/github"
	"github.com/jferrl/go-githubauth"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

// ClientConfig holds all possible configuration options for creating a GitHub client
type ClientConfig struct {
	Token          string
	Hostname       string
	AppID          string
	PrivateKey     []byte
	InstallationID int64
}

// ClientType represents the type of GitHub client to use
type ClientType int

const (
	SourceClient ClientType = iota
	TargetClient
)

// GitHubAPI holds the clients for interacting with GitHub
type GitHubAPI struct {
	sourceClient      *github.Client
	targetClient      *github.Client
	sourceGraphClient *RateLimitAwareGraphQLClient
	targetGraphClient *RateLimitAwareGraphQLClient
}

// Helper functions for config creation
func getSourceConfig() ClientConfig {
	return ClientConfig{
		Token:          viper.GetString("SOURCE_TOKEN"),
		Hostname:       viper.GetString("SOURCE_HOSTNAME"),
		AppID:          viper.GetString("SOURCE_APP_ID"),
		PrivateKey:     []byte(viper.GetString("SOURCE_PRIVATE_KEY")),
		InstallationID: viper.GetInt64("SOURCE_INSTALLATION_ID"),
	}
}

func getTargetConfig() ClientConfig {
	return ClientConfig{
		Token:          viper.GetString("TARGET_TOKEN"),
		Hostname:       viper.GetString("TARGET_HOSTNAME"),
		AppID:          viper.GetString("TARGET_APP_ID"),
		PrivateKey:     []byte(viper.GetString("TARGET_PRIVATE_KEY")),
		InstallationID: viper.GetInt64("TARGET_INSTALLATION_ID"),
	}
}

// NewSourceOnlyAPI creates a GitHubAPI instance with only source clients
func NewSourceOnlyAPI() (*GitHubAPI, error) {
	sourceConfig := getSourceConfig()

	sourceClient, err := newGitHubClient(sourceConfig)
	if err != nil {
		return nil, err
	}

	sourceGraphClient, err := newGitHubGraphQLClient(sourceConfig)
	if err != nil {
		return nil, err
	}

	return &GitHubAPI{
		sourceClient:      sourceClient,
		sourceGraphClient: sourceGraphClient,
		// target clients intentionally nil
	}, nil
}

// NewTargetOnlyAPI creates a GitHubAPI instance with only target clients
func NewTargetOnlyAPI() (*GitHubAPI, error) {
	targetConfig := getTargetConfig()

	targetClient, err := newGitHubClient(targetConfig)
	if err != nil {
		return nil, err
	}

	targetGraphClient, err := newGitHubGraphQLClient(targetConfig)
	if err != nil {
		return nil, err
	}

	return &GitHubAPI{
		targetClient:      targetClient,
		targetGraphClient: targetGraphClient,
		// source clients intentionally nil
	}, nil
}

// NewGitHubAPI creates a GitHubAPI instance with both source and target clients
func NewGitHubAPI() (*GitHubAPI, error) {
	sourceConfig := getSourceConfig()
	targetConfig := getTargetConfig()

	sourceClient, err := newGitHubClient(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create source client: %v", err)
	}

	targetClient, err := newGitHubClient(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create target client: %v", err)
	}

	sourceGraphClient, err := newGitHubGraphQLClient(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create source GraphQL client: %v", err)
	}

	targetGraphClient, err := newGitHubGraphQLClient(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create target GraphQL client: %v", err)
	}

	return &GitHubAPI{
		sourceClient:      sourceClient,
		targetClient:      targetClient,
		sourceGraphClient: sourceGraphClient,
		targetGraphClient: targetGraphClient,
	}, nil
}

// createAuthenticatedClient creates an HTTP client with proper authentication and rate limiting
func createAuthenticatedClient(config ClientConfig) (*http.Client, error) {
	var httpClient *http.Client

	if config.AppID != "" && len(config.PrivateKey) != 0 && config.InstallationID != 0 {
		// GitHub App authentication
		appIDInt, err := strconv.ParseInt(config.AppID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting app ID to int64: %v", err)
		}

		appToken, err := githubauth.NewApplicationTokenSource(appIDInt, config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("error creating app token: %v", err)
		}

		installationToken := githubauth.NewInstallationTokenSource(config.InstallationID, appToken)
		httpClient = oauth2.NewClient(context.Background(), installationToken)
	} else if config.Token != "" {
		// Personal access token authentication
		src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.Token})
		httpClient = oauth2.NewClient(context.Background(), src)
	} else {
		return nil, fmt.Errorf("please provide either a token or GitHub App credentials")
	}

	rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(httpClient.Transport)
	if err != nil {
		return nil, err
	}

	return rateLimiter, nil
}

// newGitHubClient creates a new GitHub REST client based on the provided configuration
func newGitHubClient(config ClientConfig) (*github.Client, error) {
	httpClient, err := createAuthenticatedClient(config)
	if err != nil {
		return nil, err
	}

	client := github.NewClient(httpClient)

	// Configure enterprise URL if hostname is provided
	if config.Hostname != "" {
		hostname := strings.TrimSuffix(config.Hostname, "/")
		if !strings.HasPrefix(hostname, "https://") {
			hostname = "https://" + hostname
		}
		baseURL := fmt.Sprintf("%s/api/v3/", hostname)
		client, err = client.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to configure enterprise URLs: %v", err)
		}
	}

	return client, nil
}

type RateLimitAwareGraphQLClient struct {
	client *githubv4.Client
}

// newGitHubGraphQLClient creates a new GitHub GraphQL client based on the provided configuration
func newGitHubGraphQLClient(config ClientConfig) (*RateLimitAwareGraphQLClient, error) {
	httpClient, err := createAuthenticatedClient(config)
	if err != nil {
		return nil, err
	}

	var baseClient *githubv4.Client

	// If hostname is provided, create enterprise client
	if config.Hostname != "" {
		hostname := strings.TrimSuffix(config.Hostname, "/")
		if !strings.HasPrefix(hostname, "https://") {
			hostname = "https://" + hostname
		}
		baseClient = githubv4.NewEnterpriseClient(hostname+"/api/graphql", httpClient)
	} else {
		baseClient = githubv4.NewClient(httpClient)
	}

	return &RateLimitAwareGraphQLClient{
		client: baseClient,
	}, nil
}

// getGraphQLClient returns the appropriate GraphQL client and client name based on the client type
func (api *GitHubAPI) getGraphQLClient(clientType ClientType) (*RateLimitAwareGraphQLClient, string, error) {
	switch clientType {
	case SourceClient:
		return api.sourceGraphClient, "source", nil
	case TargetClient:
		return api.targetGraphClient, "target", nil
	default:
		return nil, "", fmt.Errorf("invalid client type")
	}
}

// getRESTClient returns the appropriate REST client and client name based on the client type
func (api *GitHubAPI) getRESTClient(clientType ClientType) (*github.Client, string, error) {
	switch clientType {
	case SourceClient:
		return api.sourceClient, "source", nil
	case TargetClient:
		return api.targetClient, "target", nil
	default:
		return nil, "", fmt.Errorf("invalid client type")
	}
}

// ValidateRepoAccess validates that the client can access the specified repository
// This performs a lightweight GraphQL query to verify authentication, SAML authorization,
// and repository access permissions before attempting more expensive operations
func (api *GitHubAPI) ValidateRepoAccess(clientType ClientType, owner, name string) error {
	ctx := context.Background()

	var query struct {
		Repository struct {
			ID string
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return fmt.Errorf("failed to get %s client: %w", clientName, err)
	}

	// Use the underlying client directly to skip rate limit check for this simple validation
	err = client.client.Query(ctx, &query, variables)
	if err != nil {
		return err
	}

	return nil
}

// RateLimitInfo contains information about current rate limit status
type RateLimitInfo struct {
	Remaining int
	ResetAt   time.Time
}

// GetRateLimitStatus returns the current rate limit status for the specified client
func (api *GitHubAPI) GetRateLimitStatus(clientType ClientType) (*RateLimitInfo, error) {
	ctx := context.Background()

	var query struct {
		RateLimit struct {
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s client: %w", clientName, err)
	}

	// Use underlying client directly
	err = client.client.Query(ctx, &query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s rate limit: %w", clientName, err)
	}

	return &RateLimitInfo{
		Remaining: query.RateLimit.Remaining,
		ResetAt:   query.RateLimit.ResetAt.Time,
	}, nil
}

func (c *RateLimitAwareGraphQLClient) Query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	var rateLimitQuery struct {
		RateLimit struct {
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}

	for {
		// Check the current rate limit
		if err := c.client.Query(ctx, &rateLimitQuery, nil); err != nil {
			return err
		}

		if rateLimitQuery.RateLimit.Remaining > 0 {
			// Proceed with the actual query
			return c.client.Query(ctx, q, variables)
		}

		// Rate limited - wait silently until reset
		sleepDuration := time.Until(rateLimitQuery.RateLimit.ResetAt.Time)
		time.Sleep(sleepDuration)
	}
}

// GetIssueCount retrieves the total count of issues for a repository using GraphQL
func (api *GitHubAPI) GetIssueCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner string
			Issues        struct {
				TotalCount int
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return 0, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return 0, fmt.Errorf("failed to query %s repository issue count: %v", clientName, err)
	}

	return query.Repository.Issues.TotalCount, nil
}

// PRCounts holds the counts for different pull request states
type PRCounts struct {
	Open   int
	Merged int
	Closed int
	Total  int
}

// GetPRCounts retrieves the counts of pull requests by state for a repository using GraphQL
func (api *GitHubAPI) GetPRCounts(clientType ClientType, owner, name string) (*PRCounts, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner string
			OpenPRs       struct {
				TotalCount int
			} `graphql:"openPRs: pullRequests(states: OPEN)"`
			MergedPRs struct {
				TotalCount int
			} `graphql:"mergedPRs: pullRequests(states: MERGED)"`
			ClosedPRs struct {
				TotalCount int
			} `graphql:"closedPRs: pullRequests(states: CLOSED)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return nil, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s repository PR counts: %v", clientName, err)
	}

	counts := &PRCounts{
		Open:   query.Repository.OpenPRs.TotalCount,
		Merged: query.Repository.MergedPRs.TotalCount,
		Closed: query.Repository.ClosedPRs.TotalCount,
	}

	// Calculate total count
	counts.Total = counts.Open + counts.Merged + counts.Closed

	return counts, nil
}

// GetTagCount retrieves the total count of tags for a repository using GraphQL
func (api *GitHubAPI) GetTagCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner string
			Refs          struct {
				TotalCount int
			} `graphql:"refs(refPrefix: \"refs/tags/\")"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return 0, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return 0, fmt.Errorf("failed to query %s repository tag count: %v", clientName, err)
	}

	return query.Repository.Refs.TotalCount, nil
}

// GetReleaseCount retrieves the total count of releases for a repository using GraphQL
func (api *GitHubAPI) GetReleaseCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner string
			Releases      struct {
				TotalCount int
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return 0, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return 0, fmt.Errorf("failed to query %s repository release count: %v", clientName, err)
	}

	return query.Repository.Releases.TotalCount, nil
}

// GetCommitCount retrieves the total count of commits on the default branch using GraphQL
func (api *GitHubAPI) GetCommitCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner    string
			DefaultBranchRef struct {
				Target struct {
					Commit struct {
						History struct {
							TotalCount int
						}
					} `graphql:"... on Commit"`
				}
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return 0, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return 0, fmt.Errorf("failed to query %s repository commit count: %v", clientName, err)
	}

	return query.Repository.DefaultBranchRef.Target.Commit.History.TotalCount, nil
}

// GetLatestCommitHash retrieves the latest commit hash from the default branch using GraphQL
func (api *GitHubAPI) GetLatestCommitHash(clientType ClientType, owner, name string) (string, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner    string
			DefaultBranchRef struct {
				Target struct {
					Commit struct {
						OID string
					} `graphql:"... on Commit"`
				}
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return "", err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return "", fmt.Errorf("failed to query %s repository latest commit hash: %v", clientName, err)
	}

	return query.Repository.DefaultBranchRef.Target.Commit.OID, nil
}

// GetBranchProtectionRulesCount retrieves the total count of branch protection rules for a repository using GraphQL
func (api *GitHubAPI) GetBranchProtectionRulesCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	var query struct {
		Repository struct {
			NameWithOwner         string
			BranchProtectionRules struct {
				TotalCount int
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return 0, err
	}

	err = client.Query(ctx, &query, variables)
	if err != nil {
		return 0, fmt.Errorf("failed to query %s repository branch protection rules count: %v", clientName, err)
	}

	return query.Repository.BranchProtectionRules.TotalCount, nil
}

// GetWebhookCount retrieves the count of all webhooks (active and inactive) for a repository using REST API
func (api *GitHubAPI) GetWebhookCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	client, clientName, err := api.getRESTClient(clientType)
	if err != nil {
		return 0, err
	}

	// List all webhooks for the repository
	opts := &github.ListOptions{PerPage: 100}
	var webhookCount int

	for {
		webhooks, resp, err := client.Repositories.ListHooks(ctx, owner, name, opts)
		if err != nil {
			return 0, fmt.Errorf("failed to query %s repository webhook count: %v", clientName, err)
		}

		// Count all webhooks (both active and inactive)
		webhookCount += len(webhooks)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return webhookCount, nil
}

// ListOrganizationMigrations retrieves the list of organization migrations using REST API
// Limited to the last 100 migrations
func (api *GitHubAPI) ListOrganizationMigrations(clientType ClientType, org string) ([]*github.Migration, error) {
	ctx := context.Background()

	client, clientName, err := api.getRESTClient(clientType)
	if err != nil {
		return nil, err
	}

	opts := &github.ListOptions{PerPage: 100}
	var allMigrations []*github.Migration
	migrationCount := 0
	maxMigrations := 100

	for {
		migrations, resp, err := client.Migrations.ListMigrations(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list %s organization migrations: %v", clientName, err)
		}

		for _, migration := range migrations {
			if migrationCount >= maxMigrations {
				break
			}
			allMigrations = append(allMigrations, migration)
			migrationCount++
		}

		if resp.NextPage == 0 || migrationCount >= maxMigrations {
			break
		}
		opts.Page = resp.NextPage
	}

	return allMigrations, nil
}

// MigrationInfo holds information about a migration for display to user
type MigrationInfo struct {
	ID           int64
	CreatedAt    string
	UpdatedAt    string
	State        string
	Repositories []string
}

// FindMigrationsByRepository finds migrations that contain the specified repository
func (api *GitHubAPI) FindMigrationsByRepository(clientType ClientType, org, repoName string) ([]*MigrationInfo, error) {
	migrations, err := api.ListOrganizationMigrations(clientType, org)
	if err != nil {
		return nil, err
	}

	var matchingMigrations []*MigrationInfo

	for _, migration := range migrations {
		// Only consider migrations that are in "exported" state
		if migration.GetState() != "exported" {
			continue // Skip this entire migration - not in exported state
		}

		// Pre-allocate repository names slice and check for target repo in single pass
		repositories := make([]string, 0, len(migration.Repositories))
		foundTarget := false

		for _, repo := range migration.Repositories {
			currentRepoName := repo.GetName()
			repositories = append(repositories, currentRepoName)

			if repoName == currentRepoName {
				foundTarget = true
			}
		}

		// Only create MigrationInfo if target repository was found
		if foundTarget {
			migrationInfo := &MigrationInfo{
				ID:           migration.GetID(),
				CreatedAt:    migration.GetCreatedAt(),
				UpdatedAt:    migration.GetUpdatedAt(),
				State:        migration.GetState(),
				Repositories: repositories, // Assign pre-built slice
			}

			matchingMigrations = append(matchingMigrations, migrationInfo)
		}
	}

	return matchingMigrations, nil
}

// DownloadMigrationArchive downloads a migration archive and returns the file path
func (api *GitHubAPI) DownloadMigrationArchive(clientType ClientType, org string, migrationID int64, outputPath string) (string, error) {
	ctx := context.Background()

	client, clientName, err := api.getRESTClient(clientType)
	if err != nil {
		return "", err
	}

	// Step 1: Get the signed S3 URL from GitHub API
	signedURL, err := client.Migrations.MigrationArchiveURL(ctx, org, migrationID)
	if err != nil {
		return "", fmt.Errorf("failed to get %s migration archive URL: %v", clientName, err)
	}

	// Step 2: Use a plain HTTP client to download from the signed S3 URL
	// Note: We don't need authentication for the signed URL - it's already authorized
	httpClient := &http.Client{}
	downloadResp, err := httpClient.Get(signedURL)
	if err != nil {
		return "", fmt.Errorf("failed to download from signed URL: %v", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download migration archive from S3: status %d", downloadResp.StatusCode)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Copy the response body to the file
	_, err = io.Copy(file, downloadResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save migration archive: %v", err)
	}

	return outputPath, nil
}

// Helper function to get client config for a given client type
func getClientConfigForType(clientType ClientType) ClientConfig {
	switch clientType {
	case SourceClient:
		return ClientConfig{
			Token:          viper.GetString("SOURCE_TOKEN"),
			Hostname:       viper.GetString("SOURCE_HOSTNAME"),
			AppID:          viper.GetString("SOURCE_APP_ID"),
			PrivateKey:     []byte(viper.GetString("SOURCE_PRIVATE_KEY")),
			InstallationID: viper.GetInt64("SOURCE_INSTALLATION_ID"),
		}
	case TargetClient:
		return ClientConfig{
			Token:          viper.GetString("TARGET_TOKEN"),
			Hostname:       viper.GetString("TARGET_HOSTNAME"),
			AppID:          viper.GetString("TARGET_APP_ID"),
			PrivateKey:     []byte(viper.GetString("TARGET_PRIVATE_KEY")),
			InstallationID: viper.GetInt64("TARGET_INSTALLATION_ID"),
		}
	default:
		return ClientConfig{}
	}
}

package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

// Package-level instance of GitHubAPI
var defaultAPI *GitHubAPI

// GetAPI returns the default GitHubAPI instance, initializing it if necessary
func GetAPI() *GitHubAPI {
	if defaultAPI == nil {
		defaultAPI = NewGitHubAPI()
	}
	return defaultAPI
}

// For testing purposes - allows resetting the default API
func resetAPI() {
	defaultAPI = nil
}

// newGitHubAPI creates a new GitHubAPI instance with configured clients
// Now private since we want to control initialization through GetAPI()
func NewGitHubAPI() *GitHubAPI {
	sourceConfig := ClientConfig{
		Token:          viper.GetString("SOURCE_TOKEN"),
		Hostname:       viper.GetString("SOURCE_HOSTNAME"),
		AppID:          viper.GetString("SOURCE_APP_ID"),
		PrivateKey:     []byte(viper.GetString("SOURCE_PRIVATE_KEY")),
		InstallationID: viper.GetInt64("SOURCE_INSTALLATION_ID"),
	}

	targetConfig := ClientConfig{
		Token:          viper.GetString("TARGET_TOKEN"),
		Hostname:       viper.GetString("TARGET_HOSTNAME"),
		AppID:          viper.GetString("TARGET_APP_ID"),
		PrivateKey:     []byte(viper.GetString("TARGET_PRIVATE_KEY")),
		InstallationID: viper.GetInt64("TARGET_INSTALLATION_ID"),
	}

	sourceClient := newGitHubClient(sourceConfig)
	targetClient := newGitHubClient(targetConfig)
	sourceGraphClient := newGitHubGraphQLClient(sourceConfig)
	targetGraphClient := newGitHubGraphQLClient(targetConfig)

	return &GitHubAPI{
		sourceClient:      sourceClient,
		targetClient:      targetClient,
		sourceGraphClient: sourceGraphClient,
		targetGraphClient: targetGraphClient,
	}
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
func newGitHubClient(config ClientConfig) *github.Client {
	httpClient, err := createAuthenticatedClient(config)
	if err != nil {
		log.Fatalf("Failed to create authenticated client: %v", err)
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
			log.Fatalf("Failed to configure enterprise URLs: %v", err)
		}
	}

	return client
}

type RateLimitAwareGraphQLClient struct {
	client *githubv4.Client
}

// newGitHubGraphQLClient creates a new GitHub GraphQL client based on the provided configuration
func newGitHubGraphQLClient(config ClientConfig) *RateLimitAwareGraphQLClient {
	httpClient, err := createAuthenticatedClient(config)
	if err != nil {
		log.Fatalf("Failed to create authenticated client: %v", err)
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
	}
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

		//log.Println("Rate limit remaining:", rateLimitQuery.RateLimit.Remaining)

		if rateLimitQuery.RateLimit.Remaining > 0 {
			// Proceed with the actual query
			err := c.client.Query(ctx, q, variables)
			if err != nil {
				return err
			}
			return nil
		} else {
			// Sleep until rate limit resets
			log.Println("Rate limit exceeded, sleeping until reset at:", rateLimitQuery.RateLimit.ResetAt.Time)
			time.Sleep(time.Until(rateLimitQuery.RateLimit.ResetAt.Time))
		}
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

// GetWebhookCount retrieves the count of active webhooks for a repository using REST API
func (api *GitHubAPI) GetWebhookCount(clientType ClientType, owner, name string) (int, error) {
	ctx := context.Background()

	client, clientName, err := api.getRESTClient(clientType)
	if err != nil {
		return 0, err
	}

	// List all webhooks for the repository
	opts := &github.ListOptions{PerPage: 100}
	var activeWebhookCount int

	for {
		webhooks, resp, err := client.Repositories.ListHooks(ctx, owner, name, opts)
		if err != nil {
			return 0, fmt.Errorf("failed to query %s repository webhook count: %v", clientName, err)
		}

		// Count only active webhooks
		for _, webhook := range webhooks {
			if webhook.Active != nil && *webhook.Active {
				activeWebhookCount++
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return activeWebhookCount, nil
}

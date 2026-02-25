// Package bitbucket provides a REST API client for Bitbucket Server (Data Center).
// It retrieves repository metrics used for migration validation comparisons against
// GitHub target repositories.
package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/validator"

	"github.com/pterm/pterm"
)

// BBSClient is an HTTP client for interacting with the Bitbucket Server REST API.
type BBSClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// bbsPagedResponse represents the standard paginated response envelope from BBS REST APIs.
type bbsPagedResponse struct {
	Size       int             `json:"size"`
	Limit      int             `json:"limit"`
	IsLastPage bool            `json:"isLastPage"`
	Start      int             `json:"start"`
	Values     json.RawMessage `json:"values"`
}

// bbsCommitCountResponse extends the paged response with an optional totalCount field
// returned by the commits endpoint.
type bbsCommitCountResponse struct {
	bbsPagedResponse
	TotalCount *int `json:"totalCount,omitempty"`
}

// bbsCommit represents a single commit object from the BBS commits endpoint.
type bbsCommit struct {
	ID string `json:"id"`
}

// bbsDefaultBranch represents the default branch response from BBS.
type bbsDefaultBranch struct {
	ID           string `json:"id"`
	DisplayID    string `json:"displayId"`
	LatestCommit string `json:"latestCommit"`
}

// NewBBSClient creates a new Bitbucket Server API client.
// The baseURL should be the BBS hostname (e.g., "bitbucket.example.com").
// The token is a personal access token used for Bearer authentication.
func NewBBSClient(baseURL, token string) (*BBSClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Bitbucket Server base URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("Bitbucket Server token is required")
	}

	// Normalize URL: trim trailing slash, ensure https://
	normalizedURL := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if !strings.HasPrefix(normalizedURL, "https://") && !strings.HasPrefix(normalizedURL, "http://") {
		normalizedURL = "https://" + normalizedURL
	}

	return &BBSClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    normalizedURL,
		token:      token,
	}, nil
}

// ValidateRepoAccess verifies that the client can authenticate and access the specified repository.
func (c *BBSClient) ValidateRepoAccess(project, repo string) error {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s", project, repo)
	_, err := c.doGet(path)
	if err != nil {
		return fmt.Errorf("cannot access repository %s/%s: %w", project, repo, err)
	}
	return nil
}

// GetRepositoryMetrics retrieves all available metrics from a Bitbucket Server repository.
// It returns populated RepositoryData, a slice of error messages for partial failures,
// and a fatal error only if ALL requests fail.
func (c *BBSClient) GetRepositoryMetrics(project, repo string, spinner *pterm.SpinnerPrinter) (*validator.RepositoryData, []string, error) {
	startTime := time.Now()
	var failedRequests []string
	var errorMessages []string
	var successfulRequests int

	data := &validator.RepositoryData{
		Owner:    project,
		Name:     repo,
		Issues:   0, // BBS does not have issues
		Releases: 0, // BBS does not have releases
	}

	// Get PR counts
	spinner.UpdateText(fmt.Sprintf("Fetching pull requests from %s/%s...", project, repo))
	prCounts, err := c.getPRCounts(project, repo)
	if err != nil {
		failedRequests = append(failedRequests, "pull requests")
		errorMessages = append(errorMessages, fmt.Sprintf("pull requests: %v", err))
		data.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
	} else {
		data.PRs = prCounts
		successfulRequests++
	}

	// Get tag count
	spinner.UpdateText(fmt.Sprintf("Fetching tags from %s/%s...", project, repo))
	tags, err := c.getTagCount(project, repo)
	if err != nil {
		failedRequests = append(failedRequests, "tags")
		errorMessages = append(errorMessages, fmt.Sprintf("tags: %v", err))
		data.Tags = 0
	} else {
		data.Tags = tags
		successfulRequests++
	}

	// Get default branch (needed for commit count and latest commit)
	spinner.UpdateText(fmt.Sprintf("Fetching default branch from %s/%s...", project, repo))
	defaultBranch, err := c.getDefaultBranch(project, repo)
	if err != nil {
		failedRequests = append(failedRequests, "default branch")
		errorMessages = append(errorMessages, fmt.Sprintf("default branch: %v", err))
	} else {
		successfulRequests++
	}

	// Get commit count (requires default branch)
	if defaultBranch != "" {
		spinner.UpdateText(fmt.Sprintf("Fetching commit count from %s/%s...", project, repo))
		commitCount, err := c.getCommitCount(project, repo, defaultBranch)
		if err != nil {
			failedRequests = append(failedRequests, "commits")
			errorMessages = append(errorMessages, fmt.Sprintf("commits: %v", err))
			data.CommitCount = 0
		} else {
			data.CommitCount = commitCount
			successfulRequests++
		}

		// Get latest commit hash (requires default branch)
		spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", project, repo))
		latestHash, err := c.getLatestCommitHash(project, repo, defaultBranch)
		if err != nil {
			failedRequests = append(failedRequests, "latest commit hash")
			errorMessages = append(errorMessages, fmt.Sprintf("latest commit hash: %v", err))
			data.LatestCommitSHA = ""
		} else {
			data.LatestCommitSHA = latestHash
			successfulRequests++
		}
	} else {
		data.CommitCount = 0
		data.LatestCommitSHA = ""
	}

	// Get branch permissions count
	spinner.UpdateText(fmt.Sprintf("Fetching branch permissions from %s/%s...", project, repo))
	branchPermissions, err := c.getBranchPermissionsCount(project, repo)
	if err != nil {
		failedRequests = append(failedRequests, "branch permissions")
		errorMessages = append(errorMessages, fmt.Sprintf("branch permissions: %v", err))
		data.BranchProtectionRules = 0
	} else {
		data.BranchProtectionRules = branchPermissions
		successfulRequests++
	}

	// Get webhook count
	spinner.UpdateText(fmt.Sprintf("Fetching webhooks from %s/%s...", project, repo))
	webhooks, err := c.getWebhookCount(project, repo)
	if err != nil {
		failedRequests = append(failedRequests, "webhooks")
		errorMessages = append(errorMessages, fmt.Sprintf("webhooks: %v", err))
		data.Webhooks = 0
	} else {
		data.Webhooks = webhooks
		successfulRequests++
	}

	// LFS objects are not available via the BBS REST API
	data.LFSObjects = 0

	duration := time.Since(startTime)

	// Determine success/failure status
	if successfulRequests == 0 {
		spinner.Fail(fmt.Sprintf("Failed to retrieve any data from %s/%s", project, repo))
		return data, errorMessages, fmt.Errorf("all API requests failed for %s/%s", project, repo)
	}

	if len(failedRequests) > 0 {
		spinner.Warning(fmt.Sprintf("%s/%s: %d OK, %d failed (%v) - missing: %v",
			project, repo, successfulRequests, len(failedRequests), duration, failedRequests))
	} else {
		spinner.Success(fmt.Sprintf("%s/%s retrieved successfully (%v)", project, repo, duration))
	}

	return data, errorMessages, nil
}

// getPRCounts retrieves pull request counts by state (open, merged, declined/closed).
func (c *BBSClient) getPRCounts(project, repo string) (*api.PRCounts, error) {
	counts := &api.PRCounts{}

	// Fetch open PRs
	openCount, err := c.getPRCountByState(project, repo, "OPEN")
	if err != nil {
		return nil, fmt.Errorf("failed to get open PR count: %w", err)
	}
	counts.Open = openCount

	// Fetch merged PRs
	mergedCount, err := c.getPRCountByState(project, repo, "MERGED")
	if err != nil {
		return nil, fmt.Errorf("failed to get merged PR count: %w", err)
	}
	counts.Merged = mergedCount

	// Fetch declined PRs (maps to "Closed" in GitHub terminology)
	declinedCount, err := c.getPRCountByState(project, repo, "DECLINED")
	if err != nil {
		return nil, fmt.Errorf("failed to get declined PR count: %w", err)
	}
	counts.Closed = declinedCount

	counts.Total = counts.Open + counts.Merged + counts.Closed
	return counts, nil
}

func (c *BBSClient) getPRCountByState(project, repo, state string) (int, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=%s", project, repo, state)
	return c.getPaginatedCount(path, fmt.Sprintf("%s PR count", state))
}

// getTagCount retrieves the total number of tags in the repository.
func (c *BBSClient) getTagCount(project, repo string) (int, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/tags", project, repo)
	return c.getPaginatedCount(path, "tag count")
}

// getDefaultBranch retrieves the default branch name for the repository.
func (c *BBSClient) getDefaultBranch(project, repo string) (string, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/default-branch", project, repo)
	body, err := c.doGet(path)
	if err != nil {
		return "", fmt.Errorf("failed to get default branch: %w", err)
	}

	var branch bbsDefaultBranch
	if err := json.Unmarshal(body, &branch); err != nil {
		return "", fmt.Errorf("failed to parse default branch response: %w", err)
	}

	if branch.DisplayID != "" {
		return branch.DisplayID, nil
	}
	return branch.ID, nil
}

// getCommitCount retrieves the total number of commits on the specified branch.
func (c *BBSClient) getCommitCount(project, repo, branch string) (int, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits?limit=0&until=%s", project, repo, url.QueryEscape(branch))
	body, err := c.doGet(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get commit count: %w", err)
	}

	var resp bbsCommitCountResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse commit count response: %w", err)
	}

	// Prefer totalCount if available (more reliable for large repos)
	if resp.TotalCount != nil {
		return *resp.TotalCount, nil
	}

	// Fall back to paginating through all commits
	return c.paginateCommitCount(project, repo, branch)
}

func (c *BBSClient) paginateCommitCount(project, repo, branch string) (int, error) {
	total := 0
	start := 0

	for {
		path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits?limit=1000&until=%s&start=%d",
			project, repo, url.QueryEscape(branch), start)
		body, err := c.doGet(path)
		if err != nil {
			return 0, fmt.Errorf("failed to paginate commits: %w", err)
		}

		var paged bbsPagedResponse
		if err := json.Unmarshal(body, &paged); err != nil {
			return 0, fmt.Errorf("failed to parse commit page response: %w", err)
		}

		total += paged.Size
		if paged.IsLastPage {
			break
		}
		start = paged.Start + paged.Limit
	}

	return total, nil
}

// getLatestCommitHash retrieves the hash of the latest commit on the specified branch.
func (c *BBSClient) getLatestCommitHash(project, repo, branch string) (string, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits?limit=1&until=%s", project, repo, url.QueryEscape(branch))
	body, err := c.doGet(path)
	if err != nil {
		return "", fmt.Errorf("failed to get latest commit: %w", err)
	}

	var paged bbsPagedResponse
	if err := json.Unmarshal(body, &paged); err != nil {
		return "", fmt.Errorf("failed to parse latest commit response: %w", err)
	}

	var commits []bbsCommit
	if err := json.Unmarshal(paged.Values, &commits); err != nil {
		return "", fmt.Errorf("failed to parse commit values: %w", err)
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found on branch %s", branch)
	}

	return commits[0].ID, nil
}

// getBranchPermissionsCount retrieves the number of branch permission restrictions.
// This uses the branch-permissions API (separate from the core REST API).
func (c *BBSClient) getBranchPermissionsCount(project, repo string) (int, error) {
	path := fmt.Sprintf("/rest/branch-permissions/2.0/projects/%s/repos/%s/restrictions", project, repo)
	return c.getPaginatedCount(path, "branch permissions count")
}

// getWebhookCount retrieves the number of webhooks configured for the repository.
func (c *BBSClient) getWebhookCount(project, repo string) (int, error) {
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/webhooks", project, repo)
	return c.getPaginatedCount(path, "webhook count")
}

// getPaginatedCount iterates through all pages of a BBS paginated endpoint and returns the total item count.
func (c *BBSClient) getPaginatedCount(basePath, label string) (int, error) {
	total := 0
	start := 0
	limit := 100

	for {
		separator := "?"
		if strings.Contains(basePath, "?") {
			separator = "&"
		}
		path := fmt.Sprintf("%s%slimit=%d&start=%d", basePath, separator, limit, start)
		body, err := c.doGet(path)
		if err != nil {
			return 0, fmt.Errorf("failed to get %s: %w", label, err)
		}

		var paged bbsPagedResponse
		if err := json.Unmarshal(body, &paged); err != nil {
			return 0, fmt.Errorf("failed to parse %s response: %w", label, err)
		}

		total += paged.Size
		if paged.IsLastPage {
			break
		}
		start = paged.Start + paged.Limit
	}

	return total, nil
}

// doGet performs an authenticated GET request to the BBS API with retry logic.
func (c *BBSClient) doGet(path string) ([]byte, error) {
	url := c.baseURL + path

	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for %s: %w", path, err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed for %s: %w", path, err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response body for %s: %w", path, readErr)
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil

		case http.StatusTooManyRequests:
			if attempt < maxRetries {
				backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				time.Sleep(backoff)
				lastErr = fmt.Errorf("rate limited (429) on %s", path)
				continue
			}
			return nil, fmt.Errorf("rate limited (429) on %s after %d retries", path, maxRetries)

		case http.StatusUnauthorized:
			return nil, fmt.Errorf("authentication failed (401) for %s: check your BBS token", path)

		case http.StatusForbidden:
			return nil, fmt.Errorf("access denied (403) for %s: insufficient permissions", path)

		case http.StatusNotFound:
			return nil, fmt.Errorf("not found (404) for %s: verify the project/repo exists", path)

		default:
			return nil, fmt.Errorf("unexpected status %d for %s: %s", resp.StatusCode, path, string(body))
		}
	}

	return nil, lastErr
}

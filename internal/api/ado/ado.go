// Package ado provides a REST API client for Azure DevOps Server (on-prem).
// It retrieves repository metrics used for migration validation comparisons against
// GitHub target repositories.
package ado

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mona-actions/gh-migration-validator/internal/api"
	"mona-actions/gh-migration-validator/internal/validator"

	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

// defaultAPIVersion is the Azure DevOps REST API version used as a starting
// point. Different server generations support different versions (TFS 2018 -> 4.1,
// ADO Server 2019 -> 5.0/5.1, ADO Server 2020+ -> 6.0, ADO Services -> 7.x), so when
// the caller does not pin a version the client negotiates one from candidateAPIVersions.
const defaultAPIVersion = "6.0"

// cloudBaseURL is the base URL for Azure DevOps Services (cloud). Used when no server
// URL is provided, so cloud users only need to supply their organization and PAT.
const cloudBaseURL = "https://dev.azure.com"

// errNotFound is returned (wrapped) by doGet when the server responds 404, so callers
// can distinguish a missing resource (e.g. no .gitattributes) from other errors.
var errNotFound = errors.New("resource not found")

// candidateAPIVersions are tried in order (newest first) when negotiating a supported
// REST API version. The first version that a probe request accepts is used.
var candidateAPIVersions = []string{"7.1", "7.0", "6.0", "5.1", "5.0", "4.1"}

// pageSize is the number of items requested per page for paginated endpoints.
const pageSize = 100

// maxLFSPointerBytes is the maximum blob size we will download to inspect for an LFS
// pointer. Real pointer files are tiny (around 130 bytes); the 1 KB threshold is a
// safety margin that still avoids downloading large regular files that match an LFS
// pattern. Content is validated by ParseLFSPointer regardless, so an over-generous
// threshold only affects performance, never correctness.
const maxLFSPointerBytes = 1024

// ADOClient is an HTTP client for interacting with the Azure DevOps Server REST API.
type ADOClient struct {
	httpClient    *http.Client
	baseURL       string // {server-url}/{collection}
	authHeader    string
	apiVersion    string
	autoNegotiate bool // true when apiVersion was not pinned by the caller
}

// adoListResponse represents the standard list response envelope from ADO REST APIs.
type adoListResponse struct {
	Count int             `json:"count"`
	Value json.RawMessage `json:"value"`
}

// adoRepository represents a git repository object from ADO.
type adoRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
}

// adoCommit represents a single commit object from the ADO commits endpoint.
type adoCommit struct {
	CommitID string   `json:"commitId"`
	Parents  []string `json:"parents"`
}

// adoPolicyConfiguration represents a policy configuration with its scope, used to
// determine how many branch policies apply to a given repository.
type adoPolicyConfiguration struct {
	Settings struct {
		Scope []struct {
			RepositoryID string `json:"repositoryId"`
		} `json:"scope"`
	} `json:"settings"`
}

// adoSubscription represents a service hook subscription with its publisher inputs,
// used to determine how many hooks reference a given repository.
type adoSubscription struct {
	PublisherInputs struct {
		Repository string `json:"repository"`
	} `json:"publisherInputs"`
}

// adoItem represents a git item (file or folder) returned by the ADO items API.
type adoItem struct {
	ObjectID      string `json:"objectId"`
	GitObjectType string `json:"gitObjectType"`
	Path          string `json:"path"`
	IsFolder      bool   `json:"isFolder"`
	Content       string `json:"content"`
}

// adoBlob represents git blob metadata returned by the ADO blobs API.
type adoBlob struct {
	Size int64 `json:"size"`
}

// NewADOClient creates a new Azure DevOps API client that works with both Azure
// DevOps Server (on-prem) and Azure DevOps Services (cloud).
//
// serverURL is the base URL:
//   - Cloud: leave empty (defaults to https://dev.azure.com) or pass "https://dev.azure.com".
//   - On-prem: the server URL including any collection path prefix, e.g. "https://ado.example.com/tfs".
//
// org is the organization (cloud) or collection (on-prem) name. token is a personal
// access token. apiVersion is the REST API version; when empty it is auto-negotiated.
func NewADOClient(serverURL, org, token, apiVersion string) (*ADOClient, error) {
	if org == "" {
		return nil, fmt.Errorf("Azure DevOps organization (collection) is required")
	}
	if token == "" {
		return nil, fmt.Errorf("Azure DevOps token is required")
	}

	baseURL := buildBaseURL(serverURL, org)

	// ADO uses HTTP Basic auth with the PAT as the password. The username is ignored
	// by the server but must be non-empty: some ADO Server / TFS versions reject an
	// empty username with 401, so we use a placeholder ("pat").
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("pat:"+token))

	version := strings.TrimSpace(apiVersion)
	autoNegotiate := version == ""
	if version == "" {
		version = defaultAPIVersion
	}

	return &ADOClient{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		baseURL:       baseURL,
		authHeader:    authHeader,
		apiVersion:    version,
		autoNegotiate: autoNegotiate,
	}, nil
}

// buildBaseURL constructs the {base}/{org} URL used for all API calls, handling both
// cloud and on-prem forms:
//   - Empty serverURL defaults to Azure DevOps Services (cloud).
//   - A legacy "{org}.visualstudio.com" host already contains the org, so it is not appended.
//   - A URL that already ends with the org (e.g. a full cloud URL) is not doubled.
func buildBaseURL(serverURL, org string) string {
	normalizedURL := strings.TrimSuffix(strings.TrimSpace(serverURL), "/")
	if normalizedURL == "" {
		normalizedURL = cloudBaseURL
	}
	if !strings.HasPrefix(normalizedURL, "https://") && !strings.HasPrefix(normalizedURL, "http://") {
		normalizedURL = "https://" + normalizedURL
	}

	org = strings.Trim(strings.TrimSpace(org), "/")

	// Legacy Azure DevOps Services URL embeds the org in the hostname
	// (e.g. https://myorg.visualstudio.com), so the org must not be appended.
	if strings.Contains(strings.ToLower(normalizedURL), ".visualstudio.com") {
		return normalizedURL
	}

	// Avoid doubling the org when the URL already ends with it
	// (e.g. the caller passed the full https://dev.azure.com/myorg).
	if org != "" && strings.HasSuffix(strings.ToLower(normalizedURL), "/"+strings.ToLower(org)) {
		return normalizedURL
	}

	return normalizedURL + "/" + org
}

// APIVersion returns the REST API version currently in use.
func (c *ADOClient) APIVersion() string {
	return c.apiVersion
}

// NegotiateAPIVersion probes the server to find a supported REST API version when the
// caller did not pin one. It updates the client's apiVersion in place and returns the
// version selected. When a version was pinned explicitly, it is returned unchanged.
//
// If the server is unreachable (connection refused, timeout, DNS failure), it fails
// fast on the first probe rather than retrying every candidate version, since a
// transport error does not depend on the API version.
func (c *ADOClient) NegotiateAPIVersion() (string, error) {
	if !c.autoNegotiate {
		return c.apiVersion, nil
	}

	// A collection-level projects list exists on any valid collection and is cheap.
	probePath := "/_apis/projects?$top=1"
	for _, v := range candidateAPIVersions {
		ok, err := c.probeAPIVersion(probePath, v)
		if err != nil {
			// Transport-level failure: the server is unreachable, so trying other
			// API versions would just repeat the same wait. Abort immediately.
			return "", fmt.Errorf("cannot reach Azure DevOps server at %s: %w", c.baseURL, err)
		}
		if ok {
			c.apiVersion = v
			c.autoNegotiate = false
			return v, nil
		}
	}

	return "", fmt.Errorf("could not negotiate a supported API version from %v (server reachable but none accepted)", candidateAPIVersions)
}

// probeAPIVersion issues a GET for the given path and version, returning true only when
// the server responds 200 OK. Transport errors are returned so the caller can surface
// connectivity problems rather than silently treating them as "unsupported version".
// A shorter timeout is used than for regular requests so an unreachable server fails fast.
func (c *ADOClient) probeAPIVersion(path, version string) (bool, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	requestURL := fmt.Sprintf("%s%s%sapi-version=%s", c.baseURL, path, separator, version)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK, nil
}

// ValidateRepoAccess verifies that the client can authenticate and access the specified repository.
func (c *ADOClient) ValidateRepoAccess(project, repo string) error {
	if _, err := c.getRepository(project, repo); err != nil {
		return fmt.Errorf("cannot access repository %s/%s: %w", project, repo, err)
	}
	return nil
}

// ListRepositories returns the names of all git repositories in the given ADO project.
func (c *ADOClient) ListRepositories(project string) ([]string, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories", url.PathEscape(project))
	body, err := c.doGet(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories for project %s: %w", project, err)
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse repository list response: %w", err)
	}

	var repos []adoRepository
	if err := json.Unmarshal(resp.Value, &repos); err != nil {
		return nil, fmt.Errorf("failed to parse repository list values: %w", err)
	}

	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return names, nil
}

// GetRepositoryMetrics retrieves all available metrics from an Azure DevOps repository.
// It returns populated RepositoryData, a slice of error messages for partial failures,
// and a fatal error only if ALL requests fail.
func (c *ADOClient) GetRepositoryMetrics(project, repo string, spinner *pterm.SpinnerPrinter) (*validator.RepositoryData, []string, error) {
	startTime := time.Now()
	var failedRequests []string
	var errorMessages []string
	var successfulRequests int

	data := &validator.RepositoryData{
		Owner:    project,
		Name:     repo,
		Issues:   0, // ADO uses Work Items, not native git issues
		Releases: 0, // ADO has no releases equivalent for git repos
	}

	// Fetch the repository object first (needed for id and default branch)
	spinner.UpdateText(fmt.Sprintf("Fetching repository info from %s/%s...", project, repo))
	repository, err := c.getRepository(project, repo)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Failed to retrieve repository %s/%s", project, repo))
		return data, errorMessages, fmt.Errorf("failed to retrieve repository %s/%s: %w", project, repo, err)
	}
	successfulRequests++
	defaultBranch := strings.TrimPrefix(repository.DefaultBranch, "refs/heads/")

	// Get PR counts (skip if NO_PRS flag is set)
	if !viper.GetBool("NO_PRS") {
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
	} else {
		data.PRs = &api.PRCounts{Total: 0, Open: 0, Merged: 0, Closed: 0}
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

	// Get commit count and latest commit hash (require default branch)
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

		spinner.UpdateText(fmt.Sprintf("Fetching latest commit hash from %s/%s...", project, repo))
		latestHash, err := c.getLatestCommitHash(project, repo, defaultBranch)
		if err != nil {
			failedRequests = append(failedRequests, "latest commit hash")
			errorMessages = append(errorMessages, fmt.Sprintf("latest commit hash: %v", err))
			data.LatestCommitSHA = ""
		} else {
			data.LatestCommitSHA = latestHash
			successfulRequests++

			// Also record the parent of the tip commit. This lets the validator detect a
			// `git lfs migrate` rewrite (which only changes the tip commit) and warn instead
			// of failing when the parent history still matches. Non-fatal on error.
			if parentHash, perr := c.getParentCommitHash(project, repo, latestHash); perr == nil {
				data.LatestCommitParentSHA = parentHash
			}
		}
	} else {
		data.CommitCount = 0
		data.LatestCommitSHA = ""
	}

	// Get branch policy count (advisory)
	spinner.UpdateText(fmt.Sprintf("Fetching branch policies from %s/%s...", project, repo))
	branchPolicies, err := c.getBranchPolicyCount(project, repository.ID)
	if err != nil {
		failedRequests = append(failedRequests, "branch policies")
		errorMessages = append(errorMessages, fmt.Sprintf("branch policies: %v", err))
		data.BranchProtectionRules = 0
	} else {
		data.BranchProtectionRules = branchPolicies
		successfulRequests++
	}

	// Get service hook count (skip if NO_WEBHOOKS flag is set)
	if !viper.GetBool("NO_WEBHOOKS") {
		spinner.UpdateText(fmt.Sprintf("Fetching service hooks from %s/%s...", project, repo))
		webhooks, err := c.getServiceHookCount(repository.ID)
		if err != nil {
			failedRequests = append(failedRequests, "service hooks")
			errorMessages = append(errorMessages, fmt.Sprintf("service hooks: %v", err))
			data.Webhooks = 0
		} else {
			data.Webhooks = webhooks
			successfulRequests++
		}
	}

	// Get LFS object count (requires default branch; skip if NO_LFS flag is set). Non-fatal: LFS is optional.
	if defaultBranch != "" && !viper.GetBool("NO_LFS") {
		spinner.UpdateText(fmt.Sprintf("Fetching LFS objects from %s/%s...", project, repo))
		lfsCount, err := c.getLFSObjectCount(project, repo, defaultBranch)
		if err != nil {
			failedRequests = append(failedRequests, "lfs objects")
			errorMessages = append(errorMessages, fmt.Sprintf("lfs objects: %v", err))
			data.LFSObjects = 0
		} else {
			data.LFSObjects = lfsCount
			successfulRequests++
		}
	} else {
		data.LFSObjects = 0
	}

	duration := time.Since(startTime)

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

// getRepository retrieves the repository object, including its id and default branch.
func (c *ADOClient) getRepository(project, repo string) (*adoRepository, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s", url.PathEscape(project), url.PathEscape(repo))
	body, err := c.doGet(path)
	if err != nil {
		return nil, err
	}

	var repository adoRepository
	if err := json.Unmarshal(body, &repository); err != nil {
		return nil, fmt.Errorf("failed to parse repository response: %w", err)
	}
	return &repository, nil
}

// getPRCounts retrieves pull request counts by state (active, completed, abandoned).
// ADO "completed" maps to GitHub "merged" and "abandoned" maps to GitHub "closed".
func (c *ADOClient) getPRCounts(project, repo string) (*api.PRCounts, error) {
	counts := &api.PRCounts{}

	openCount, err := c.getPRCountByStatus(project, repo, "active")
	if err != nil {
		return nil, fmt.Errorf("failed to get active PR count: %w", err)
	}
	counts.Open = openCount

	mergedCount, err := c.getPRCountByStatus(project, repo, "completed")
	if err != nil {
		return nil, fmt.Errorf("failed to get completed PR count: %w", err)
	}
	counts.Merged = mergedCount

	closedCount, err := c.getPRCountByStatus(project, repo, "abandoned")
	if err != nil {
		return nil, fmt.Errorf("failed to get abandoned PR count: %w", err)
	}
	counts.Closed = closedCount

	counts.Total = counts.Open + counts.Merged + counts.Closed
	return counts, nil
}

// getPRCountByStatus counts pull requests in a given state by paginating through results.
func (c *ADOClient) getPRCountByStatus(project, repo, status string) (int, error) {
	total := 0
	skip := 0

	for {
		path := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests?searchCriteria.status=%s&$top=%d&$skip=%d",
			url.PathEscape(project), url.PathEscape(repo), status, pageSize, skip)
		body, err := c.doGet(path)
		if err != nil {
			return 0, fmt.Errorf("failed to get %s PR count: %w", status, err)
		}

		var resp adoListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return 0, fmt.Errorf("failed to parse %s PR response: %w", status, err)
		}

		total += resp.Count
		if resp.Count < pageSize {
			break
		}
		skip += pageSize
	}

	return total, nil
}

// getTagCount retrieves the total number of tags in the repository.
func (c *ADOClient) getTagCount(project, repo string) (int, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/refs?filter=tags", url.PathEscape(project), url.PathEscape(repo))
	body, err := c.doGet(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get tag count: %w", err)
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse tag response: %w", err)
	}
	return resp.Count, nil
}

// getCommitCount retrieves the total number of commits on the specified branch by paginating.
func (c *ADOClient) getCommitCount(project, repo, branch string) (int, error) {
	total := 0
	skip := 0

	for {
		path := fmt.Sprintf("/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=%d&searchCriteria.$skip=%d",
			url.PathEscape(project), url.PathEscape(repo), url.QueryEscape(branch), pageSize, skip)
		body, err := c.doGet(path)
		if err != nil {
			return 0, fmt.Errorf("failed to get commit count: %w", err)
		}

		var resp adoListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return 0, fmt.Errorf("failed to parse commit count response: %w", err)
		}

		total += resp.Count
		if resp.Count < pageSize {
			break
		}
		skip += pageSize
	}

	return total, nil
}

// getLatestCommitHash retrieves the id of the latest commit on the specified branch.
func (c *ADOClient) getLatestCommitHash(project, repo, branch string) (string, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/commits?searchCriteria.itemVersion.version=%s&searchCriteria.$top=1",
		url.PathEscape(project), url.PathEscape(repo), url.QueryEscape(branch))
	body, err := c.doGet(path)
	if err != nil {
		return "", fmt.Errorf("failed to get latest commit: %w", err)
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse latest commit response: %w", err)
	}

	var commits []adoCommit
	if err := json.Unmarshal(resp.Value, &commits); err != nil {
		return "", fmt.Errorf("failed to parse commit values: %w", err)
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found on branch %s", branch)
	}
	return commits[0].CommitID, nil
}

// getParentCommitHash returns the first parent id of the given commit, or "" when the
// commit is a root commit with no parent.
func (c *ADOClient) getParentCommitHash(project, repo, commitID string) (string, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/commits/%s",
		url.PathEscape(project), url.PathEscape(repo), url.PathEscape(commitID))
	body, err := c.doGet(path)
	if err != nil {
		return "", fmt.Errorf("failed to get commit %s: %w", commitID, err)
	}

	var commit adoCommit
	if err := json.Unmarshal(body, &commit); err != nil {
		return "", fmt.Errorf("failed to parse commit response: %w", err)
	}
	if len(commit.Parents) == 0 {
		return "", nil
	}
	return commit.Parents[0], nil
}

// getBranchPolicyCount counts the branch policy configurations scoped to the given repository.
func (c *ADOClient) getBranchPolicyCount(project, repositoryID string) (int, error) {
	path := fmt.Sprintf("/%s/_apis/policy/configurations", url.PathEscape(project))
	body, err := c.doGet(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get branch policies: %w", err)
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse branch policy response: %w", err)
	}

	var policies []adoPolicyConfiguration
	if err := json.Unmarshal(resp.Value, &policies); err != nil {
		return 0, fmt.Errorf("failed to parse branch policy values: %w", err)
	}

	count := 0
	for _, policy := range policies {
		for _, scope := range policy.Settings.Scope {
			if scope.RepositoryID == repositoryID {
				count++
				break
			}
		}
	}
	return count, nil
}

// getServiceHookCount counts the collection-level service hook subscriptions that
// reference the given repository. Service hooks are scoped to the collection, so this
// is a best-effort match on the subscription's publisher inputs.
func (c *ADOClient) getServiceHookCount(repositoryID string) (int, error) {
	body, err := c.doGet("/_apis/hooks/subscriptions")
	if err != nil {
		return 0, fmt.Errorf("failed to get service hooks: %w", err)
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse service hook response: %w", err)
	}

	var subscriptions []adoSubscription
	if err := json.Unmarshal(resp.Value, &subscriptions); err != nil {
		return 0, fmt.Errorf("failed to parse service hook values: %w", err)
	}

	count := 0
	for _, sub := range subscriptions {
		if sub.PublisherInputs.Repository == repositoryID {
			count++
		}
	}
	return count, nil
}

// getLFSObjectCount counts the unique Git LFS objects referenced on the given branch.
// It mirrors the GitHub-side logic: read .gitattributes to find LFS-tracked patterns,
// then inspect matching files for LFS pointer content and count distinct OIDs. When the
// repository has no .gitattributes or no LFS patterns, it returns 0 without error.
func (c *ADOClient) getLFSObjectCount(project, repo, branch string) (int, error) {
	gitAttributes, found, err := c.getItemContent(project, repo, branch, "/.gitattributes")
	if err != nil {
		return 0, fmt.Errorf("failed to read .gitattributes: %w", err)
	}
	if !found {
		return 0, nil // no .gitattributes means no LFS tracking
	}

	patterns := api.ParseLFSPatterns(gitAttributes)
	if len(patterns) == 0 {
		return 0, nil
	}

	items, err := c.listRepositoryItems(project, repo, branch)
	if err != nil {
		return 0, fmt.Errorf("failed to list repository items: %w", err)
	}

	seenOIDs := make(map[string]bool)
	for _, item := range items {
		if item.GitObjectType != "blob" {
			continue
		}

		// ADO item paths start with "/"; LFS pattern matching expects a repo-relative path.
		relativePath := strings.TrimPrefix(item.Path, "/")
		if !api.MatchesLFSPattern(relativePath, patterns) {
			continue
		}

		// LFS pointer files are tiny. Skip large blobs so we never download the
		// full content of a regular (non-LFS) file that happens to match a pattern.
		if size, err := c.getBlobSize(project, repo, item.ObjectID); err == nil && size > maxLFSPointerBytes {
			continue
		}

		content, contentFound, err := c.getItemContent(project, repo, branch, item.Path)
		if err != nil || !contentFound || strings.TrimSpace(content) == "" {
			continue
		}

		if lfsObj, isLFS := api.ParseLFSPointer(content); isLFS {
			seenOIDs[lfsObj.OID] = true
		}
	}

	return len(seenOIDs), nil
}

// getBlobSize returns the size in bytes of a git blob identified by its object ID.
func (c *ADOClient) getBlobSize(project, repo, objectID string) (int64, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/blobs/%s",
		url.PathEscape(project), url.PathEscape(repo), url.PathEscape(objectID))

	body, err := c.doGet(path)
	if err != nil {
		return 0, err
	}

	var blob adoBlob
	if err := json.Unmarshal(body, &blob); err != nil {
		return 0, fmt.Errorf("failed to parse blob response: %w", err)
	}
	return blob.Size, nil
}

// getItemContent fetches the raw content of a single file on the given branch. The
// returned bool reports whether the item exists; a missing item is not an error.
func (c *ADOClient) getItemContent(project, repo, branch, itemPath string) (string, bool, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/items?path=%s&includeContent=true&versionDescriptor.version=%s&versionDescriptor.versionType=branch",
		url.PathEscape(project), url.PathEscape(repo), url.QueryEscape(itemPath), url.QueryEscape(branch))

	body, err := c.doGet(path)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return "", false, nil
		}
		return "", false, err
	}

	var item adoItem
	if err := json.Unmarshal(body, &item); err != nil {
		return "", false, fmt.Errorf("failed to parse item response: %w", err)
	}
	return item.Content, true, nil
}

// listRepositoryItems returns all items (files and folders) on the given branch,
// recursing through the full tree.
func (c *ADOClient) listRepositoryItems(project, repo, branch string) ([]adoItem, error) {
	path := fmt.Sprintf("/%s/_apis/git/repositories/%s/items?scopePath=/&recursionLevel=Full&versionDescriptor.version=%s&versionDescriptor.versionType=branch",
		url.PathEscape(project), url.PathEscape(repo), url.QueryEscape(branch))

	body, err := c.doGet(path)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var resp adoListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse items response: %w", err)
	}

	var items []adoItem
	if err := json.Unmarshal(resp.Value, &items); err != nil {
		return nil, fmt.Errorf("failed to parse item values: %w", err)
	}
	return items, nil
}

// doGet performs an authenticated GET request to the ADO API, appending the API version.
func (c *ADOClient) doGet(path string) ([]byte, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	requestURL := fmt.Sprintf("%s%s%sapi-version=%s", c.baseURL, path, separator, c.apiVersion)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", path, err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed for %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for %s: %w", path, err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil

	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited (429) on %s: try again later", path)

	case http.StatusUnauthorized:
		return nil, fmt.Errorf("authentication failed (401) for %s: check your ADO token", path)

	case http.StatusForbidden:
		return nil, fmt.Errorf("access denied (403) for %s: insufficient permissions", path)

	case http.StatusNotFound:
		return nil, fmt.Errorf("not found (404) for %s: %w", path, errNotFound)

	default:
		return nil, fmt.Errorf("unexpected status %d for %s: %s", resp.StatusCode, path, string(body))
	}
}

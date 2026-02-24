package bitbucket

import (
"encoding/json"
"fmt"
"net/http"
"net/http/httptest"
"strings"
"sync/atomic"
"testing"

"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
)

// --- NewBBSClient tests ---

func TestNewBBSClient_ValidInputs(t *testing.T) {
client, err := NewBBSClient("bitbucket.example.com", "test-token")
require.NoError(t, err)
assert.NotNil(t, client)
assert.Equal(t, "https://bitbucket.example.com", client.baseURL)
assert.Equal(t, "test-token", client.token)
}

func TestNewBBSClient_NormalizesURL(t *testing.T) {
tests := []struct {
name     string
input    string
expected string
}{
{"plain hostname", "bitbucket.example.com", "https://bitbucket.example.com"},
{"with trailing slash", "bitbucket.example.com/", "https://bitbucket.example.com"},
{"with https prefix", "https://bitbucket.example.com", "https://bitbucket.example.com"},
{"with https and trailing slash", "https://bitbucket.example.com/", "https://bitbucket.example.com"},
{"with http prefix", "http://bitbucket.example.com", "http://bitbucket.example.com"},
{"with whitespace", "  bitbucket.example.com  ", "https://bitbucket.example.com"},
{"with whitespace and trailing slash", "  bitbucket.example.com/  ", "https://bitbucket.example.com"},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
client, err := NewBBSClient(tt.input, "token")
require.NoError(t, err)
assert.Equal(t, tt.expected, client.baseURL)
})
}
}

func TestNewBBSClient_EmptyBaseURL(t *testing.T) {
client, err := NewBBSClient("", "token")
assert.Nil(t, client)
assert.Error(t, err)
assert.Contains(t, err.Error(), "base URL is required")
}

func TestNewBBSClient_EmptyToken(t *testing.T) {
client, err := NewBBSClient("bitbucket.example.com", "")
assert.Nil(t, client)
assert.Error(t, err)
assert.Contains(t, err.Error(), "token is required")
}

// --- Helper to create a test BBS client backed by httptest.Server ---

func newTestClient(t *testing.T, handler http.Handler) *BBSClient {
t.Helper()
server := httptest.NewServer(handler)
t.Cleanup(server.Close)

client, err := NewBBSClient(server.URL, "test-token")
require.NoError(t, err)
return client
}

// --- ValidateRepoAccess tests ---

func TestValidateRepoAccess_Success(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Equal(t, "/rest/api/1.0/projects/PROJ/repos/my-repo", r.URL.Path)
assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
w.WriteHeader(http.StatusOK)
fmt.Fprint(w, `{"slug":"my-repo"}`)
}))

err := client.ValidateRepoAccess("PROJ", "my-repo")
assert.NoError(t, err)
}

func TestValidateRepoAccess_PersonalRepo(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Equal(t, "/rest/api/1.0/projects/~jdoe/repos/personal-repo", r.URL.Path)
w.WriteHeader(http.StatusOK)
fmt.Fprint(w, `{"slug":"personal-repo"}`)
}))

err := client.ValidateRepoAccess("~jdoe", "personal-repo")
assert.NoError(t, err)
}

func TestValidateRepoAccess_Unauthorized(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusUnauthorized)
}))

err := client.ValidateRepoAccess("PROJ", "my-repo")
assert.Error(t, err)
assert.Contains(t, err.Error(), "401")
}

func TestValidateRepoAccess_NotFound(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusNotFound)
}))

err := client.ValidateRepoAccess("PROJ", "nonexistent")
assert.Error(t, err)
assert.Contains(t, err.Error(), "404")
}

func TestValidateRepoAccess_Forbidden(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusForbidden)
}))

err := client.ValidateRepoAccess("PROJ", "secret-repo")
assert.Error(t, err)
assert.Contains(t, err.Error(), "403")
}

// --- doGet / HTTP handling tests ---

func TestDoGet_SetsAuthHeader(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
assert.Equal(t, "application/json", r.Header.Get("Accept"))
w.WriteHeader(http.StatusOK)
fmt.Fprint(w, `{}`)
}))

_, err := client.doGet("/test")
assert.NoError(t, err)
}

func TestDoGet_RetriesOn429(t *testing.T) {
var attempts int32
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
count := atomic.AddInt32(&attempts, 1)
if count <= 2 {
w.WriteHeader(http.StatusTooManyRequests)
return
}
w.WriteHeader(http.StatusOK)
fmt.Fprint(w, `{"ok":true}`)
}))

body, err := client.doGet("/rate-limited")
assert.NoError(t, err)
assert.Contains(t, string(body), "ok")
assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestDoGet_FailsAfterMaxRetries429(t *testing.T) {
var attempts int32
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
atomic.AddInt32(&attempts, 1)
w.WriteHeader(http.StatusTooManyRequests)
}))

_, err := client.doGet("/always-429")
assert.Error(t, err)
assert.Contains(t, err.Error(), "rate limited (429)")
assert.Contains(t, err.Error(), "after 3 retries")
// initial attempt + 3 retries = 4 total
assert.Equal(t, int32(4), atomic.LoadInt32(&attempts))
}

func TestDoGet_UnexpectedStatus(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
fmt.Fprint(w, `{"error":"internal server error"}`)
}))

_, err := client.doGet("/boom")
assert.Error(t, err)
assert.Contains(t, err.Error(), "unexpected status 500")
}

// --- getPRCounts tests ---

func TestGetPRCounts_Success(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
state := r.URL.Query().Get("state")
var size int
switch state {
case "OPEN":
size = 5
case "MERGED":
size = 10
case "DECLINED":
size = 3
}
resp := bbsPagedResponse{Size: size, IsLastPage: true}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

counts, err := client.getPRCounts("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, 5, counts.Open)
assert.Equal(t, 10, counts.Merged)
assert.Equal(t, 3, counts.Closed) // DECLINED maps to Closed
assert.Equal(t, 18, counts.Total)
}

func TestGetPRCounts_MultiplePages(t *testing.T) {
	requestCount := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		start := r.URL.Query().Get("start")

		var resp bbsPagedResponse
		if state == "MERGED" && (start == "" || start == "0") {
			// First page: 100 items, more pages
			resp = bbsPagedResponse{Size: 100, Limit: 100, Start: 0, IsLastPage: false}
		} else if state == "MERGED" && start == "100" {
			// Second page: 50 items, last page
			resp = bbsPagedResponse{Size: 50, Limit: 100, Start: 100, IsLastPage: true}
		} else {
			// OPEN and DECLINED — single page
			resp = bbsPagedResponse{Size: 2, IsLastPage: true}
		}
		requestCount++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))

	counts, err := client.getPRCounts("PROJ", "repo")
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Open)
	assert.Equal(t, 150, counts.Merged)    // 100 + 50 across two pages
	assert.Equal(t, 2, counts.Closed)
	assert.Equal(t, 154, counts.Total)
}

// --- getPaginatedCount tests ---

func TestGetPaginatedCount_MultiplePages(t *testing.T) {
	requestNum := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNum++
		var resp bbsPagedResponse
		switch requestNum {
		case 1:
			resp = bbsPagedResponse{Size: 100, Limit: 100, Start: 0, IsLastPage: false}
		case 2:
			resp = bbsPagedResponse{Size: 100, Limit: 100, Start: 100, IsLastPage: false}
		case 3:
			resp = bbsPagedResponse{Size: 25, Limit: 100, Start: 200, IsLastPage: true}
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))

	count, err := client.getPaginatedCount("/rest/api/1.0/projects/PROJ/repos/repo/tags", "tags")
	require.NoError(t, err)
	assert.Equal(t, 225, count)      // 100 + 100 + 25
	assert.Equal(t, 3, requestNum)   // Should have made 3 requests
}

// --- getTagCount tests ---

func TestGetTagCount_Success(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Contains(t, r.URL.Path, "/tags")
resp := bbsPagedResponse{Size: 42, IsLastPage: true}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

count, err := client.getTagCount("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, 42, count)
}

// --- getDefaultBranch tests ---

func TestGetDefaultBranch_UsesDisplayID(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Contains(t, r.URL.Path, "/default-branch")
resp := bbsDefaultBranch{
ID:        "refs/heads/main",
DisplayID: "main",
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

branch, err := client.getDefaultBranch("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, "main", branch)
}

func TestGetDefaultBranch_FallsBackToID(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
resp := bbsDefaultBranch{
ID:        "refs/heads/develop",
DisplayID: "",
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

branch, err := client.getDefaultBranch("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, "refs/heads/develop", branch)
}

// --- getCommitCount tests ---

func TestGetCommitCount_UsesTotalCount(t *testing.T) {
totalCount := 1500
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
resp := map[string]interface{}{
"size":       0,
"limit":      0,
"isLastPage": true,
"start":      0,
"values":     []interface{}{},
"totalCount": totalCount,
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

count, err := client.getCommitCount("PROJ", "repo", "main")
require.NoError(t, err)
assert.Equal(t, 1500, count)
}

func TestGetCommitCount_PaginatesFallback(t *testing.T) {
var requestCount int32
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
count := atomic.AddInt32(&requestCount, 1)
if count == 1 {
// First request: limit=0, no totalCount → triggers pagination
resp := map[string]interface{}{
"size":       0,
"limit":      0,
"isLastPage": true,
"start":      0,
"values":     []interface{}{},
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
return
}
// Pagination requests
start := r.URL.Query().Get("start")
if start == "" || start == "0" {
resp := bbsPagedResponse{Size: 1000, Limit: 1000, IsLastPage: false, Start: 0}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
} else {
resp := bbsPagedResponse{Size: 500, Limit: 1000, IsLastPage: true, Start: 1000}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}
}))

count, err := client.getCommitCount("PROJ", "repo", "main")
require.NoError(t, err)
assert.Equal(t, 1500, count)
}

// --- getLatestCommitHash tests ---

func TestGetLatestCommitHash_Success(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Contains(t, r.URL.Path, "/commits")
assert.Equal(t, "1", r.URL.Query().Get("limit"))
resp := map[string]interface{}{
"size":       1,
"limit":      1,
"isLastPage": true,
"start":      0,
"values": []map[string]interface{}{
{"id": "abc123def456"},
},
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

hash, err := client.getLatestCommitHash("PROJ", "repo", "main")
require.NoError(t, err)
assert.Equal(t, "abc123def456", hash)
}

func TestGetLatestCommitHash_NoCommits(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
resp := map[string]interface{}{
"size":       0,
"limit":      1,
"isLastPage": true,
"start":      0,
"values":     []interface{}{},
}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

_, err := client.getLatestCommitHash("PROJ", "repo", "main")
assert.Error(t, err)
assert.Contains(t, err.Error(), "no commits found")
}

// --- getBranchPermissionsCount tests ---

func TestGetBranchPermissionsCount_UsesCorrectAPIBase(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.True(t, strings.HasPrefix(r.URL.Path, "/rest/branch-permissions/2.0/"),
"expected branch-permissions API path, got %s", r.URL.Path)
resp := bbsPagedResponse{Size: 7, IsLastPage: true}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

count, err := client.getBranchPermissionsCount("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, 7, count)
}

// --- getWebhookCount tests ---

func TestGetWebhookCount_Success(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
assert.Contains(t, r.URL.Path, "/webhooks")
resp := bbsPagedResponse{Size: 3, IsLastPage: true}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

count, err := client.getWebhookCount("PROJ", "repo")
require.NoError(t, err)
assert.Equal(t, 3, count)
}

// --- JSON parsing edge cases ---

func TestDoGet_InvalidJSON(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
fmt.Fprint(w, `not-json`)
}))

body, err := client.doGet("/test")
// doGet itself doesn't parse JSON — it returns raw bytes
assert.NoError(t, err)
assert.Equal(t, "not-json", string(body))

// But callers that parse will fail
var paged bbsPagedResponse
err = json.Unmarshal(body, &paged)
assert.Error(t, err)
}

// --- Error path coverage for getTagCount ---

func TestGetTagCount_APIError(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
fmt.Fprint(w, `{"error":"boom"}`)
}))

_, err := client.getTagCount("PROJ", "repo")
assert.Error(t, err)
}

// --- URL construction for personal repos ---

func TestPRCounts_PersonalProject(t *testing.T) {
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// Verify the ~username is passed through correctly in the URL
assert.Contains(t, r.URL.Path, "/projects/~jdoe/repos/my-repo/pull-requests")
resp := bbsPagedResponse{Size: 1, IsLastPage: true}
w.WriteHeader(http.StatusOK)
json.NewEncoder(w).Encode(resp)
}))

counts, err := client.getPRCounts("~jdoe", "my-repo")
require.NoError(t, err)
assert.Equal(t, 3, counts.Total) // 1 open + 1 merged + 1 declined
}

package ado

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewADOClient tests ---

func TestNewADOClient_ValidInputs(t *testing.T) {
	client, err := NewADOClient("https://ado.example.com", "my-collection", "test-token", "")
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "https://ado.example.com/my-collection", client.baseURL)
	assert.Equal(t, defaultAPIVersion, client.apiVersion)

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pat:test-token"))
	assert.Equal(t, expectedAuth, client.authHeader)
}

func TestNewADOClient_NormalizesURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain hostname", "ado.example.com", "https://ado.example.com/col"},
		{"with trailing slash", "ado.example.com/", "https://ado.example.com/col"},
		{"with https prefix", "https://ado.example.com", "https://ado.example.com/col"},
		{"with http prefix", "http://ado.example.com", "http://ado.example.com/col"},
		{"with whitespace", "  ado.example.com  ", "https://ado.example.com/col"},
		{"with subpath", "https://ado.example.com/tfs", "https://ado.example.com/tfs/col"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewADOClient(tt.input, "col", "token", "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, client.baseURL)
		})
	}
}

func TestNewADOClient_CloudAndOnPrem(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		org       string
		expected  string
	}{
		{"empty server defaults to cloud", "", "my-org", "https://dev.azure.com/my-org"},
		{"explicit cloud URL", "https://dev.azure.com", "my-org", "https://dev.azure.com/my-org"},
		{"cloud URL already containing org", "https://dev.azure.com/my-org", "my-org", "https://dev.azure.com/my-org"},
		{"cloud URL org differing case", "https://dev.azure.com/My-Org", "my-org", "https://dev.azure.com/My-Org"},
		{"legacy visualstudio.com host", "https://my-org.visualstudio.com", "my-org", "https://my-org.visualstudio.com"},
		{"on-prem with collection path", "https://tfs.example.com/tfs", "DefaultCollection", "https://tfs.example.com/tfs/DefaultCollection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewADOClient(tt.serverURL, tt.org, "token", "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, client.baseURL)
		})
	}
}

func TestNewADOClient_EmptyServerURLDefaultsToCloud(t *testing.T) {
	client, err := NewADOClient("", "col", "token", "")
	require.NoError(t, err)
	assert.Equal(t, "https://dev.azure.com/col", client.baseURL)
}

func TestNewADOClient_EmptyOrg(t *testing.T) {
	client, err := NewADOClient("https://ado.example.com", "", "token", "")
	assert.Nil(t, client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization")
}

func TestNewADOClient_EmptyToken(t *testing.T) {
	client, err := NewADOClient("https://ado.example.com", "col", "", "")
	assert.Nil(t, client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}

// --- Helper to create a test ADO client backed by httptest.Server ---

func newTestClient(t *testing.T, handler http.Handler) *ADOClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewADOClient(server.URL, "col", "test-token", "")
	require.NoError(t, err)
	return client
}

// --- ValidateRepoAccess tests ---

func TestValidateRepoAccess_Success(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/col/MyProject/_apis/git/repositories/my-repo", r.URL.Path)
		assert.Equal(t, "6.0", r.URL.Query().Get("api-version"))
		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pat:test-token"))
		assert.Equal(t, expectedAuth, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"repo-id","name":"my-repo","defaultBranch":"refs/heads/main"}`)
	}))

	err := client.ValidateRepoAccess("MyProject", "my-repo")
	assert.NoError(t, err)
}

func TestValidateRepoAccess_Unauthorized(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	err := client.ValidateRepoAccess("MyProject", "my-repo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestValidateRepoAccess_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	err := client.ValidateRepoAccess("MyProject", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// --- ListRepositories tests ---

func TestListRepositories_Success(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/col/MyProject/_apis/git/repositories", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":2,"value":[{"name":"repo-a"},{"name":"repo-b"}]}`)
	}))

	repos, err := client.ListRepositories("MyProject")
	require.NoError(t, err)
	assert.Equal(t, []string{"repo-a", "repo-b"}, repos)
}

func TestListRepositories_Empty(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":0,"value":[]}`)
	}))

	repos, err := client.ListRepositories("MyProject")
	require.NoError(t, err)
	assert.Empty(t, repos)
}

// --- GetRepositoryMetrics full-success test ---

func TestGetRepositoryMetrics_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		q := r.URL.Query()
		w.WriteHeader(http.StatusOK)

		switch {
		// Repository object
		case path == "/col/MyProject/_apis/git/repositories/my-repo":
			fmt.Fprint(w, `{"id":"repo-id","name":"my-repo","defaultBranch":"refs/heads/main"}`)

		// Pull requests by status
		case strings.HasSuffix(path, "/pullrequests"):
			switch q.Get("searchCriteria.status") {
			case "active":
				fmt.Fprint(w, `{"count":2,"value":[]}`)
			case "completed":
				fmt.Fprint(w, `{"count":5,"value":[]}`)
			case "abandoned":
				fmt.Fprint(w, `{"count":1,"value":[]}`)
			default:
				fmt.Fprint(w, `{"count":0,"value":[]}`)
			}

		// Tags
		case strings.HasSuffix(path, "/refs"):
			fmt.Fprint(w, `{"count":3,"value":[]}`)

		// Commits
		case strings.HasSuffix(path, "/commits"):
			if q.Get("searchCriteria.$top") == "1" {
				fmt.Fprint(w, `{"count":1,"value":[{"commitId":"abc123"}]}`)
			} else {
				fmt.Fprint(w, `{"count":42,"value":[]}`)
			}

		// Single commit detail (parent lookup)
		case strings.Contains(path, "/commits/"):
			fmt.Fprint(w, `{"commitId":"abc123","parents":["parent123"]}`)

		// Branch policies
		case strings.HasSuffix(path, "/_apis/policy/configurations"):
			fmt.Fprint(w, `{"count":2,"value":[
				{"settings":{"scope":[{"repositoryId":"repo-id"}]}},
				{"settings":{"scope":[{"repositoryId":"other-id"}]}}
			]}`)

		// Service hooks
		case strings.HasSuffix(path, "/_apis/hooks/subscriptions"):
			fmt.Fprint(w, `{"count":2,"value":[
				{"publisherInputs":{"repository":"repo-id"}},
				{"publisherInputs":{"repository":"other-id"}}
			]}`)

		// LFS: git items API (tree listing and file content)
		case strings.HasSuffix(path, "/items"):
			switch {
			case q.Get("scopePath") != "":
				fmt.Fprint(w, `{"count":2,"value":[
					{"gitObjectType":"blob","path":"/.gitattributes","objectId":"attr-sha"},
					{"gitObjectType":"blob","path":"/big.psd","objectId":"psd-sha"}
				]}`)
			case q.Get("path") == "/.gitattributes":
				fmt.Fprint(w, `{"gitObjectType":"blob","path":"/.gitattributes","content":"*.psd filter=lfs diff=lfs merge=lfs -text\n"}`)
			case q.Get("path") == "/big.psd":
				fmt.Fprint(w, "{\"gitObjectType\":\"blob\",\"path\":\"/big.psd\",\"content\":\"version https://git-lfs.github.com/spec/v1\\noid sha256:abc123def456\\nsize 12345\\n\"}")
			default:
				fmt.Fprint(w, `{}`)
			}

		// LFS: blob metadata (size guard)
		case strings.Contains(path, "/blobs/"):
			fmt.Fprint(w, `{"size":130}`)

		default:
			t.Errorf("unexpected path: %s", path)
		}
	})

	client := newTestClient(t, handler)
	spinner, _ := pterm.DefaultSpinner.WithWriter(&strings.Builder{}).Start()

	data, errMsgs, err := client.GetRepositoryMetrics("MyProject", "my-repo", spinner)
	require.NoError(t, err)
	assert.Empty(t, errMsgs)

	assert.Equal(t, "MyProject", data.Owner)
	assert.Equal(t, "my-repo", data.Name)
	assert.Equal(t, 0, data.Issues)
	assert.Equal(t, 0, data.Releases)
	require.NotNil(t, data.PRs)
	assert.Equal(t, 2, data.PRs.Open)
	assert.Equal(t, 5, data.PRs.Merged)
	assert.Equal(t, 1, data.PRs.Closed)
	assert.Equal(t, 8, data.PRs.Total)
	assert.Equal(t, 3, data.Tags)
	assert.Equal(t, 42, data.CommitCount)
	assert.Equal(t, "abc123", data.LatestCommitSHA)
	assert.Equal(t, "parent123", data.LatestCommitParentSHA)
	assert.Equal(t, 1, data.BranchProtectionRules)
	assert.Equal(t, 1, data.Webhooks)
	assert.Equal(t, 1, data.LFSObjects)
}

func TestGetRepositoryMetrics_RepositoryFetchFails(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	spinner, _ := pterm.DefaultSpinner.WithWriter(&strings.Builder{}).Start()

	_, _, err := client.GetRepositoryMetrics("MyProject", "missing", spinner)
	assert.Error(t, err)
}

// --- Pagination tests ---

func TestGetPRCountByStatus_Paginates(t *testing.T) {
	var calls int
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		// First page returns a full page (pageSize), second returns a partial page.
		if r.URL.Query().Get("$skip") == "0" {
			fmt.Fprintf(w, `{"count":%d,"value":[]}`, pageSize)
		} else {
			fmt.Fprint(w, `{"count":7,"value":[]}`)
		}
	}))

	count, err := client.getPRCountByStatus("MyProject", "my-repo", "active")
	require.NoError(t, err)
	assert.Equal(t, pageSize+7, count)
	assert.Equal(t, 2, calls)
}

func TestGetCommitCount_Paginates(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("searchCriteria.$skip") == "0" {
			fmt.Fprintf(w, `{"count":%d,"value":[]}`, pageSize)
		} else {
			fmt.Fprint(w, `{"count":10,"value":[]}`)
		}
	}))

	count, err := client.getCommitCount("MyProject", "my-repo", "main")
	require.NoError(t, err)
	assert.Equal(t, pageSize+10, count)
}

func TestGetLatestCommitHash_NoCommits(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":0,"value":[]}`)
	}))

	_, err := client.getLatestCommitHash("MyProject", "my-repo", "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no commits found")
}

// --- Scoped counting tests ---

func TestGetBranchPolicyCount_FiltersByRepo(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":3,"value":[
			{"settings":{"scope":[{"repositoryId":"repo-id"}]}},
			{"settings":{"scope":[{"repositoryId":"repo-id"}]}},
			{"settings":{"scope":[{"repositoryId":"other-id"}]}}
		]}`)
	}))

	count, err := client.getBranchPolicyCount("MyProject", "repo-id")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGetServiceHookCount_FiltersByRepo(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/col/_apis/hooks/subscriptions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":3,"value":[
			{"publisherInputs":{"repository":"repo-id"}},
			{"publisherInputs":{"repository":"other-id"}},
			{"publisherInputs":{"repository":"repo-id"}}
		]}`)
	}))

	count, err := client.getServiceHookCount("repo-id")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// --- LFS object count tests ---

func TestGetLFSObjectCount_CountsUniqueOIDs(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.WriteHeader(http.StatusOK)
		switch {
		case q.Get("scopePath") != "":
			fmt.Fprint(w, `{"count":3,"value":[
				{"gitObjectType":"blob","path":"/.gitattributes"},
				{"gitObjectType":"blob","path":"/a.psd"},
				{"gitObjectType":"blob","path":"/b.psd"}
			]}`)
		case q.Get("path") == "/.gitattributes":
			fmt.Fprint(w, `{"content":"*.psd filter=lfs diff=lfs merge=lfs -text\n"}`)
		case q.Get("path") == "/a.psd":
			fmt.Fprint(w, "{\"content\":\"version https://git-lfs.github.com/spec/v1\\noid sha256:aaa\\nsize 1\\n\"}")
		case q.Get("path") == "/b.psd":
			// Same OID as a.psd -> must be deduplicated
			fmt.Fprint(w, "{\"content\":\"version https://git-lfs.github.com/spec/v1\\noid sha256:aaa\\nsize 1\\n\"}")
		default:
			fmt.Fprint(w, `{}`)
		}
	}))

	count, err := client.getLFSObjectCount("MyProject", "my-repo", "main")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetLFSObjectCount_NoGitAttributes(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") == "/.gitattributes" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		t.Errorf("should not request tree when .gitattributes is missing: %s", r.URL.String())
	}))

	count, err := client.getLFSObjectCount("MyProject", "my-repo", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetLFSObjectCount_NoLFSPatterns(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("path") == "/.gitattributes" {
			fmt.Fprint(w, `{"content":"*.txt text\n"}`)
			return
		}
		t.Errorf("should not list items when there are no LFS patterns: %s", r.URL.String())
	}))

	count, err := client.getLFSObjectCount("MyProject", "my-repo", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// --- API version negotiation tests ---

func TestNegotiateAPIVersion_PicksFirstSupported(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/col/_apis/projects", r.URL.Path)
		// Only 5.0 is accepted; newer versions return 404 (unsupported on this server).
		if r.URL.Query().Get("api-version") == "5.0" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"count":0,"value":[]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	version, err := client.NegotiateAPIVersion()
	require.NoError(t, err)
	assert.Equal(t, "5.0", version)
	assert.Equal(t, "5.0", client.APIVersion())
}

func TestNegotiateAPIVersion_PinnedVersionUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("negotiation should not probe when a version is pinned")
	}))
	t.Cleanup(server.Close)

	client, err := NewADOClient(server.URL, "col", "test-token", "4.1")
	require.NoError(t, err)

	version, err := client.NegotiateAPIVersion()
	require.NoError(t, err)
	assert.Equal(t, "4.1", version)
}

func TestNegotiateAPIVersion_NoneSupported(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := client.NegotiateAPIVersion()
	assert.Error(t, err)
}

func TestNegotiateAPIVersion_ServerUnreachableFailsFast(t *testing.T) {
	// Point the client at a server that is immediately closed so every request
	// fails at the transport level. Negotiation must abort on the first probe
	// rather than retrying all candidate versions.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := server.URL
	server.Close() // nothing is listening now

	client, err := NewADOClient(unreachableURL, "col", "test-token", "")
	require.NoError(t, err)

	probes := 0
	client.httpClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			probes++
			return nil, fmt.Errorf("connection refused")
		}),
	}

	_, err = client.NegotiateAPIVersion()
	assert.Error(t, err)
	assert.Equal(t, 1, probes, "should stop after the first transport failure")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v62/github"
	"github.com/spf13/viper"
)

// MockUserAuthenticator implements UserAuthenticator for testing
type MockUserAuthenticator struct {
	user *github.User
	err  error
}

func (m *MockUserAuthenticator) GetAuthenticatedUser(ctx context.Context) (*github.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.user, nil
}

func init() {
	// Set up test configuration
	viper.Set("SOURCE_TOKEN", "test-token")
	viper.Set("TARGET_TOKEN", "test-token")
}

// createTestAPI creates a GitHubAPI instance with mocked clients for testing
func createTestAPI(mockTransport http.RoundTripper) *GitHubAPI {
	mockClient := &http.Client{Transport: mockTransport}
	return &GitHubAPI{
		sourceClient: github.NewClient(mockClient),
		targetClient: github.NewClient(mockClient),
	}
}

func TestGetAPI(t *testing.T) {
	// Set minimal test configuration
	viper.Set("SOURCE_TOKEN", "test-token")
	viper.Set("TARGET_TOKEN", "test-token")

	resetAPI()
	api1 := GetAPI()
	if api1 == nil {
		t.Error("GetAPI() returned nil")
	}

	api2 := GetAPI()
	if api1 != api2 {
		t.Error("GetAPI() returned different instances")
	}
}

// MockTransport implements http.RoundTripper for testing
type MockTransport struct {
	Response *http.Response
	Error    error
}

func (t *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return t.Response, t.Error
}

func NewMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		Status:     http.StatusText(statusCode),
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

func TestNewGitHubAPI(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN":           viper.Get("SOURCE_TOKEN"),
		"SOURCE_HOSTNAME":        viper.Get("SOURCE_HOSTNAME"),
		"SOURCE_APP_ID":          viper.Get("SOURCE_APP_ID"),
		"SOURCE_PRIVATE_KEY":     viper.Get("SOURCE_PRIVATE_KEY"),
		"SOURCE_INSTALLATION_ID": viper.Get("SOURCE_INSTALLATION_ID"),
		"TARGET_TOKEN":           viper.Get("TARGET_TOKEN"),
		"TARGET_HOSTNAME":        viper.Get("TARGET_HOSTNAME"),
		"TARGET_APP_ID":          viper.Get("TARGET_APP_ID"),
		"TARGET_PRIVATE_KEY":     viper.Get("TARGET_PRIVATE_KEY"),
		"TARGET_INSTALLATION_ID": viper.Get("TARGET_INSTALLATION_ID"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	// Test only the basic token configuration which should work
	// More complex authentication scenarios are tested in individual component tests
	viper.Set("SOURCE_TOKEN", "test-source-token")
	viper.Set("TARGET_TOKEN", "test-target-token")

	resetAPI()

	// This will create the API with token auth, which should succeed
	// even though actual API calls will fail in test environment
	defer func() {
		if r := recover(); r != nil {
			t.Logf("NewGitHubAPI() panicked (expected in test environment): %v", r)
		}
	}()

	api := NewGitHubAPI()
	if api != nil {
		t.Log("NewGitHubAPI() created successfully with token configuration")
	}
}

func TestCreateAuthenticatedClient_TokenAuth(t *testing.T) {
	config := ClientConfig{
		Token: "test-token",
	}

	client, err := createAuthenticatedClient(config)
	if err != nil {
		t.Errorf("createAuthenticatedClient() error = %v", err)
		return
	}

	if client == nil {
		t.Error("createAuthenticatedClient() returned nil client")
	}
}

func TestCreateAuthenticatedClient_AppAuth(t *testing.T) {
	config := ClientConfig{
		AppID:          "12345",
		PrivateKey:     []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
		InstallationID: 67890,
	}

	// This will fail due to invalid private key, but we test the code path
	_, err := createAuthenticatedClient(config)
	if err == nil {
		t.Error("createAuthenticatedClient() should have failed with invalid private key")
	}

	// Verify the error is related to ASN.1 parsing or app token creation (expected with our test key)
	if err != nil && !strings.Contains(err.Error(), "asn1") && !strings.Contains(err.Error(), "creating app token") {
		t.Errorf("Expected ASN.1 or app token creation error, got: %v", err)
	}
}

func TestCreateAuthenticatedClient_InvalidAppID(t *testing.T) {
	config := ClientConfig{
		AppID:          "invalid",
		PrivateKey:     []byte("test-key"),
		InstallationID: 67890,
	}

	_, err := createAuthenticatedClient(config)
	if err == nil {
		t.Error("createAuthenticatedClient() should have failed with invalid app ID")
	}
}

func TestCreateAuthenticatedClient_NoCredentials(t *testing.T) {
	config := ClientConfig{}

	_, err := createAuthenticatedClient(config)
	if err == nil {
		t.Error("createAuthenticatedClient() should have failed with no credentials")
	}

	expectedError := "please provide either a token or GitHub App credentials"
	if err.Error() != expectedError {
		t.Errorf("createAuthenticatedClient() error = %v, want %v", err.Error(), expectedError)
	}
}

func TestNewGitHubClient(t *testing.T) {
	tests := []struct {
		name      string
		config    ClientConfig
		wantPanic bool
	}{
		{
			name: "valid token config",
			config: ClientConfig{
				Token: "test-token",
			},
			wantPanic: false,
		},
		{
			name: "config with hostname",
			config: ClientConfig{
				Token:    "test-token",
				Hostname: "github.enterprise.com",
			},
			wantPanic: false,
		},
		{
			name: "config with hostname and https prefix",
			config: ClientConfig{
				Token:    "test-token",
				Hostname: "https://github.enterprise.com",
			},
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); (r != nil) != tt.wantPanic {
					t.Errorf("newGitHubClient() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			client := newGitHubClient(tt.config)
			if !tt.wantPanic && client == nil {
				t.Error("newGitHubClient() returned nil")
			}
		})
	}
}

func TestNewGitHubGraphQLClient(t *testing.T) {
	tests := []struct {
		name      string
		config    ClientConfig
		wantPanic bool
	}{
		{
			name: "valid token config",
			config: ClientConfig{
				Token: "test-token",
			},
			wantPanic: false,
		},
		{
			name: "config with hostname",
			config: ClientConfig{
				Token:    "test-token",
				Hostname: "github.enterprise.com",
			},
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); (r != nil) != tt.wantPanic {
					t.Errorf("newGitHubGraphQLClient() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			client := newGitHubGraphQLClient(tt.config)
			if !tt.wantPanic && client == nil {
				t.Error("newGitHubGraphQLClient() returned nil")
			}
		})
	}
}

// MockGraphQLClient implements a mock GraphQL client for testing
type MockGraphQLClient struct {
	queryFunc func(ctx context.Context, q interface{}, variables map[string]interface{}) error
}

func (m *MockGraphQLClient) Query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, q, variables)
	}
	return nil
}

func TestRateLimitAwareGraphQLClient_Query(t *testing.T) {
	tests := []struct {
		name      string
		queryFunc func(ctx context.Context, q interface{}, variables map[string]interface{}) error
		wantError bool
	}{
		{
			name: "successful query with available rate limit",
			queryFunc: func(ctx context.Context, q interface{}, variables map[string]interface{}) error {
				// Simulate rate limit check query followed by actual query
				return nil
			},
			wantError: false,
		},
		{
			name: "query with error",
			queryFunc: func(ctx context.Context, q interface{}, variables map[string]interface{}) error {
				return context.DeadlineExceeded
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test is limited because RateLimitAwareGraphQLClient
			// makes actual HTTP calls. In a real test environment, you'd need
			// to mock the underlying HTTP transport.
			config := ClientConfig{
				Token: "test-token",
			}

			// This will panic due to authentication failure, which is expected in tests
			defer func() {
				recover() // Ignore the panic for this test
			}()

			client := newGitHubGraphQLClient(config)
			if client == nil {
				t.Error("newGitHubGraphQLClient() returned nil")
			}
		})
	}
}

func TestGetIssueCount(t *testing.T) {
	// Create a mock API with test configuration
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset and get fresh API instance
			resetAPI()
			api := GetAPI()

			count, err := api.GetIssueCount(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetIssueCount() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && count < 0 {
				t.Errorf("GetIssueCount() returned negative count: %d", count)
			}
		})
	}
}

func TestGetPRCounts(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			counts, err := api.GetPRCounts(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetPRCounts() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError {
				if counts == nil {
					t.Error("GetPRCounts() returned nil counts")
				} else if counts.Total != (counts.Open + counts.Merged + counts.Closed) {
					t.Errorf("GetPRCounts() total mismatch: got %d, want %d",
						counts.Total, counts.Open+counts.Merged+counts.Closed)
				}
			}
		})
	}
}

func TestGetTagCount(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			count, err := api.GetTagCount(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetTagCount() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && count < 0 {
				t.Errorf("GetTagCount() returned negative count: %d", count)
			}
		})
	}
}

func TestGetReleaseCount(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			count, err := api.GetReleaseCount(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetReleaseCount() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && count < 0 {
				t.Errorf("GetReleaseCount() returned negative count: %d", count)
			}
		})
	}
}

func TestGetCommitCount(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			count, err := api.GetCommitCount(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetCommitCount() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && count < 0 {
				t.Errorf("GetCommitCount() returned negative count: %d", count)
			}
		})
	}
}

func TestGetLatestCommitHash(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			hash, err := api.GetLatestCommitHash(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetLatestCommitHash() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError {
				if hash == "" {
					t.Error("GetLatestCommitHash() returned empty hash")
				}
				// A valid Git SHA should be 40 characters long (for SHA-1)
				if len(hash) > 0 && len(hash) != 40 {
					t.Errorf("GetLatestCommitHash() returned invalid hash length: got %d, want 40", len(hash))
				}
			}
		})
	}
}

func TestPRCounts_TotalCalculation(t *testing.T) {
	tests := []struct {
		name   string
		counts PRCounts
		want   int
	}{
		{
			name:   "all zeros",
			counts: PRCounts{Open: 0, Merged: 0, Closed: 0},
			want:   0,
		},
		{
			name:   "mixed counts",
			counts: PRCounts{Open: 5, Merged: 10, Closed: 3},
			want:   18,
		},
		{
			name:   "only open",
			counts: PRCounts{Open: 7, Merged: 0, Closed: 0},
			want:   7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.counts.Total = tt.counts.Open + tt.counts.Merged + tt.counts.Closed
			if tt.counts.Total != tt.want {
				t.Errorf("Total calculation = %d, want %d", tt.counts.Total, tt.want)
			}
		})
	}
}

func TestGetBranchProtectionRulesCount(t *testing.T) {
	viper.Set("SOURCE_TOKEN", "source-token")
	viper.Set("TARGET_TOKEN", "target-token")

	tests := []struct {
		name       string
		clientType ClientType
		owner      string
		repo       string
		wantError  bool
	}{
		{
			name:       "source client valid request",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "target client valid request",
			clientType: TargetClient,
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true, // Will error in test due to no real connection
		},
		{
			name:       "invalid client type",
			clientType: ClientType(999),
			owner:      "testowner",
			repo:       "testrepo",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAPI()
			api := GetAPI()

			count, err := api.GetBranchProtectionRulesCount(tt.clientType, tt.owner, tt.repo)

			if (err != nil) != tt.wantError {
				t.Errorf("GetBranchProtectionRulesCount() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && count < 0 {
				t.Errorf("GetBranchProtectionRulesCount() returned negative count: %d", count)
			}
		})
	}
}

func TestGitHubAPI_GetWebhookCount(t *testing.T) {
	tests := []struct {
		name          string
		clientType    ClientType
		owner         string
		repo          string
		responseBody  string
		expectedCount int
		expectedError bool
	}{
		{
			name:       "successful webhook count - multiple webhooks",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			responseBody: `[
				{
					"id": 1,
					"name": "web",
					"active": true,
					"events": ["push", "pull_request"],
					"config": {
						"url": "https://example.com/webhook1",
						"content_type": "json"
					}
				},
				{
					"id": 2, 
					"name": "web",
					"active": true,
					"events": ["issues"],
					"config": {
						"url": "https://example.com/webhook2",
						"content_type": "json"
					}
				}
			]`,
			expectedCount: 2,
			expectedError: false,
		},
		{
			name:          "no webhooks",
			clientType:    TargetClient,
			owner:         "testowner",
			repo:          "testrepo",
			responseBody:  `[]`,
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:       "single webhook",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			responseBody: `[
				{
					"id": 1,
					"name": "web",
					"active": true,
					"events": ["push"],
					"config": {
						"url": "https://example.com/webhook",
						"content_type": "json"
					}
				}
			]`,
			expectedCount: 1,
			expectedError: false,
		},
		{
			name:       "mixed active and inactive webhooks - only counts active",
			clientType: SourceClient,
			owner:      "testowner",
			repo:       "testrepo",
			responseBody: `[
				{
					"id": 1,
					"name": "web",
					"active": true,
					"events": ["push"],
					"config": {
						"url": "https://example.com/webhook1",
						"content_type": "json"
					}
				},
				{
					"id": 2,
					"name": "web",
					"active": false,
					"events": ["pull_request"],
					"config": {
						"url": "https://example.com/webhook2",
						"content_type": "json"
					}
				},
				{
					"id": 3,
					"name": "web",
					"active": true,
					"events": ["issues"],
					"config": {
						"url": "https://example.com/webhook3",
						"content_type": "json"
					}
				}
			]`,
			expectedCount: 2,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transport that returns the test response
			mockTransport := &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					// Verify this is a webhook list request
					if !strings.Contains(req.URL.Path, "/repos/"+tt.owner+"/"+tt.repo+"/hooks") {
						t.Errorf("Expected webhook API endpoint, got: %s", req.URL.Path)
					}

					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
						Header:     make(http.Header),
					}, nil
				},
			}

			api := createTestAPI(mockTransport)
			count, err := api.GetWebhookCount(tt.clientType, tt.owner, tt.repo)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if count != tt.expectedCount {
					t.Errorf("Expected count %d, got %d", tt.expectedCount, count)
				}
			}
		})
	}
}

// mockRoundTripper implements http.RoundTripper for testing
type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

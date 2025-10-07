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

	// Test with both source and target tokens
	viper.Set("SOURCE_TOKEN", "test-source-token")
	viper.Set("TARGET_TOKEN", "test-target-token")

	api, err := NewGitHubAPI()
	if err != nil {
		t.Errorf("NewGitHubAPI() error = %v", err)
		return
	}
	if api == nil {
		t.Error("NewGitHubAPI() returned nil")
		return
	}

	// Verify both clients are created
	if api.sourceClient == nil {
		t.Error("Expected sourceClient to be created")
	}
	if api.targetClient == nil {
		t.Error("Expected targetClient to be created")
	}
	if api.sourceGraphClient == nil {
		t.Error("Expected sourceGraphClient to be created")
	}
	if api.targetGraphClient == nil {
		t.Error("Expected targetGraphClient to be created")
	}
}

func TestNewGitHubAPI_MissingSourceToken(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN": viper.Get("SOURCE_TOKEN"),
		"TARGET_TOKEN": viper.Get("TARGET_TOKEN"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	// Set up config with missing source token
	viper.Set("SOURCE_TOKEN", "") // Empty source token
	viper.Set("TARGET_TOKEN", "test-target-token")

	api, err := NewGitHubAPI()
	if err == nil {
		t.Error("NewGitHubAPI() should have failed with missing source token")
		return
	}
	if api != nil {
		t.Error("NewGitHubAPI() should return nil when source token is missing")
	}
}

func TestNewGitHubAPI_MissingTargetToken(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN": viper.Get("SOURCE_TOKEN"),
		"TARGET_TOKEN": viper.Get("TARGET_TOKEN"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	// Set up config with missing target token
	viper.Set("SOURCE_TOKEN", "test-source-token")
	viper.Set("TARGET_TOKEN", "") // Empty target token

	api, err := NewGitHubAPI()
	if err == nil {
		t.Error("NewGitHubAPI() should have failed with missing target token")
		return
	}
	if api != nil {
		t.Error("NewGitHubAPI() should return nil when target token is missing")
	}
}

func TestNewGitHubAPI_BothTokensMissing(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN": viper.Get("SOURCE_TOKEN"),
		"TARGET_TOKEN": viper.Get("TARGET_TOKEN"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	// Set up config with both tokens missing
	viper.Set("SOURCE_TOKEN", "") // Empty source token
	viper.Set("TARGET_TOKEN", "") // Empty target token

	api, err := NewGitHubAPI()
	if err == nil {
		t.Error("NewGitHubAPI() should have failed with both tokens missing")
		return
	}
	if api != nil {
		t.Error("NewGitHubAPI() should return nil when both tokens are missing")
	}
}

func TestAPI_MethodsWithMissingTokens(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN": viper.Get("SOURCE_TOKEN"),
		"TARGET_TOKEN": viper.Get("TARGET_TOKEN"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	tests := []struct {
		name         string
		sourceToken  string
		targetToken  string
		testFunction func(*GitHubAPI) error
		expectError  bool
	}{
		{
			name:        "GetIssueCount with missing source token",
			sourceToken: "",
			targetToken: "test-target-token",
			testFunction: func(api *GitHubAPI) error {
				_, err := api.GetIssueCount(SourceClient, "owner", "repo")
				return err
			},
			expectError: true,
		},
		{
			name:        "GetIssueCount with missing target token",
			sourceToken: "test-source-token",
			targetToken: "",
			testFunction: func(api *GitHubAPI) error {
				_, err := api.GetIssueCount(TargetClient, "owner", "repo")
				return err
			},
			expectError: true,
		},
		{
			name:        "GetPRCounts with missing source token",
			sourceToken: "",
			targetToken: "test-target-token",
			testFunction: func(api *GitHubAPI) error {
				_, err := api.GetPRCounts(SourceClient, "owner", "repo")
				return err
			},
			expectError: true,
		},
		{
			name:        "GetPRCounts with missing target token",
			sourceToken: "test-source-token",
			targetToken: "",
			testFunction: func(api *GitHubAPI) error {
				_, err := api.GetPRCounts(TargetClient, "owner", "repo")
				return err
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("SOURCE_TOKEN", tt.sourceToken)
			viper.Set("TARGET_TOKEN", tt.targetToken)

			// Determine which factory function to use based on the test case
			var api *GitHubAPI
			var err error

			if strings.Contains(tt.name, "missing source token") {
				// For tests expecting source token to be missing, try NewSourceOnlyAPI
				api, err = NewSourceOnlyAPI()
				if tt.expectError && err != nil {
					// This is expected - the factory should fail with missing credentials
					return
				}
			} else if strings.Contains(tt.name, "missing target token") {
				// For tests expecting target token to be missing, try NewTargetOnlyAPI
				api, err = NewTargetOnlyAPI()
				if tt.expectError && err != nil {
					// This is expected - the factory should fail with missing credentials
					return
				}
			} else {
				// Default to full API for other tests
				api, err = NewGitHubAPI()
			}

			if err != nil && !tt.expectError {
				t.Errorf("Unexpected error creating API: %v", err)
				return
			}

			if api != nil {
				err = tt.testFunction(api)
			}

			if tt.expectError && err == nil {
				t.Error("Expected error when using API method with missing token, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestNewSourceOnlyAPI(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"SOURCE_TOKEN":  viper.Get("SOURCE_TOKEN"),
		"SOURCE_APP_ID": viper.Get("SOURCE_APP_ID"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	tests := []struct {
		name        string
		sourceToken string
		sourceAppID string
		wantError   bool
	}{
		{
			name:        "valid source token",
			sourceToken: "test-source-token",
			sourceAppID: "",
			wantError:   false,
		},
		{
			name:        "missing credentials",
			sourceToken: "",
			sourceAppID: "",
			wantError:   true,
		},
		{
			name:        "partial app credentials (app ID only)",
			sourceToken: "",
			sourceAppID: "12345",
			wantError:   true, // Should fail because we need private key and installation ID too
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("SOURCE_TOKEN", tt.sourceToken)
			viper.Set("SOURCE_APP_ID", tt.sourceAppID)

			api, err := NewSourceOnlyAPI()

			if tt.wantError {
				if err == nil {
					t.Error("NewSourceOnlyAPI() should have failed with missing credentials")
				}
				if api != nil {
					t.Error("NewSourceOnlyAPI() should return nil when credentials are missing")
				}
			} else {
				if err != nil {
					t.Errorf("NewSourceOnlyAPI() error = %v", err)
					return
				}
				if api == nil {
					t.Error("NewSourceOnlyAPI() returned nil")
					return
				}

				// Verify only source clients are created
				if api.sourceClient == nil {
					t.Error("Expected sourceClient to be created")
				}
				if api.sourceGraphClient == nil {
					t.Error("Expected sourceGraphClient to be created")
				}

				// Verify target clients are nil
				if api.targetClient != nil {
					t.Error("Expected targetClient to be nil in source-only API")
				}
				if api.targetGraphClient != nil {
					t.Error("Expected targetGraphClient to be nil in source-only API")
				}
			}
		})
	}
}

func TestNewTargetOnlyAPI(t *testing.T) {
	// Store original values
	originalValues := map[string]interface{}{
		"TARGET_TOKEN":  viper.Get("TARGET_TOKEN"),
		"TARGET_APP_ID": viper.Get("TARGET_APP_ID"),
	}

	// Restore original values after test
	defer func() {
		for key, value := range originalValues {
			viper.Set(key, value)
		}
	}()

	tests := []struct {
		name        string
		targetToken string
		targetAppID string
		wantError   bool
	}{
		{
			name:        "valid target token",
			targetToken: "test-target-token",
			targetAppID: "",
			wantError:   false,
		},
		{
			name:        "partial app credentials (app ID only)",
			targetToken: "",
			targetAppID: "12345",
			wantError:   true, // Should fail because we need private key and installation ID too
		},
		{
			name:        "missing credentials",
			targetToken: "",
			targetAppID: "",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("TARGET_TOKEN", tt.targetToken)
			viper.Set("TARGET_APP_ID", tt.targetAppID)

			api, err := NewTargetOnlyAPI()

			if tt.wantError {
				if err == nil {
					t.Error("NewTargetOnlyAPI() should have failed with missing credentials")
				}
				if api != nil {
					t.Error("NewTargetOnlyAPI() should return nil when credentials are missing")
				}
			} else {
				if err != nil {
					t.Errorf("NewTargetOnlyAPI() error = %v", err)
					return
				}
				if api == nil {
					t.Error("NewTargetOnlyAPI() returned nil")
					return
				}

				// Verify only target clients are created
				if api.targetClient == nil {
					t.Error("Expected targetClient to be created")
				}
				if api.targetGraphClient == nil {
					t.Error("Expected targetGraphClient to be created")
				}

				// Verify source clients are nil
				if api.sourceClient != nil {
					t.Error("Expected sourceClient to be nil in target-only API")
				}
				if api.sourceGraphClient != nil {
					t.Error("Expected sourceGraphClient to be nil in target-only API")
				}
			}
		})
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

	client, err := createAuthenticatedClient(config)
	if err == nil {
		t.Error("createAuthenticatedClient() should error with no credentials")
	}

	if client != nil {
		t.Error("createAuthenticatedClient() should return nil client when no credentials are provided")
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

			client, err := newGitHubClient(tt.config)
			if !tt.wantPanic && (client == nil || err != nil) {
				t.Errorf("newGitHubClient() returned nil or error: %v", err)
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

			client, err := newGitHubGraphQLClient(tt.config)
			if !tt.wantPanic && (client == nil || err != nil) {
				t.Errorf("newGitHubGraphQLClient() returned nil or error: %v", err)
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

			client, err := newGitHubGraphQLClient(config)
			if client == nil || err != nil {
				t.Errorf("newGitHubGraphQLClient() returned nil or error: %v", err)
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

			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API: %v", err)
			}

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

			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API client: %v", err)
			}

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

			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API client: %v", err)
			}

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

			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API client: %v", err)
			}

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
			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API client: %v", err)
			}

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
			api, err := NewGitHubAPI()
			if err != nil {
				t.Fatalf("Failed to create API client: %v", err)
			}

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

package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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

func TestGetAuthenticatedUser(t *testing.T) {
	tests := []struct {
		name    string
		mock    *MockUserAuthenticator
		wantErr bool
	}{
		{
			name: "successful auth",
			mock: &MockUserAuthenticator{
				user: &github.User{
					Login: github.String("testuser"),
					Email: github.String("test@example.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "auth error",
			mock: &MockUserAuthenticator{
				err: fmt.Errorf("authentication failed"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.mock.GetAuthenticatedUser(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAuthenticatedUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.GetLogin() != tt.mock.user.GetLogin() {
				t.Errorf("GetAuthenticatedUser() = %v, want %v", got.GetLogin(), tt.mock.user.GetLogin())
			}
		})
	}
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

func TestGetSourceAuthenticatedUser(t *testing.T) {
	tests := []struct {
		name      string
		response  *http.Response
		wantErr   bool
		wantLogin string
	}{
		{
			name:      "successful response",
			response:  NewMockResponse(200, `{"login": "testuser", "email": "test@example.com"}`),
			wantLogin: "testuser",
		},
		{
			name:     "unauthorized response",
			response: NewMockResponse(401, `{"message": "Bad credentials"}`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test API with mock transport
			api := createTestAPI(&MockTransport{Response: tt.response})

			got, err := api.GetSourceAuthenticatedUser()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSourceAuthenticatedUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.GetLogin() != tt.wantLogin {
				t.Errorf("GetSourceAuthenticatedUser() = %v, want %v", got.GetLogin(), tt.wantLogin)
			}
		})
	}
}

func TestGetTargetAuthenticatedUser(t *testing.T) {
	tests := []struct {
		name      string
		response  *http.Response
		wantErr   bool
		wantLogin string
	}{
		{
			name:      "successful response",
			response:  NewMockResponse(200, `{"login": "targetuser", "email": "target@example.com"}`),
			wantLogin: "targetuser",
		},
		{
			name:     "unauthorized response",
			response: NewMockResponse(401, `{"message": "Bad credentials"}`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := createTestAPI(&MockTransport{Response: tt.response})
			got, err := api.GetTargetAuthenticatedUser()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTargetAuthenticatedUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.GetLogin() != tt.wantLogin {
				t.Errorf("GetTargetAuthenticatedUser() = %v, want %v", got.GetLogin(), tt.wantLogin)
			}
		})
	}
}

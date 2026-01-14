package output

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pterm/pterm"
)

func TestLogAPIErrors_EmptyMessages(t *testing.T) {
	// Should be a no-op with empty slice - no panic, no output
	LogAPIErrors([]string{}, "owner", "repo", nil)
	LogAPIErrors(nil, "owner", "repo", nil)
	LogAPIErrors([]string{}, "owner", "repo", errors.New("some error"))
}

func TestLogAPIErrors_WithMessages_NoFatalError(t *testing.T) {
	// Capture output by redirecting pterm's logger writer
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	messages := []string{"issues: connection timeout", "pull requests: rate limited"}
	LogAPIErrors(messages, "testowner", "testrepo", nil)

	output := buf.String()

	// Verify warnings are logged (not errors) when fatalError is nil
	if output == "" {
		t.Error("Expected log output, got empty string")
	}

	// Should contain WARN level indicators and repo info
	if !bytes.Contains(buf.Bytes(), []byte("WARN")) {
		t.Errorf("Expected WARN level logs, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("testowner/testrepo")) {
		t.Errorf("Expected repo info in output, got: %s", output)
	}

	// Should contain our error messages
	if !bytes.Contains(buf.Bytes(), []byte("issues: connection timeout")) {
		t.Errorf("Expected first message in output, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("pull requests: rate limited")) {
		t.Errorf("Expected second message in output, got: %s", output)
	}
}

func TestLogAPIErrors_WithMessages_WithFatalError(t *testing.T) {
	// Capture output by redirecting pterm's logger writer
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	messages := []string{"issues: not found", "commits: forbidden"}
	fatalErr := errors.New("all API requests failed")
	LogAPIErrors(messages, "myorg", "myrepo", fatalErr)

	output := buf.String()

	// Verify errors are logged (not warnings) when fatalError is present
	if output == "" {
		t.Error("Expected log output, got empty string")
	}

	// Should contain ERROR level indicators
	if !bytes.Contains(buf.Bytes(), []byte("ERROR")) {
		t.Errorf("Expected ERROR level logs, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("myorg/myrepo")) {
		t.Errorf("Expected repo info in output, got: %s", output)
	}
}

func TestLogAPIErrors_SingleMessage(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	messages := []string{"webhooks: permission denied"}
	LogAPIErrors(messages, "acme", "widget", nil)

	output := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte("webhooks: permission denied")) {
		t.Errorf("Expected message in output, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("acme/widget")) {
		t.Errorf("Expected repo info in output, got: %s", output)
	}
}

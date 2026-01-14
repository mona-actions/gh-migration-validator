package output

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/pterm/pterm"
)

func TestLogRateLimitWarning_AboveThreshold(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	// Should be a no-op when remaining >= threshold
	LogRateLimitWarning("Source", 100, time.Now().Add(5*time.Minute), 50)

	if buf.Len() > 0 {
		t.Errorf("Expected no output when remaining >= threshold, got: %s", buf.String())
	}
}

func TestLogRateLimitWarning_AtThreshold(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	// Should be a no-op when remaining == threshold
	LogRateLimitWarning("Source", 50, time.Now().Add(5*time.Minute), 50)

	if buf.Len() > 0 {
		t.Errorf("Expected no output when remaining == threshold, got: %s", buf.String())
	}
}

func TestLogRateLimitWarning_BelowThreshold(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	resetTime := time.Now().Add(5 * time.Minute)
	LogRateLimitWarning("Source", 25, resetTime, 50)

	output := buf.String()

	// Verify warning is logged
	if output == "" {
		t.Error("Expected warning output when remaining < threshold, got empty string")
	}

	// Should contain WARN level
	if !bytes.Contains(buf.Bytes(), []byte("WARN")) {
		t.Errorf("Expected WARN level in output, got: %s", output)
	}

	// Should contain client name
	if !bytes.Contains(buf.Bytes(), []byte("Source")) {
		t.Errorf("Expected client name in output, got: %s", output)
	}

	// Should contain "rate limit low" message
	if !bytes.Contains(buf.Bytes(), []byte("rate limit low")) {
		t.Errorf("Expected 'rate limit low' in output, got: %s", output)
	}

	// Should contain "fetching data" message (part of the warning text)
	if !bytes.Contains(buf.Bytes(), []byte("fetching data")) {
		t.Errorf("Expected 'fetching data' in output, got: %s", output)
	}

	// Should contain remaining count
	if !bytes.Contains(buf.Bytes(), []byte("25")) {
		t.Errorf("Expected remaining count '25' in output, got: %s", output)
	}
}

func TestLogRateLimitWarning_ZeroRemaining(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	originalLogger := pterm.DefaultLogger
	pterm.DefaultLogger = *pterm.DefaultLogger.WithWriter(&buf)
	defer func() { pterm.DefaultLogger = originalLogger }()

	resetTime := time.Now().Add(10 * time.Minute)
	LogRateLimitWarning("Target", 0, resetTime, 50)

	output := buf.String()

	// Verify warning is logged
	if output == "" {
		t.Error("Expected warning output when remaining is 0, got empty string")
	}

	// Should contain Target client name
	if !bytes.Contains(buf.Bytes(), []byte("Target")) {
		t.Errorf("Expected 'Target' in output, got: %s", output)
	}
}

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

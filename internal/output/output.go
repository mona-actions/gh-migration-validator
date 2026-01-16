package output

import (
	"fmt"
	"time"

	"github.com/pterm/pterm"
)

// LogRateLimitWarning logs a warning if the rate limit is below the threshold.
// Safe to call - will be a no-op if remaining >= threshold.
func LogRateLimitWarning(clientName string, remaining int, resetAt time.Time, threshold int) {
	if remaining >= threshold {
		return
	}

	waitTime := time.Until(resetAt).Round(time.Second)
	pterm.DefaultLogger.Warn(
		fmt.Sprintf("%s API rate limit low - fetching data may take longer until reset", clientName),
		pterm.DefaultLogger.Args(
			"remaining", remaining,
			"resets_in", waitTime.String(),
		),
	)
}

// LogAPIErrors logs API error messages using pterm's structured logger.
// Uses Error level if fatalError is non-nil (complete failure), Warn level for partial failures.
// Safe to call with empty messages slice - will be a no-op.
func LogAPIErrors(messages []string, owner, repo string, fatalError error) {
	if len(messages) == 0 {
		return
	}

	repoArg := pterm.DefaultLogger.Args("repo", fmt.Sprintf("%s/%s", owner, repo))
	for _, msg := range messages {
		if fatalError != nil {
			pterm.DefaultLogger.Error(msg, repoArg)
		} else {
			pterm.DefaultLogger.Warn(msg, repoArg)
		}
	}
}

package output

import (
	"fmt"

	"github.com/pterm/pterm"
)

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

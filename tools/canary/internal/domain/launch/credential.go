package launch

import "strings"

// OAuthTokenVariable is the variable that carries the long-lived credential of a
// Claude Code session. Only that session's environment holds it.
const OAuthTokenVariable = "CLAUDE_CODE_OAUTH_TOKEN"

// WithoutOAuthToken returns the environment without the Claude Code credential:
// the environment of every process that is not the Claude Code session itself.
func WithoutOAuthToken(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, pair := range environ {
		if name, _, _ := strings.Cut(pair, "="); name != OAuthTokenVariable {
			kept = append(kept, pair)
		}
	}
	return kept
}

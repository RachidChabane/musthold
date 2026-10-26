// Package startup checks the start of a Claude Code session from its stream, line
// by line, while the session runs: every plugin the run gives it is loaded, and the
// model API answers its first call. A session that fails either check is ended
// at once, before it spends its retries or its caps on a start that cannot work.
package startup

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
)

// Reasons the runner gives for ending a session whose start failed.
const (
	ReasonPlugins     = "plugins-not-loaded"
	ReasonUnreachable = "api-unreachable"
)

// Failed says whether a reason the runner ended a session with is a failed start.
func Failed(reason string) bool {
	return reason == ReasonPlugins || reason == ReasonUnreachable
}

// Check follows one session's stream. It is not safe for concurrent use.
type Check struct {
	expected []string
	answered bool
	problem  string
}

// New checks a session that must load the plugins named.
func New(expected []string) *Check {
	return &Check{expected: slices.Clone(expected)}
}

// event holds the members of a stream line the check reads.
type event struct {
	Type         string                 `json:"type"`
	Subtype      string                 `json:"subtype"`
	Plugins      []evidence.Named       `json:"plugins"`
	PluginErrors []pluginError          `json:"plugin_errors"`
	Attempt      int                    `json:"attempt"`
	ErrorStatus  *int                   `json:"error_status"`
	Error        string                 `json:"error"`
	Message      struct{ Model string } `json:"message"`
}

type pluginError struct {
	Plugin  string `json:"plugin"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Line reads one line of the stream and returns the reason to end the session,
// or an empty string.
func (c *Check) Line(line []byte) string {
	var e event
	if json.Unmarshal(line, &e) != nil {
		return ""
	}
	switch {
	case e.Type == evidence.ClaudeTypeSystem && e.Subtype == evidence.ClaudeSubtypeInit:
		return c.init(e)
	case e.Type == evidence.ClaudeTypeAssistant && e.Message.Model != evidence.ClaudeSyntheticModel:
		c.answered = true
	case e.Type == evidence.ClaudeTypeSystem && e.Subtype == evidence.ClaudeSubtypeAPIRetry:
		return c.retry(e)
	}
	return ""
}

// init fails a start whose init event reports a plugin error or lacks a plugin.
func (c *Check) init(e event) string {
	var problems []string
	for _, failed := range e.PluginErrors {
		problems = append(problems, fmt.Sprintf("plugin %s: %s: %s", failed.Plugin, failed.Type, failed.Message))
	}
	loaded := make([]string, 0, len(e.Plugins))
	for _, plugin := range e.Plugins {
		loaded = append(loaded, plugin.Name)
	}
	for _, name := range c.expected {
		if !slices.Contains(loaded, name) {
			problems = append(problems, "plugin "+name+" is not loaded")
		}
	}
	if len(problems) == 0 {
		return ""
	}
	c.problem = strings.Join(problems, "; ")
	return ReasonPlugins
}

// retry fails a start whose call reached no server before any answer: a retry
// with an HTTP status means the API answered, and later retries are the CLI's own
// business.
func (c *Check) retry(e event) string {
	if e.ErrorStatus != nil {
		c.answered = true
		return ""
	}
	if c.answered {
		return ""
	}
	c.problem = fmt.Sprintf("the first call to the model API reached no server (attempt %d, error %q, no HTTP status)", e.Attempt, e.Error)
	return ReasonUnreachable
}

// Problem says what failed, once Line has returned a reason.
func (c *Check) Problem() string { return c.problem }

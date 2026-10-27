package runs

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
)

// QuotaPatterns are the patterns that recognise, in the error of a `result`
// event, the CLI's wording of a usage or rate limit. The canary records them; the
// inputs freeze carries them as `quota_patterns`; each run reads them from there.
func QuotaPatterns() []string {
	return []string{
		`(?i)usage limit`,
		`(?i)rate[ _-]?limit`,
		`(?i)hit your( [a-z]+)? limit`,
		`(?i)limit (reached|exceeded)`,
		`(?i)out of (extra )?usage`,
		`(?i)quota`,
		`\b429\b`,
	}
}

// StatusTooManyRequests is the HTTP status of an API error that is a quota stop.
const StatusTooManyRequests = 429

// ReasonQuota is why the runner ends a session at a quota stop.
const ReasonQuota = "quota-stop"

// ErrQuotaPattern refuses a quota pattern that does not compile.
var ErrQuotaPattern = errors.New("a quota pattern is not a regular expression")

// QuotaWatch reads a session's stream line by line and finds its first quota
// stop: a `result` event with `is_error` whose error matches a quota pattern, or
// an API error event with HTTP status 429. It is not safe for concurrent use.
type QuotaWatch struct {
	patterns []*regexp.Regexp
	// at is when the stop was seen, the time a relative reset counts from.
	at      func() time.Time
	stopped bool
	reset   *time.Time
}

// NewQuotaWatch compiles the patterns; at tells the time a line is read.
func NewQuotaWatch(patterns []string, at func() time.Time) (*QuotaWatch, error) {
	watch := &QuotaWatch{at: at}
	for _, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %v", ErrQuotaPattern, pattern, err)
		}
		watch.patterns = append(watch.patterns, compiled)
	}
	return watch, nil
}

// quotaEvent holds the members of a stream line the watch reads.
type quotaEvent struct {
	Type           string          `json:"type"`
	Subtype        string          `json:"subtype"`
	IsError        bool            `json:"is_error"`
	Result         json.RawMessage `json:"result"`
	Error          json.RawMessage `json:"error"`
	Errors         json.RawMessage `json:"errors"`
	ErrorStatus    *int            `json:"error_status"`
	APIErrorStatus *int            `json:"api_error_status"`
}

// Line reads one line and returns ReasonQuota at the first quota stop, and an
// empty string otherwise.
func (q *QuotaWatch) Line(line []byte) string {
	if q.stopped {
		return ""
	}
	var e quotaEvent
	if json.Unmarshal(line, &e) != nil {
		return ""
	}
	text := strings.Join([]string{raw(e.Result), raw(e.Error), raw(e.Errors)}, " ")
	switch {
	case is429(e.ErrorStatus) || is429(e.APIErrorStatus):
	case e.Type == evidence.ClaudeTypeResult && e.IsError && q.matches(text):
	default:
		return ""
	}
	q.stopped = true
	q.reset = ParseReset(text, q.at())
	return ReasonQuota
}

// Stopped reports whether a quota stop was seen, and the reset time it named.
func (q *QuotaWatch) Stopped() (bool, *time.Time) { return q.stopped, q.reset }

func (q *QuotaWatch) matches(text string) bool {
	for _, pattern := range q.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func is429(status *int) bool { return status != nil && *status == StatusTooManyRequests }

// raw is a member's text: a JSON string unquoted, anything else as written.
func raw(member json.RawMessage) string {
	var text string
	if json.Unmarshal(member, &text) == nil {
		return text
	}
	return string(member)
}

// Forms of a reset time in an error's wording.
var (
	resetEpoch = regexp.MustCompile(`\|(\d{10})\b`)
	resetISO   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	resetClock = regexp.MustCompile(`(?i)resets?(?: at)? (\d{1,2})(?::(\d{2}))?\s*(am|pm)(?:\s*\(([^)]+)\))?`)
)

// hoursHalfDay is how many hours a 12-hour clock counts.
const hoursHalfDay = 12

// ParseReset reads the reset time an error names: a Unix time after a `|`, an
// RFC 3339 time, or a time of day such as "resets 3pm (Europe/Paris)", taken as
// its next occurrence after now in the zone it names, or in now's zone. It
// returns nil when the error names none.
func ParseReset(text string, now time.Time) *time.Time {
	if match := resetEpoch.FindStringSubmatch(text); match != nil {
		seconds, _ := strconv.ParseInt(match[1], 10, 64)
		at := time.Unix(seconds, 0).UTC()
		return &at
	}
	if match := resetISO.FindString(text); match != "" {
		if at, err := time.Parse(time.RFC3339, match); err == nil {
			at = at.UTC()
			return &at
		}
	}
	if match := resetClock.FindStringSubmatch(text); match != nil {
		return nextOClock(match, now)
	}
	return nil
}

func nextOClock(match []string, now time.Time) *time.Time {
	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])
	if hour < 1 || hour > hoursHalfDay || minute > 59 {
		return nil
	}
	hour %= hoursHalfDay
	if strings.EqualFold(match[3], "pm") {
		hour += hoursHalfDay
	}
	zone := now.Location()
	if match[4] != "" {
		if loaded, err := time.LoadLocation(match[4]); err == nil {
			zone = loaded
		}
	}
	local := now.In(zone)
	at := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, zone)
	if !at.After(local) {
		at = at.AddDate(0, 0, 1)
	}
	at = at.UTC()
	return &at
}

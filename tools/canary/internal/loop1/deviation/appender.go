package deviation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	dirMode        = 0o755
	logMode        = 0o644
	dateLayout     = "2006-01-02"
	timestampExt   = ".txt"
	slugSeparator  = "-"
	slugMaxLength  = 60
	slugFallback   = "entry"
	firstSuffix    = 2
	maxSlugRetries = 1000
)

var (
	// ErrTimestamp is returned when an entry is in the log but its timestamp
	// could not be taken: the entry stays, and the missing token is what the
	// analysis refuses on.
	ErrTimestamp = errors.New("the entry is logged but its timestamp is missing")

	slugRun = regexp.MustCompile(`[^a-z0-9]+`)
)

// Stamping takes the timestamp of a file.
type Stamping interface {
	Stamp(ctx context.Context, file string) (Token, error)
}

// Appender appends entries to the deviation log of a work directory. It is safe
// for concurrent use.
type Appender struct {
	layout  study.Layout
	stamper Stamping
	now     func() time.Time
	mu      sync.Mutex
}

// NewAppender returns an appender that stamps the entries the protocol requires
// to be timestamped, and dates an entry that carries no time with now.
func NewAppender(layout study.Layout, stamper Stamping, now func() time.Time) *Appender {
	return &Appender{layout: layout, stamper: stamper, now: now}
}

// Append writes the entry as one line of the log, creating the log with its title
// when it is missing. For a protocol or token-exposed entry it then writes the
// line to deviation-timestamps/<UTC date>-<slug>.txt, a name that never replaces
// an earlier file, and timestamps that file.
func (a *Appender) Append(ctx context.Context, entry study.Deviation) error {
	if entry.Time.IsZero() {
		entry.Time = a.now()
	}
	entry.Time = entry.Time.UTC()
	line, err := entry.Line()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.appendLine(line); err != nil {
		return err
	}
	if !entry.Kind.Timestamped() {
		return nil
	}
	if err := a.timestamp(ctx, entry, line); err != nil {
		return fmt.Errorf("%w: %w", ErrTimestamp, err)
	}
	return nil
}

func (a *Appender) appendLine(line string) error {
	if err := os.MkdirAll(a.layout.Root, dirMode); err != nil {
		return err
	}
	file, err := os.OpenFile(a.layout.Deviations(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, logMode)
	if err != nil {
		return err
	}
	text := line + "\n"
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if info.Size() == 0 {
		text = study.DeviationsTitle + "\n" + text
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (a *Appender) timestamp(ctx context.Context, entry study.Deviation, line string) error {
	if err := os.MkdirAll(a.layout.Timestamps(), dirMode); err != nil {
		return err
	}
	file, err := a.createStampedFile(entry, line)
	if err != nil {
		return err
	}
	_, err = a.stamper.Stamp(ctx, file)
	return err
}

// createStampedFile creates the .txt of an entry under a name no earlier file
// has, and writes the entry line to it.
func (a *Appender) createStampedFile(entry study.Deviation, line string) (string, error) {
	base := entry.Time.Format(dateLayout) + slugSeparator + slug(entry)
	for attempt := 0; attempt < maxSlugRetries; attempt++ {
		name := base
		if attempt > 0 {
			name += slugSeparator + strconv.Itoa(firstSuffix+attempt-1)
		}
		path := filepath.Join(a.layout.Timestamps(), name+timestampExt)
		handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, logMode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := handle.WriteString(line + "\n"); err != nil {
			_ = handle.Close()
			return "", err
		}
		return path, handle.Close()
	}
	return "", fmt.Errorf("%w: no free name for %s", ErrTimestamp, base)
}

// slug names an entry in a file name: its kind and its scope.
func slug(entry study.Deviation) string {
	text := strings.ToLower(string(entry.Kind) + " " + strings.Join(entry.Scope, " "))
	text = strings.Trim(slugRun.ReplaceAllString(text, slugSeparator), slugSeparator)
	if len(text) > slugMaxLength {
		text = strings.Trim(text[:slugMaxLength], slugSeparator)
	}
	if text == "" {
		return slugFallback
	}
	return text
}

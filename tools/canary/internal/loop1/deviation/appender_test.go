package deviation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

var fixedNow = time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)

type recordingStamper struct {
	mu    sync.Mutex
	files []string
	err   error
}

func (r *recordingStamper) Stamp(_ context.Context, file string) (deviation.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files = append(r.files, file)
	return deviation.Token{}, r.err
}

func newAppender(t *testing.T, stamper deviation.Stamping) (*deviation.Appender, study.Layout) {
	t.Helper()
	layout := study.Layout{Root: filepath.Join(t.TempDir(), "work")}
	return deviation.NewAppender(layout, stamper, func() time.Time { return fixedNow }), layout
}

func entry(kind study.Kind, scope ...string) study.Deviation {
	return study.Deviation{Kind: kind, Scope: scope, Section: "6.4", Says: "a run ends in an hour", Happened: "it ended in two"}
}

func TestAppend(t *testing.T) {
	t.Run("the log is created with its title and each entry is one line, read back as written [verifies SP-2613~1]", func(t *testing.T) {
		stamper := &recordingStamper{}
		appender, layout := newAppender(t, stamper)
		for _, kind := range []study.Kind{study.KindQuotaStop, study.KindHarnessError} {
			if err := appender.Append(t.Context(), entry(kind, "parcel-control-1")); err != nil {
				t.Fatal(err)
			}
		}
		text, err := os.ReadFile(layout.Deviations())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(text), study.DeviationsTitle+"\n") || strings.Count(string(text), study.DeviationsTitle) != 1 {
			t.Errorf("log %q", text)
		}
		parsed := study.ParseDeviations(string(text))
		if len(parsed) != 2 || parsed[0].Kind != study.KindQuotaStop || !parsed[0].Time.Equal(fixedNow) || parsed[1].Kind != study.KindHarnessError {
			t.Errorf("entries %+v", parsed)
		}
		if len(stamper.files) != 0 {
			t.Errorf("stamped %v", stamper.files)
		}
	})

	t.Run("an invalid entry writes nothing [verifies SP-2613~1]", func(t *testing.T) {
		appender, layout := newAppender(t, &recordingStamper{})
		bad := entry(study.KindProtocol, "all")
		bad.Says = ""
		if err := appender.Append(t.Context(), bad); !errors.Is(err, study.ErrDeviation) {
			t.Fatalf("error %v", err)
		}
		if _, err := os.Stat(layout.Deviations()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("log: %v", err)
		}
	})

	t.Run("concurrent appends lose no entry and write the title once [verifies SP-2613~1]", func(t *testing.T) {
		appender, layout := newAppender(t, &recordingStamper{})
		const writers = 32
		var wait sync.WaitGroup
		for range writers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				if err := appender.Append(context.Background(), entry(study.KindQuotaStop, "all")); err != nil {
					t.Error(err)
				}
			}()
		}
		wait.Wait()
		text, _ := os.ReadFile(layout.Deviations())
		if got := len(study.ParseDeviations(string(text))); got != writers || strings.Count(string(text), study.DeviationsTitle) != 1 {
			t.Errorf("%d entries in %q", got, text)
		}
	})
}

func TestAppendTimestamped(t *testing.T) {
	t.Run("a protocol and a token-exposed entry are stored under a dated slug and stamped [verifies SP-2614~1]", func(t *testing.T) {
		stamper := &recordingStamper{}
		appender, layout := newAppender(t, stamper)
		if err := appender.Append(t.Context(), entry(study.KindProtocol, "boxoffice", "Parcel One")); err != nil {
			t.Fatal(err)
		}
		if err := appender.Append(t.Context(), entry(study.KindTokenExposed, "all")); err != nil {
			t.Fatal(err)
		}
		want := []string{
			filepath.Join(layout.Timestamps(), "2026-11-02-protocol-boxoffice-parcel-one.txt"),
			filepath.Join(layout.Timestamps(), "2026-11-02-token-exposed-all.txt"),
		}
		if strings.Join(stamper.files, "|") != strings.Join(want, "|") {
			t.Fatalf("stamped %v", stamper.files)
		}
		stored, _ := os.ReadFile(want[0])
		logged, _ := os.ReadFile(layout.Deviations())
		if !strings.Contains(string(logged), strings.TrimSpace(string(stored))) || !strings.HasPrefix(string(stored), "- 2026-11-02T09:30:00Z | protocol | boxoffice,Parcel One | §6.4") {
			t.Errorf("stored %q", stored)
		}
	})

	t.Run("two entries of one day and one slug keep their own files [verifies SP-2614~1]", func(t *testing.T) {
		stamper := &recordingStamper{}
		appender, layout := newAppender(t, stamper)
		for range 3 {
			if err := appender.Append(t.Context(), entry(study.KindProtocol, "all")); err != nil {
				t.Fatal(err)
			}
		}
		names := map[string]bool{}
		for _, file := range stamper.files {
			names[filepath.Base(file)] = true
		}
		for _, name := range []string{"2026-11-02-protocol-all.txt", "2026-11-02-protocol-all-2.txt", "2026-11-02-protocol-all-3.txt"} {
			if !names[name] {
				t.Errorf("%s is missing from %v (%s)", name, names, layout.Timestamps())
			}
		}
	})

	t.Run("a failed timestamp keeps the entry in the log and says the token is missing [verifies SP-2614~1]", func(t *testing.T) {
		appender, layout := newAppender(t, &recordingStamper{err: deviation.ErrAuthority})
		err := appender.Append(t.Context(), entry(study.KindProtocol, "all"))
		if !errors.Is(err, deviation.ErrTimestamp) || !errors.Is(err, deviation.ErrAuthority) {
			t.Fatalf("error %v", err)
		}
		logged, _ := os.ReadFile(layout.Deviations())
		if len(study.ParseDeviations(string(logged))) != 1 {
			t.Errorf("log %q", logged)
		}
	})
}

func TestLogDeviationTimestamp(t *testing.T) {
	arguments := func(work string) []string {
		return []string{"--work", work, "--scope", "all", "--section", "6.3", "--says", "x", "--happened", "y"}
	}
	t.Run("a protocol entry is stamped, and a missing token exits 1 with the entry kept [verifies SP-2615~1]", func(t *testing.T) {
		work := filepath.Join(t.TempDir(), "work")
		stamper := &recordingStamper{}
		var stdout, stderr strings.Builder
		if code := deviation.RunWith(t.Context(), arguments(work), &stdout, &stderr, stamper); code != deviation.ExitOK || len(stamper.files) != 1 {
			t.Fatalf("code %d, stamped %v, %q", code, stamper.files, stderr.String())
		}
		stamper.err = deviation.ErrAuthority
		if code := deviation.RunWith(t.Context(), arguments(work), &stdout, &stderr, stamper); code != deviation.ExitFailed || !strings.Contains(stderr.String(), "timestamp is missing") {
			t.Errorf("code %d, %q", code, stderr.String())
		}
		text, _ := os.ReadFile(study.Layout{Root: work}.Deviations())
		if len(study.ParseDeviations(string(text))) != 2 {
			t.Errorf("log %q", text)
		}
	})
}

func TestLogDeviation(t *testing.T) {
	t.Run("the verb refuses what the log cannot hold with a usage error and logs a valid entry [verifies SP-2615~1]", func(t *testing.T) {
		work := filepath.Join(t.TempDir(), "work")
		var stdout, stderr strings.Builder
		code := deviation.Main(t.Context(), []string{"--work", work, "--kind", "everything", "--scope", "all", "--section", "6.4", "--says", "x", "--happened", "y"}, &stdout, &stderr)
		if code != deviation.ExitUsage || !strings.Contains(stderr.String(), "kind") {
			t.Errorf("code %d, %q", code, stderr.String())
		}
		code = deviation.Main(t.Context(), []string{"--nonsense"}, &stdout, &stderr)
		if code != deviation.ExitUsage {
			t.Errorf("code %d", code)
		}
		code = deviation.Main(t.Context(), []string{"--work", work, "--kind", "quota-stop", "--scope", "a, b", "--section", "§6.5", "--says", "x", "--happened", "y"}, &stdout, &stderr)
		if code != deviation.ExitOK {
			t.Fatalf("code %d, %q", code, stderr.String())
		}
		text, _ := os.ReadFile(study.Layout{Root: work}.Deviations())
		parsed := study.ParseDeviations(string(text))
		if len(parsed) != 1 || strings.Join(parsed[0].Scope, ",") != "a,b" || parsed[0].Section != "6.5" {
			t.Errorf("entries %+v", parsed)
		}
	})
}

package freeze_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// scriptedExec answers a command by its name and first arguments.
type scriptedExec struct {
	answers map[string]string
	calls   *[]string
	fail    bool
}

func (s scriptedExec) Output(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	if s.calls != nil {
		*s.calls = append(*s.calls, line)
	}
	if s.fail {
		return nil, freeze.ErrCommand
	}
	for prefix, answer := range s.answers {
		if strings.HasPrefix(line, prefix) {
			return []byte(answer), nil
		}
	}
	return nil, freeze.ErrCommand
}

func TestEngineAdapters(t *testing.T) {
	listing := `{"name":"default","cpus":1,"memory":1,"disk":1}
{"name":"musthold-loop-1","status":"Running","cpus":2,"memory":4294967296,"disk":32212254720}
`
	var calls []string
	exec := scriptedExec{calls: &calls, answers: map[string]string{
		"/study/docker version": "27.5.1\n", "/study/docker image inspect": "sha256:abc\n",
		"colima version": "colima version 0.8.1\ngit commit: 1234\n", "colima list --json": listing,
		"uname -srm": "Darwin 25.6.0 arm64\n",
	}}
	engine := freeze.CLIEngine{Exec: exec, Docker: []string{"/study/docker"}}

	t.Run("the engine's versions, settings and image id are read through the study's docker client and colima [verifies SP-2617~1]", func(t *testing.T) {
		docker, err := engine.DockerVersion(t.Context())
		colima, err2 := engine.ColimaVersion(t.Context())
		settings, err3 := engine.Settings(t.Context(), freeze.ColimaProfile)
		image, err4 := engine.ImageID(t.Context(), "ref")
		if err != nil || err2 != nil || err3 != nil || err4 != nil {
			t.Fatal(err, err2, err3, err4)
		}
		if docker != "27.5.1" || colima != "0.8.1" || image != "sha256:abc" {
			t.Errorf("%q %q %q", docker, colima, image)
		}
		if settings != (freeze.ColimaSettings{CPUs: "2", MemoryBytes: "4294967296", DiskBytes: "32212254720"}) {
			t.Errorf("settings %+v", settings)
		}
		if _, err := engine.Settings(t.Context(), "absent"); !errors.Is(err, freeze.ErrUnreadable) {
			t.Errorf("an absent profile: %v", err)
		}
	})

	t.Run("the host is read with uname [verifies SP-2617~1]", func(t *testing.T) {
		host, err := freeze.UnameHost{Exec: exec}.Info(t.Context())
		if err != nil || host != (freeze.HostInfo{OS: "Darwin", Version: "25.6.0", Arch: "arm64"}) {
			t.Errorf("host %+v, %v", host, err)
		}
		if _, err := (freeze.UnameHost{Exec: scriptedExec{answers: map[string]string{"uname": "Darwin\n"}}}).Info(t.Context()); !errors.Is(err, freeze.ErrUnreadable) {
			t.Errorf("a short answer: %v", err)
		}
	})
}

func TestGitHubReleases(t *testing.T) {
	var calls []string
	exec := scriptedExec{calls: &calls, answers: map[string]string{
		"gh api --paginate repos/RachidChabane/shallnot/tags":      "v0.5.1\nv0.5.0\n",
		"gh api repos/RachidChabane/shallnot/compare/pin...v0.5.1": "ahead\n",
		"gh api repos/RachidChabane/shallnot/compare/pin...v0.5.0": "behind\n",
		"gh api repos/RachidChabane/shallnot/compare/pin...v0.4.0": "identical\n",
	}}
	releases := freeze.GitHubReleases{Exec: exec, Repository: freeze.ShallnotRepository}

	t.Run("the tags are listed and a tag that is ahead of the commit or identical to it holds it [verifies SP-2618~1]", func(t *testing.T) {
		tags, err := releases.Tags(t.Context())
		if err != nil || !slices.Equal(tags, []string{"v0.5.1", "v0.5.0"}) {
			t.Fatalf("tags %v, %v", tags, err)
		}
		for tag, want := range map[string]bool{"v0.5.1": true, "v0.5.0": false, "v0.4.0": true} {
			if got, err := releases.Contains(t.Context(), tag, "pin"); err != nil || got != want {
				t.Errorf("%s: %v, %v", tag, got, err)
			}
		}
	})
	t.Run("the first release is found through the query [verifies SP-2618~1]", func(t *testing.T) {
		got, err := freeze.FirstRelease(t.Context(), releases, "pin")
		if err != nil || got != "v0.5.1" {
			t.Errorf("release %q, %v", got, err)
		}
	})
}

func TestWorkFiles(t *testing.T) {
	work := t.TempDir()
	layout := study.Layout{Root: work}
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

	t.Run("the construction records are the lines construct appends, blank lines skipped, and a broken line is refused [verifies SP-2620~2]", func(t *testing.T) {
		writer := records.NewFileWriter(layout)
		for _, entry := range []records.Entry{{Call: &records.Call{CodexVersion: "c", Start: start}}, {Drop: &records.Drop{Domain: "d"}}} {
			if err := writer.Append(entry); err != nil {
				t.Fatal(err)
			}
		}
		log, err := freeze.FileConstruction{Layout: layout}.Records(t.Context())
		if err != nil || len(log.Calls) != 1 || !log.Calls[0].Start.Equal(start) || len(log.Drops) != 1 {
			t.Fatalf("records %+v, %v", log, err)
		}
		write(t, work, "construction/records.jsonl", "not json\n")
		if _, err := (freeze.FileConstruction{Layout: layout}).Records(t.Context()); !errors.Is(err, records.ErrMalformedRecords) {
			t.Errorf("a broken line: %v", err)
		}
	})

	t.Run("the starts of the runs come from the run directories [verifies SP-2625~1]", func(t *testing.T) {
		write(t, work, "runs/alpha-control-1-1/records/run.json", `{"startedAt":"2026-11-02T08:00:00Z"}`)
		write(t, work, "runs/alpha-control-1-2/records/run.json", `{"startedAt":"2026-11-02T09:00:00Z"}`)
		starts, err := freeze.FileRuns{Layout: layout}.Starts(t.Context())
		if err != nil || len(starts) != 2 || starts[0].Name != "alpha-control-1-1" || !starts[1].Start.Equal(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)) {
			t.Errorf("starts %+v, %v", starts, err)
		}
		write(t, work, "runs/broken/records/run.json", "{")
		if _, err := (freeze.FileRuns{Layout: layout}).Starts(t.Context()); !errors.Is(err, freeze.ErrUnreadable) {
			t.Errorf("a broken record: %v", err)
		}
	})
}

func TestGit(t *testing.T) {
	built := shared(t)
	repo := freeze.Git{Dir: built.root, Exec: freeze.OSExec{}}
	ctx := context.Background()

	t.Run("tags peel, parents and ancestry follow the history, and the files of a commit are listed [verifies SP-2616~1]", func(t *testing.T) {
		prereg, err := repo.PeelTag(ctx, freeze.PreregTag)
		if err != nil {
			t.Fatal(err)
		}
		inputs, err := repo.PeelTag(ctx, freeze.InputsTag)
		if err != nil {
			t.Fatal(err)
		}
		if descends, err := repo.IsAncestor(ctx, prereg, inputs); err != nil || !descends {
			t.Errorf("prereg is not an ancestor: %v, %v", descends, err)
		}
		if descends, err := repo.IsAncestor(ctx, inputs, prereg); err != nil || descends {
			t.Errorf("inputs is an ancestor of prereg: %v, %v", descends, err)
		}
		if _, err := repo.PeelTag(ctx, "absent"); !errors.Is(err, freeze.ErrNoTag) {
			t.Errorf("an absent tag: %v", err)
		}
		changed, err := repo.Changed(ctx, inputs)
		if err != nil || len(changed) != 2 {
			t.Errorf("changed %v, %v", changed, err)
		}
		content, err := repo.Show(ctx, freeze.InputsTag, freeze.ShallnotPinFile)
		if err != nil || strings.TrimSpace(string(content)) != freeze.ExpectedShallnotPin {
			t.Errorf("show %q, %v", content, err)
		}
		files, err := repo.TreeFiles(ctx, freeze.InputsTag, "fixtures")
		if err != nil || !slices.Equal(files, []string{"fixtures/alpha/alpha.py"}) {
			t.Errorf("tree files %v, %v", files, err)
		}
		tracked, err := repo.Tracked(ctx, "fixtures")
		if err != nil || !slices.Equal(tracked, files) {
			t.Errorf("tracked %v, %v", tracked, err)
		}
	})

	t.Run("the commits between two revisions that touch a directory come oldest first [verifies SP-2616~1]", func(t *testing.T) {
		prereg, _ := repo.PeelTag(ctx, freeze.PreregTag)
		between, err := repo.Between(ctx, prereg, "HEAD", freeze.ProductDirs)
		if err != nil || len(between) != 1 {
			t.Errorf("between %v, %v", between, err)
		}
		none, err := repo.Between(ctx, prereg, "HEAD", []string{"umbrella"})
		if err != nil || len(none) != 0 {
			t.Errorf("a directory no commit touches: %v, %v", none, err)
		}
	})

	t.Run("a repository command ignores the variables that redirect it [verifies SP-2616~1]", func(t *testing.T) {
		other := t.TempDir()
		t.Setenv("GIT_DIR", other)
		t.Setenv("GIT_WORK_TREE", other)
		head, err := repo.Head(ctx)
		if err != nil || len(head) != 40 {
			t.Errorf("head %q, %v", head, err)
		}
	})

	t.Run("a modified tracked file is listed and an untracked one is not [verifies SP-2616~1]", func(t *testing.T) {
		scratch := newStudy(t, scenario{})
		own := freeze.Git{Dir: scratch.root, Exec: freeze.OSExec{}}
		write(t, scratch.root, "untracked.txt", "x")
		if modified, err := own.Modified(ctx); err != nil || len(modified) != 0 {
			t.Errorf("modified %v, %v", modified, err)
		}
		write(t, scratch.root, "fixtures/alpha/alpha.py", "changed")
		if modified, err := own.Modified(ctx); err != nil || !slices.Equal(modified, []string{"fixtures/alpha/alpha.py"}) {
			t.Errorf("modified %v, %v", modified, err)
		}
	})
}

func TestMains(t *testing.T) {
	t.Run("resolve exits 2 on a usage error and 1 when the product or the repository cannot be read [verifies SP-2623~1]", func(t *testing.T) {
		var stdout, stderr strings.Builder
		main := freeze.ResolveCommand{Wire: func(freeze.Options, box.Runner) (freeze.ResolveWiring, error) {
			return freeze.ResolveWiring{Preparer: fakePreparer{}, Canary: fakeCanary{}}, nil
		}}.Main
		if code := main(t.Context(), []string{"--nonsense"}, &stdout, &stderr); code != freeze.ExitUsage {
			t.Errorf("usage: code %d", code)
		}
		empty := t.TempDir()
		code := main(t.Context(), []string{"--repo", empty, "--work", empty, "--product", empty, "--config", filepath.Join(empty, "none.json")}, &stdout, &stderr)
		if code != freeze.ExitFailed || !strings.Contains(stderr.String(), "resolve:") {
			t.Errorf("code %d, %q", code, stderr.String())
		}
	})
}

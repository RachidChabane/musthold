package app

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/codexconfig"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
)

// Places Codex keeps its state in, relative to its home.
const (
	codexConfigFile      = "config.toml"
	codexSessionsDir     = "sessions"
	codexHomeVariable    = "CODEX_HOME"
	codexDefaultHomeDir  = ".codex"
	rolloutGlob          = "rollout-*.jsonl"
	rolloutThreadPattern = "-%s.jsonl"
	rolloutDayLayout     = "2006/01/02"
)

// codexHome is the directory the session's Codex keeps its configuration and its
// sessions in: the dedicated one, the CODEX_HOME of the environment, or ~/.codex.
func (r Runner) codexHome(options Options) string {
	if options.CodexHome != "" {
		return options.CodexHome
	}
	if home := variable(r.Env, codexHomeVariable); home != "" {
		return home
	}
	return filepath.Join(variable(r.Env, "HOME"), codexDefaultHomeDir)
}

// restoreCodexTrust takes out of Codex's configuration file the table that trusts
// the run's project, which Codex persists there when the session trusts it, and
// reads the file again to check that the table is gone and the rest of the file is
// what it was. It returns what the session wrote and what was done about it; a
// file that does not name the project has nothing to restore.
func (r Runner) restoreCodexTrust(options Options, l layout) []evidence.ConfigWrite {
	if options.Harness != harness.FlagCodex {
		return nil
	}
	path := filepath.Join(r.codexHome(options), codexConfigFile)
	content, err := r.Files.Read(path)
	if err != nil || !codexconfig.NamesProject(content, l.project) {
		return nil
	}
	write := evidence.ConfigWrite{Path: path}
	removal := codexconfig.RemoveProjectTable(content, l.project)
	if !removal.Found {
		return []evidence.ConfigWrite{write}
	}
	write.Table = removal.Table
	if r.Files.Replace(path, removal.Cleaned) != nil {
		return []evidence.ConfigWrite{write}
	}
	write.Removed = true
	after, err := r.Files.Read(path)
	write.Verified = err == nil && bytes.Equal(after, removal.Cleaned) && !codexconfig.NamesProject(after, l.project)
	return []evidence.ConfigWrite{write}
}

// collectRollouts copies the rollout of the session's main thread, and the rollouts
// of the threads it spawned, into the records. `codex exec --json` prints neither
// the spawn calls nor the identity of the threads they start; the rollouts do.
func (r Runner) collectRollouts(options Options, l layout, session evidence.Session) error {
	if options.Harness != harness.FlagCodex {
		return nil
	}
	stream, err := r.Files.Read(l.record(evidence.StreamFile))
	if err != nil {
		return err
	}
	lines, err := evidence.Lines(evidence.StreamFile, stream)
	if err != nil {
		return err
	}
	threadID := evidence.DecodeCodex(lines).ThreadID
	if threadID == "" {
		return nil
	}
	rollouts, err := r.sessionRollouts(options, session)
	if err != nil {
		return err
	}
	mainSuffix := fmt.Sprintf(rolloutThreadPattern, threadID)
	mainAt := slices.IndexFunc(rollouts, func(path string) bool { return strings.HasSuffix(path, mainSuffix) })
	if mainAt < 0 {
		return nil
	}
	if err := r.copyFile(rollouts[mainAt], l.record(evidence.RolloutMainFile)); err != nil {
		return err
	}
	for index, path := range rollouts {
		if index == mainAt {
			continue
		}
		first, err := r.Files.FirstLine(path)
		if err != nil {
			return err
		}
		if parent, spawned := evidence.RolloutParent(first); spawned && parent == threadID {
			if err := r.copyFile(path, filepath.Join(l.record(evidence.RolloutChildrenDir), filepath.Base(path))); err != nil {
				return err
			}
		}
	}
	return nil
}

// sessionRollouts lists the rollout files of the days the session ran on, by the
// local clock and by UTC: Codex files a rollout under the day it started.
func (r Runner) sessionRollouts(options Options, session evidence.Session) ([]string, error) {
	var found []string
	for _, day := range rolloutDays(session.StartedAt, session.EndedAt) {
		paths, err := r.Files.Glob(filepath.Join(r.codexHome(options), codexSessionsDir, day, rolloutGlob))
		if err != nil {
			return nil, err
		}
		found = append(found, paths...)
	}
	return found, nil
}

// rolloutDays lists the day directories, in either time zone, of the given times.
func rolloutDays(times ...time.Time) []string {
	var days []string
	for _, moment := range times {
		for _, zoned := range []time.Time{moment.Local(), moment.UTC()} {
			day := filepath.FromSlash(zoned.Format(rolloutDayLayout))
			if !slices.Contains(days, day) {
				days = append(days, day)
			}
		}
	}
	return days
}

func (r Runner) copyFile(from, to string) error {
	data, err := r.Files.Read(from)
	if err != nil {
		return err
	}
	return r.Files.Write(to, data, false)
}

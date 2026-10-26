package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// Exit codes of a scan that asks a host (ADR 0005).
const (
	ExitFinished     = 0
	ExitAwaitingHost = 3
)

// Errors of a scan that asks a host.
var (
	// ErrNoHost is returned when a scan that asks a host has no host store.
	ErrNoHost = errors.New("a scan that asks a host needs the host store")
	// ErrNothingToResume is returned by --resume when no scan session is open.
	ErrNothingToResume = errors.New("nothing to resume: no scan session is open")
	// ErrSessionMismatch is returned by --resume when no open scan session has
	// the binding of this scan.
	ErrSessionMismatch = errors.New("no open scan session matches this scan")
)

// HostDeps are the ports a scan asks its host through, beside the ports every
// scan has.
type HostDeps struct {
	// Store is the host directory of the repository, shared with `prove`.
	Store hostproto.HostStore
	// Memo keeps the witness facts the request ids derive from: the admission
	// of each witness set and the coverage of the witnesses.
	Memo ports.ExecutionMemo
	// Sources keeps the quotable sources under the scan's folder.
	Sources SourceStore
	// Head reads the README of the head commit.
	Head    HeadFileReader
	Entropy io.Reader
	// Plugin is the plugin the executor roles belong to.
	Plugin string
}

// HostResult is what one invocation of a scan that asks a host established:
// the report once every answer is in, or the tickets it waits for.
type HostResult struct {
	Session hostproto.Session
	Wave    int
	// Finished is set when the scan wrote its statement; Report is then set.
	Finished bool
	Report   Report
	// Pending are the tickets of the session that wait for an answer.
	Pending []hostproto.Ticket
}

// ExitCode is 3 while tickets wait for the host, 0 once the scan finished.
func (r HostResult) ExitCode() int {
	if r.Finished {
		return ExitFinished
	}
	return ExitAwaitingHost
}

// RunHost runs one invocation of a scan that asks its host (ADR 0014): it
// reruns every stage from the sources and the host store, as far as the
// answers allow, and either writes the scan's statement or leaves tickets for
// the host. A stage with nothing to ask continues in the same invocation; the
// suite runs only in the invocation that finishes.
func (s *Service) RunHost(ctx context.Context, request Request) (HostResult, error) {
	if s.deps.Host == nil || s.deps.Host.Store == nil {
		return HostResult{}, ErrNoHost
	}
	started := s.deps.Clock()
	request.Workers = s.workers(ctx, request.Workers)
	run, err := s.openHostRun(ctx, request)
	if err != nil {
		return HostResult{}, err
	}
	defer run.release()
	result := HostResult{Session: run.bridge.Session, Wave: run.bridge.Wave}
	finished, err := run.stages(ctx, started)
	if err != nil {
		return HostResult{}, err
	}
	if !finished.done {
		pending, err := run.bridge.FinishWave()
		if err != nil {
			return HostResult{}, err
		}
		result.Pending = pending
		return result, nil
	}
	if err := run.consume(); err != nil {
		return HostResult{}, err
	}
	if err := run.bridge.End(); err != nil {
		return HostResult{}, fmt.Errorf("end the scan session: %w", err)
	}
	result.Finished, result.Report = true, finished.report
	return result, nil
}

// openHostRun resolves what the scan is bound to, opens or resumes its session
// and saves its quotable sources.
func (s *Service) openHostRun(ctx context.Context, request Request) (*hostRun, error) {
	commit, err := s.commitOf(ctx)
	if err != nil {
		return nil, err
	}
	change, err := s.changeOf(ctx, request)
	if err != nil {
		return nil, err
	}
	quotable, err := s.quotableSources(ctx, change)
	if err != nil {
		return nil, err
	}
	tree, err := s.treeDigest(ctx)
	if err != nil {
		return nil, err
	}
	bound := newBinding(request, commit, tree, change, quotable)
	bridge, err := s.openSession(bound, request.Resume)
	if err != nil {
		return nil, err
	}
	saved, err := s.saveSources(commit, quotable)
	if err != nil {
		return nil, err
	}
	run := &hostRun{
		service: s, request: request, commit: commit, tree: tree, change: change, sources: saved,
		bridge: bridge, asker: newAsker(s.deps.Host.Store, bridge), state: newPromiseState(),
	}
	return run, nil
}

// openSession recomputes the session key from the binding, refuses a resume
// that no open scan session matches, naming what differs, ends the older scan
// sessions of a new scan, advances the session's wave and journals its binding
// once.
func (s *Service) openSession(bound binding, resume bool) (*hostbridge.Bridge, error) {
	store := s.deps.Host.Store
	canonical, err := bound.canonical()
	if err != nil {
		return nil, err
	}
	session := hostproto.ScanSession(sessionKeyOf(canonical))
	if resume {
		if err := checkResume(store, session, bound); err != nil {
			return nil, err
		}
	}
	wave, err := store.AdvanceWave(session)
	if err != nil {
		return nil, fmt.Errorf("advance the wave of %s: %w", session, err)
	}
	bridge := hostbridge.New(store, s.deps.Clock, s.deps.Host.Entropy, session, wave)
	bridge.EmissionCap = promise.Budget
	if !resume {
		if err := bridge.EndOlderScans(); err != nil {
			return nil, fmt.Errorf("end the older scan sessions: %w", err)
		}
	}
	if err := journalBinding(store, session, canonical); err != nil {
		return nil, err
	}
	return bridge, nil
}

// finish is the end of the stages of one invocation: the report when the scan
// finished.
type finish struct {
	done   bool
	report Report
}

// stages runs the stages in order and stops at the first that waits for the
// host.
func (r *hostRun) stages(ctx context.Context, started time.Time) (finish, error) {
	for _, stage := range []func(context.Context) (bool, error){r.extract, r.write, r.admit} {
		settled, err := stage(ctx)
		if err != nil || !settled {
			return finish{}, err
		}
	}
	return r.final(ctx, started)
}

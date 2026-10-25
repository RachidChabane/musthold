package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/render"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/transcript"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

// Errors of a recording that cannot be rendered, or of a rendered transcript
// that no longer matches its recording.
var (
	ErrNoRecording = errors.New("the directory holds no recording")
	ErrStale       = errors.New("the rendered transcript differs from what its recording renders")
)

// Renderer turns a recording into its transcript. It reads files and starts no
// process, so a committed recording renders the same everywhere.
type Renderer struct {
	Files canary.Files
}

// Load reads a recording into the transcript's input.
func (r Renderer) Load(dir string) (transcript.Input, error) {
	var in transcript.Input
	meta, err := r.Files.Read(filepath.Join(dir, record.MetaFile))
	if err != nil {
		return in, err
	}
	if meta == nil {
		return in, fmt.Errorf("%w: %s has no %s", ErrNoRecording, dir, record.MetaFile)
	}
	if err := json.Unmarshal(meta, &in.Meta); err != nil {
		return in, fmt.Errorf("%s: %w", record.MetaFile, err)
	}
	stream, err := r.Files.Read(filepath.Join(dir, record.StreamFile))
	if err != nil {
		return in, err
	}
	if in.Stream, err = evidence.Lines(record.StreamFile, stream); err != nil {
		return in, err
	}
	times, err := r.Files.Read(filepath.Join(dir, record.TimesFile))
	if err != nil {
		return in, err
	}
	if in.Arrivals, err = record.DecodeArrivals(times); err != nil {
		return in, err
	}
	hooks, err := r.Files.Read(filepath.Join(dir, record.HooksFile))
	if err != nil {
		return in, err
	}
	if in.Hooks, err = evidence.DecodeHooks(hooks); err != nil {
		return in, err
	}
	report, err := r.Files.Read(filepath.Join(dir, record.FinalGateFile))
	if err != nil {
		return in, err
	}
	if in.FinalGate, err = record.DecodeGateReport(report); err != nil {
		return in, fmt.Errorf("%s: %w", record.FinalGateFile, err)
	}
	change, err := r.Files.Read(filepath.Join(dir, record.ChangeFile))
	if err != nil {
		return in, err
	}
	in.Change = string(change)
	in.Counterexamples, err = r.counterexamples(dir)
	return in, err
}

func (r Renderer) counterexamples(dir string) (map[string]string, error) {
	paths, err := r.Files.Glob(filepath.Join(dir, record.CounterexamplesDir, "*"+record.DiffExtension))
	if err != nil {
		return nil, err
	}
	found := map[string]string{}
	for _, path := range paths {
		content, err := r.Files.Read(path)
		if err != nil {
			return nil, err
		}
		found[strings.TrimSuffix(filepath.Base(path), record.DiffExtension)] = string(content)
	}
	return found, nil
}

// Rendered is a transcript in both forms.
type Rendered struct {
	Markdown, HTML []byte
}

// Render renders a recording.
func (r Renderer) Render(dir string) (Rendered, error) {
	in, err := r.Load(dir)
	if err != nil {
		return Rendered{}, err
	}
	built := transcript.Build(in)
	return Rendered{Markdown: []byte(render.Markdown(built)), HTML: []byte(render.HTML(built))}, nil
}

// Write renders a recording into the output directory, or with check set
// compares what it renders with what the directory holds and writes nothing.
func (r Renderer) Write(dir, out string, check bool) error {
	rendered, err := r.Render(dir)
	if err != nil {
		return err
	}
	for name, content := range map[string][]byte{record.TranscriptMarkdown: rendered.Markdown, record.TranscriptHTML: rendered.HTML} {
		path := filepath.Join(out, name)
		if !check {
			if err := r.Files.Write(path, content, false); err != nil {
				return err
			}
			continue
		}
		current, err := r.Files.Read(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, content) {
			return fmt.Errorf("%w: %s", ErrStale, path)
		}
	}
	return nil
}

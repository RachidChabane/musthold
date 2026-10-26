package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/ledger/internal/app"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const verifiesFetchTrust = " [verifies SP-226~1]"

type fixedTrustSource struct {
	files map[string][]byte
	err   error
}

func (f fixedTrustSource) Fetch(context.Context) (map[string][]byte, error) { return f.files, f.err }

type recordingFiles struct {
	prepared bool
	written  []string
}

func (r *recordingFiles) Prepare() error { r.prepared = true; return nil }

func (r *recordingFiles) WriteFile(path string, _ []byte) error {
	r.written = append(r.written, path)
	return nil
}

func TestFetchTrustService(t *testing.T) {
	t.Run("the files of the snapshot are written in path order"+verifiesFetchTrust, func(t *testing.T) {
		sink := &recordingFiles{}
		service := app.FetchTrustService{
			Source: fixedTrustSource{files: map[string][]byte{"timestamp.json": {1}, "2.snapshot.json": {2}, "targets/x.trusted_root.json": {3}}},
			Sink:   sink,
		}
		count, err := service.Run(context.Background())
		if err != nil || count != 3 || !sink.prepared {
			t.Fatalf("count %d prepared %v: %v", count, sink.prepared, err)
		}
		if len(sink.written) != 3 || sink.written[0] != "2.snapshot.json" || sink.written[2] != "timestamp.json" {
			t.Fatalf("written %v", sink.written)
		}
	})

	t.Run("a set without a timestamp file is refused and nothing is written"+verifiesFetchTrust, func(t *testing.T) {
		sink := &recordingFiles{}
		service := app.FetchTrustService{Source: fixedTrustSource{files: map[string][]byte{"1.snapshot.json": {1}}}, Sink: sink}
		if _, err := service.Run(context.Background()); !errors.Is(err, refusal.ErrRefused) || sink.prepared || len(sink.written) != 0 {
			t.Fatalf("error %v prepared %v written %v", err, sink.prepared, sink.written)
		}
	})

	t.Run("a source that fails writes nothing"+verifiesFetchTrust, func(t *testing.T) {
		sink := &recordingFiles{}
		service := app.FetchTrustService{Source: fixedTrustSource{err: errors.New("unreachable")}, Sink: sink}
		if _, err := service.Run(context.Background()); err == nil || sink.prepared {
			t.Fatalf("error %v prepared %v", err, sink.prepared)
		}
	})
}

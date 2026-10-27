package facts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ErrMissing refuses a counted run whose records lack a file the facts need.
var ErrMissing = errors.New("a record the analysis needs is missing")

// reader reads the files of a records directory and keeps the SHA-256 of each
// file it read, by its path relative to the directory.
type reader struct {
	dir     string
	digests map[string]string
}

func (r reader) read(name string, required bool) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(r.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		if required {
			return nil, fmt.Errorf("%w: %s", ErrMissing, filepath.Join(r.dir, name))
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	r.digests[filepath.ToSlash(name)] = hex.EncodeToString(digest[:])
	return data, nil
}

func (r reader) decode(name string, target any) error {
	data, err := r.read(name, true)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", filepath.Join(r.dir, name), err)
	}
	return nil
}

// Load reads the records of a counted attempt: run.json, the stream and its
// times, the hook log, the stop reports, gate-after.json, and the oracle's,
// held-out and detector files. It returns the decoded input and the SHA-256 of
// every file it read.
func Load(records, project string) (Input, map[string]string, error) {
	r := reader{dir: records, digests: map[string]string{}}
	in := Input{Project: project, Stops: map[int]*Report{}}
	if err := r.decode(record.MetaFile, &in.Run); err != nil {
		return in, nil, err
	}
	for _, step := range []func() error{
		func() error { return r.lines(record.StreamFile, &in.Stream) },
		func() error {
			data, err := r.read(record.TimesFile, false)
			if err == nil {
				in.Arrivals, err = record.DecodeArrivals(data)
			}
			return err
		},
		func() error {
			data, err := r.read(record.HooksFile, false)
			if err == nil {
				in.Hooks, err = evidence.DecodeHooks(data)
			}
			return err
		},
		func() error {
			data, err := r.read(record.FinalGateFile, false)
			if err == nil {
				in.FinalGate, err = DecodeReport(data)
			}
			return err
		},
		func() error { return r.decode(study.OracleFile, &in.Oracle) },
		func() error { return r.decode(study.HeldOutFile, &in.HeldOut) },
		func() error { return r.decode(study.DetectorsFile, &in.Detection) },
		func() error { return r.stops(in) },
	} {
		if err := step(); err != nil {
			return in, nil, err
		}
	}
	return in, r.digests, nil
}

func (r reader) lines(name string, target *[]evidence.Line) error {
	data, err := r.read(name, false)
	if err != nil {
		return err
	}
	*target, err = evidence.Lines(name, data)
	return err
}

// stops reads stops/<n>/report.json for every numbered stop directory.
func (r reader) stops(in Input) error {
	entries, err := os.ReadDir(filepath.Join(r.dir, study.StopsDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		n, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		name := filepath.Join(study.StopsDir, entry.Name(), study.StopReportFile)
		data, err := r.read(name, false)
		if err != nil {
			return err
		}
		report, err := DecodeReport(data)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Join(r.dir, name), err)
		}
		if report != nil {
			in.Stops[n] = report
		}
	}
	return nil
}

package cli

import (
	"fmt"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/bundlefs"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/prove"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostsync"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// issuedDirectory is where a CI run leaves the requests it issues, relative to
// the project, for the job to upload as an artifact.
const issuedDirectory = stateDirectory + "/ci-issued"

// ciRun is the CI part of a `prove` invocation.
type ciRun struct {
	enabled   bool
	importDir string
}

// prepareCI adjusts a run for CI and returns what to do once it ended: produce
// the Statements as CI, serve the tickets from the pushed bundle, refuse a bundle
// of another version when it left requests unanswered, and leave the requests
// that remain for the host.
func (e Env) prepareCI(project *project, ci ciRun) (func(prove.Result) error, error) {
	if !ci.enabled {
		return func(prove.Result) error { return nil }, nil
	}
	project.settings.ProducedBy = format.ProducedByCI
	importer, err := e.importer(project, ci.importDir)
	if err != nil {
		return nil, err
	}
	if importer != nil {
		project.deps.Replay = importer
	}
	return func(result prove.Result) error {
		if importer != nil {
			if err := hostbundle.CheckSkew(importer.Bundle.Manifest.Module, project.settings.Module, importer.Unanswered); err != nil {
				return refusal{err}
			}
		}
		if result.PendingTickets == 0 {
			return nil
		}
		return e.writeIssued(project)
	}, nil
}

// importer reads the bundle a push published, when one is given.
func (e Env) importer(project *project, dir string) (*hostsync.Importer, error) {
	if dir == "" {
		return nil, nil
	}
	files, err := bundlefs.Read(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := hostbundle.Decode(files)
	if err != nil {
		return nil, refusal{err}
	}
	service := requests.Service{Plugin: pluginName, Store: project.store, Validator: responsecheck.New(), Clock: e.Now}
	return &hostsync.Importer{Service: service, Store: project.store, Bundle: bundle}, nil
}

// writeIssued leaves the requests that still wait for an answer where the CI job
// uploads them from.
func (e Env) writeIssued(project *project) error {
	issued, err := hostsync.ExportIssued(project.store)
	if err != nil {
		return err
	}
	files, err := issued.Files()
	if err != nil {
		return err
	}
	if err := bundlefs.Write(filepath.Join(project.root, filepath.FromSlash(issuedDirectory)), files); err != nil {
		return fmt.Errorf("write the issued requests: %w", err)
	}
	return nil
}

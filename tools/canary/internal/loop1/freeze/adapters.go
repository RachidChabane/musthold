package freeze

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ErrUnreadable is returned for tool output or a record file of an unexpected shape.
var ErrUnreadable = errors.New("unreadable output")

var versionNumber = regexp.MustCompile(`[0-9]+(\.[0-9]+)+`)

// VersionOf is the first dotted number of a tool's version line, as the inputs
// freeze records a version.
func VersionOf(line string) (string, error) {
	version := versionNumber.FindString(line)
	if version == "" {
		return "", fmt.Errorf("%w: no version in %q", ErrUnreadable, line)
	}
	return version, nil
}

// CLIEngine reads the engine through the docker client of the study and the
// colima command.
type CLIEngine struct {
	Exec Exec
	// Docker is the docker client command, which names the study's engine.
	Docker []string
	// Colima is the colima command; empty means "colima".
	Colima string
}

var _ Engine = CLIEngine{}

func (e CLIEngine) docker(ctx context.Context, args ...string) (string, error) {
	argv := append(append([]string{}, e.Docker...), args...)
	output, err := e.Exec.Output(ctx, "", argv[0], argv[1:]...)
	return strings.TrimSpace(string(output)), err
}

func (e CLIEngine) colima(ctx context.Context, args ...string) ([]byte, error) {
	name := e.Colima
	if name == "" {
		name = "colima"
	}
	return e.Exec.Output(ctx, "", name, args...)
}

// DockerVersion implements Engine.
func (e CLIEngine) DockerVersion(ctx context.Context) (string, error) {
	return e.docker(ctx, "version", "--format", "{{.Server.Version}}")
}

// ColimaVersion implements Engine.
func (e CLIEngine) ColimaVersion(ctx context.Context) (string, error) {
	output, err := e.colima(ctx, "version")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(string(output), "\n")
	return VersionOf(first)
}

// ImageID implements Engine.
func (e CLIEngine) ImageID(ctx context.Context, image string) (string, error) {
	return e.docker(ctx, "image", "inspect", "--format", "{{.Id}}", image)
}

type colimaListing struct {
	Name   string      `json:"name"`
	CPUs   json.Number `json:"cpus"`
	Memory json.Number `json:"memory"`
	Disk   json.Number `json:"disk"`
}

// Settings implements Engine. `colima list --json` prints one JSON document per
// profile, one per line.
func (e CLIEngine) Settings(ctx context.Context, profile string) (ColimaSettings, error) {
	output, err := e.colima(ctx, "list", "--json")
	if err != nil {
		return ColimaSettings{}, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var listing colimaListing
		if json.Unmarshal(scanner.Bytes(), &listing) != nil || listing.Name != profile {
			continue
		}
		return ColimaSettings{CPUs: listing.CPUs.String(), MemoryBytes: listing.Memory.String(), DiskBytes: listing.Disk.String()}, nil
	}
	return ColimaSettings{}, fmt.Errorf("%w: colima has no profile %q", ErrUnreadable, profile)
}

// UnameHost reads the host with `uname -srm`.
type UnameHost struct{ Exec Exec }

var _ Host = UnameHost{}

const unameFields = 3

// Info implements Host.
func (h UnameHost) Info(ctx context.Context) (HostInfo, error) {
	output, err := h.Exec.Output(ctx, "", "uname", "-srm")
	if err != nil {
		return HostInfo{}, err
	}
	fields := strings.Fields(string(output))
	if len(fields) != unameFields {
		return HostInfo{}, fmt.Errorf("%w: uname printed %q", ErrUnreadable, output)
	}
	return HostInfo{OS: fields[0], Version: fields[1], Arch: fields[2]}, nil
}

// Status values of the compare endpoint that say a tag has the commit in its
// history.
const (
	compareAhead     = "ahead"
	compareIdentical = "identical"
)

// GitHubReleases reads the public tags of a repository with the gh command.
type GitHubReleases struct {
	Exec Exec
	// Repository is "owner/name".
	Repository string
}

var _ Releases = GitHubReleases{}

// Tags implements Releases.
func (g GitHubReleases) Tags(ctx context.Context) ([]string, error) {
	output, err := g.Exec.Output(ctx, "", "gh", "api", "--paginate", "repos/"+g.Repository+"/tags", "--jq", ".[].name")
	if err != nil {
		return nil, err
	}
	return splitLines(string(output), "\n"), nil
}

// Contains implements Releases.
func (g GitHubReleases) Contains(ctx context.Context, tag, commit string) (bool, error) {
	output, err := g.Exec.Output(ctx, "", "gh", "api", "repos/"+g.Repository+"/compare/"+commit+"..."+tag, "--jq", ".status")
	if err != nil {
		return false, err
	}
	status := strings.TrimSpace(string(output))
	return status == compareAhead || status == compareIdentical, nil
}

// FileConstruction reads the construction records file of the work directory.
type FileConstruction struct{ Layout study.Layout }

var _ ConstructionSource = FileConstruction{}

// Records implements ConstructionSource.
func (f FileConstruction) Records(context.Context) (records.Log, error) {
	return records.Read(records.Path(f.Layout))
}

// FileRuns reads the starts of the runs from the run directories of the work
// directory.
type FileRuns struct{ Layout study.Layout }

var _ Runs = FileRuns{}

// Starts implements Runs.
func (f FileRuns) Starts(context.Context) ([]RunStart, error) {
	files, err := filepath.Glob(filepath.Join(f.Layout.Root, study.RunsDir, "*", study.RecordsDir, record.MetaFile))
	if err != nil {
		return nil, err
	}
	starts := make([]RunStart, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var run study.Run
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrUnreadable, file, err)
		}
		starts = append(starts, RunStart{Name: filepath.Base(filepath.Dir(filepath.Dir(file))), Start: run.StartedAt})
	}
	return starts, nil
}

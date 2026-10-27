package freeze

import (
	"context"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ColimaSettings are the virtual-machine settings of a colima profile.
type ColimaSettings struct {
	CPUs        string
	MemoryBytes string
	DiskBytes   string
}

// Engine reads the container engine of the study.
type Engine interface {
	// DockerVersion is the version of the engine's server.
	DockerVersion(ctx context.Context) (string, error)
	// ColimaVersion is the version of the colima command.
	ColimaVersion(ctx context.Context) (string, error)
	// Settings are the settings of the named colima profile.
	Settings(ctx context.Context, profile string) (ColimaSettings, error)
	// ImageID is the id of an image the engine holds.
	ImageID(ctx context.Context, image string) (string, error)
}

// HostInfo names the machine that runs the host's side of the study.
type HostInfo struct {
	OS      string
	Version string
	Arch    string
}

// Host reads the machine the verb runs on.
type Host interface {
	Info(ctx context.Context) (HostInfo, error)
}

// Releases reads the public releases of Shallnot.
type Releases interface {
	// Tags lists the release tags.
	Tags(ctx context.Context) ([]string, error)
	// Contains reports whether the tag's commit has the given commit in its history.
	Contains(ctx context.Context, tag, commit string) (bool, error)
}

// CanarySource reads the output of the canary's last invocation (PREREG §13.3).
type CanarySource interface {
	Output(ctx context.Context) (study.CanaryOutput, error)
}

// ConstructionSource reads the construction records.
type ConstructionSource interface {
	Records(ctx context.Context) (records.Log, error)
}

// Prepared is what the preparation of one task for one arm produced: the start
// commit and the equipped commit of PREREG §3.5.
type Prepared struct {
	// BaseCommit is the hash of the commit of the copied fixture.
	BaseCommit string
	// EquippedTree is the digest of the normalised equipped tree.
	EquippedTree string
	// Files maps every file the equipped commit adds or changes relative to the
	// base commit to its SHA-256.
	Files map[string]string
	// SessionSettings is the SHA-256 of session-settings.json.
	SessionSettings string
}

// Preparer prepares a task for an arm, in the study's image, as a run does.
type Preparer interface {
	Prepare(ctx context.Context, task config.Task, arm config.Arm) (Prepared, error)
}

// Runs lists the starts of the runs the work directory holds.
type Runs interface {
	Starts(ctx context.Context) ([]RunStart, error)
}

// RunStart is the start of one attempt of a run.
type RunStart struct {
	Name  string
	Start time.Time
}

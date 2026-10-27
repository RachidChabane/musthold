// Package freeze implements the two verbs that close the study's inputs before
// its first run: resolve, which records every input in freeze/inputs.json and its
// sums (PREREG §13.4), and freeze-check, which refuses unless the tags, the
// timestamps, the sums, the pins and the digests of the study agree (PREREG
// §13.5, with the amendment tag).
package freeze

import (
	"path"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The tags of the study's freezes.
const (
	PreregTag    = "prereg-loop-1"
	AmendmentTag = "prereg-loop-1-a1"
	InputsTag    = "loop-1-inputs"
)

// Locations in the repository, slash-separated from its root, beside those of
// the study package.
const (
	ImageDir          = study.StudyDir + "/image"
	PreregSumsFile    = study.FreezeDir + "/SHA256SUMS"
	InputsSumsFile    = study.FreezeDir + "/inputs-SHA256SUMS"
	ShallnotPinFile   = "script/shallnot.pin"
	ClaudeCodePinFile = "script/claude-code.pin"
	commitFileSuffix  = "-commit.txt"
)

// Freeze is one tag with the files that timestamp it.
type Freeze struct {
	Tag string
	// Stem names the freeze's files: freeze/<Stem>-commit.{txt,tsq,tsr}.
	Stem string
}

// CommitFile is the repository path of the file that holds the tagged commit.
func (f Freeze) CommitFile() string { return path.Join(study.FreezeDir, f.Stem+commitFileSuffix) }

// The freezes, in the order of their tags.
var (
	Prereg    = Freeze{Tag: PreregTag, Stem: "prereg"}
	Amendment = Freeze{Tag: AmendmentTag, Stem: "a1"}
	Inputs    = Freeze{Tag: InputsTag, Stem: "inputs"}
)

// Freezes lists every freeze.
func Freezes() []Freeze { return []Freeze{Prereg, Amendment, Inputs} }

// PreregRoots are the paths whose files the pre-registration's sums list
// (PREREG §13.1), and PreregExcluded the prefix they leave out.
var PreregRoots = []string{
	study.StudyDir,
	"demo/loop/fixture/boxoffice",
	"tools/canary/internal/demoloop/fixturegate/testdata/task",
}

// PreregExcluded is the directory the pre-registration's sums do not list.
const PreregExcluded = study.FreezeDir + "/"

// ProductDirs are the directories whose commits between the two tags enter
// commits.between (PREREG §2).
var ProductDirs = []string{"discpower", "alignment", "completeness", "format", "umbrella", "verify", "tools/canary"}

// The pins of PREREG §2, with the Shallnot pin of the amendment.
const (
	ExpectedShallnotPin   = "c331f9b97d5b96bdc18eba3b17be870c004dc239"
	ExpectedClaudeVersion = "2.1.295"
	ColimaProfile         = "musthold-loop-1"
	ExpectedColimaCPUs    = "2"
	ExpectedColimaMemory  = "4294967296"
	ExpectedColimaDisk    = "32212254720"
)

// ExpectedToolchains are the toolchain versions PREREG §2 fixes.
var ExpectedToolchains = map[string]string{
	ToolchainGo:      "1.26.4",
	ToolchainPython:  "3.13.14",
	ToolchainUV:      "0.11.23",
	ToolchainColimaC: ExpectedColimaCPUs,
	ToolchainColimaM: ExpectedColimaMemory,
	ToolchainColimaD: ExpectedColimaDisk,
}

// The keys of inputs.json's toolchains.
const (
	ToolchainGo       = "go"
	ToolchainPython   = "python"
	ToolchainPytest   = "pytest"
	ToolchainGit      = "git"
	ToolchainNode     = "node"
	ToolchainUV       = "uv"
	ToolchainDocker   = "docker_engine"
	ToolchainColima   = "colima"
	ToolchainColimaC  = "colima_cpus"
	ToolchainColimaM  = "colima_memory_bytes"
	ToolchainColimaD  = "colima_disk_bytes"
	ToolchainHostOS   = "host_os"
	ToolchainHostVer  = "host_os_version"
	ToolchainHostArch = "host_arch"
)

// PluginKeyPrefix precedes a plugin tree's path in inputs.json's binaries.
const PluginKeyPrefix = "plugins/"

// ArmSpecificFile is the one file of the equipped commit that differs between the
// arms (PREREG §5).
const ArmSpecificFile = "shallnot.yaml"

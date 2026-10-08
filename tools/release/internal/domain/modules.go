// Package domain holds the rules of the release pipeline: which tags release which
// module, what a release contains and how it is named, what a pre-flight checks
// and what a finished dist directory must hold. It performs no input or output.
package domain

// Repository identity, the one place the release pipeline names where the
// project lives.
const (
	RepositoryOwner  = "RachidChabane"
	RepositoryName   = "spec-proof"
	RepositorySlug   = RepositoryOwner + "/" + RepositoryName
	RepositoryURL    = "https://github.com/" + RepositorySlug
	ModulePathPrefix = "github.com/" + RepositorySlug + "/"
	FormatModuleName = "format"
	FormatModulePath = ModulePathPrefix + FormatModuleName
	ReleaseWorkflow  = ".github/workflows/release.yml"
	MarketplaceFile  = ".claude-plugin/marketplace.json"
	PluginGenScript  = "script/plugin-gen"
	LicenseFile      = "LICENSE"
	ReadmeFile       = "README.md"
	// NoticeFile is the file at the repository root that names the copyright holder.
	NoticeFile = "NOTICE"
	// NoticeHolderPlaceholder stands in NoticeFile for the copyright holder until the
	// owner names one; a real release refuses a NoticeFile that still holds it.
	NoticeHolderPlaceholder = "[COPYRIGHT HOLDER]"
	// ThirdPartyNoticesFile is the third-party notices every archive ships.
	ThirdPartyNoticesFile = "THIRD_PARTY_NOTICES.md"
	LicenseIdentifier     = "Apache-2.0"
)

// Kind says what a module releases.
type Kind string

// The kinds of the release table.
const (
	// KindLibrary is a Go library: the tag is the release and there are no assets.
	KindLibrary Kind = "library"
	// KindBinaryPlugin is a binary and the plugin trees that drive it.
	KindBinaryPlugin Kind = "binary+plugin"
	// KindBinary is a binary alone.
	KindBinary Kind = "binary"
	// KindPlugin is plugin files alone: the release has notes and no binaries.
	KindPlugin Kind = "plugin"
)

// Module is one row of the release table.
type Module struct {
	// Name is the directory of the module and the prefix of its tags.
	Name string
	Kind Kind
	// Binary is the executable the module ships, and the directory of its main
	// package under cmd. It is empty for a module that ships none.
	Binary string
	// PluginName is the name of the module's plugin in the marketplace. It is empty
	// for a module that ships none.
	PluginName string
	// PluginRoots are the directories, relative to the repository, whose
	// plugin.json files carry the plugin version.
	PluginRoots []string
	// Summary is the one-line description of the shipped binary.
	Summary string
}

// ShipsBinary reports whether the module releases built binaries.
func (m Module) ShipsBinary() bool { return m.Kind == KindBinaryPlugin || m.Kind == KindBinary }

// ShipsPlugin reports whether the module releases plugin trees.
func (m Module) ShipsPlugin() bool { return m.Kind == KindBinaryPlugin || m.Kind == KindPlugin }

// RequiresFormat reports whether the module's go.mod may require the format
// library: every module but the library itself.
func (m Module) RequiresFormat() bool { return m.Name != FormatModuleName }

// MainPackage is the package, relative to the module directory, that builds the
// binary.
func (m Module) MainPackage() string { return "./cmd/" + m.Binary }

// ModulePath is the Go module path of the module.
func (m Module) ModulePath() string { return ModulePathPrefix + m.Name }

// releaseTable is the release table. Any module not in it is never released.
var releaseTable = []Module{
	{Name: FormatModuleName, Kind: KindLibrary},
	{
		Name: "discpower", Kind: KindBinaryPlugin, Binary: "spec-proof-dp", PluginName: "spec-proof-dp",
		PluginRoots: []string{"discpower/plugin"},
		Summary:     "Measures the discriminating power of the tests bound to each requirement",
	},
	{
		Name: "alignment", Kind: KindBinaryPlugin, Binary: "spec-proof-alignment", PluginName: "spec-proof-alignment",
		PluginRoots: []string{"alignment/plugin"},
		Summary:     "Judges whether the tests bound to a requirement verify what it states",
	},
	{
		Name: "completeness", Kind: KindBinaryPlugin, Binary: "spec-proof-completeness", PluginName: "spec-proof-completeness",
		PluginRoots: []string{"completeness/plugin"},
		Summary:     "Advises on behaviour the specification leaves unstated",
	},
	{
		Name: "s1relay", Kind: KindBinaryPlugin, Binary: "spec-proof-s1", PluginName: "spec-proof-s1",
		PluginRoots: []string{"s1relay/plugin"},
		Summary:     "Relays closed judgment questions to a System One endpoint",
	},
	{
		Name: "ledger", Kind: KindBinary, Binary: "spec-proof-ledger",
		Summary: "Signs proofs and keeps them in the append-only proof ledger",
	},
	{
		Name: "verify", Kind: KindBinary, Binary: "spec-proof-verify",
		Summary: "Checks signed proofs and writes the verified set",
	},
	{
		Name: "umbrella", Kind: KindPlugin, PluginName: "spec-proof",
		PluginRoots: []string{"umbrella/plugin", "umbrella/plugins"},
	},
}

// Modules returns the release table in release order: format first, because the
// modules that require it are tagged after it.
func Modules() []Module { return append([]Module(nil), releaseTable...) }

// LookupModule returns the row of the release table for a module name.
func LookupModule(name string) (Module, bool) {
	for _, module := range releaseTable {
		if module.Name == name {
			return module, true
		}
	}
	return Module{}, false
}

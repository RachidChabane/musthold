package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// SPDX document constants.
const (
	spdxVersion       = "SPDX-2.3"
	spdxDataLicense   = "CC0-1.0"
	spdxDocumentID    = "SPDXRef-DOCUMENT"
	spdxMainID        = "SPDXRef-Package-main"
	spdxGoID          = "SPDXRef-Package-go"
	spdxDependencyID  = "SPDXRef-Package-%d"
	spdxUnknown       = "NOASSERTION"
	spdxCreatedLayout = "2006-01-02T15:04:05Z"
	spdxCreatorTool   = "Tool: musthold-release"
	spdxDescribes     = "DESCRIBES"
	spdxDependsOn     = "DEPENDS_ON"
	spdxPackageManger = "PACKAGE-MANAGER"
	spdxPurl          = "purl"
	goStdlibName      = "go"
	goVersionPrefix   = "go"
	jsonIndent        = "  "
)

// Dependency is a module a binary was linked with.
type Dependency struct {
	Path    string
	Version string
	// Sum is the `h1:` hash of the module in go.sum.
	Sum string
}

// BuildSetting is a key and value of the build information of a binary.
type BuildSetting struct{ Key, Value string }

// BuildFacts is what the build information of a Go binary says.
type BuildFacts struct {
	GoVersion    string
	MainPath     string
	MainVersion  string
	Dependencies []Dependency
	Settings     []BuildSetting
}

// SBOMInput is everything the SBOM of one archive is rendered from.
type SBOMInput struct {
	Tag         Tag
	ArchiveName string
	Build       BuildFacts
	// Created is the commit time, so that rendering twice gives the same bytes.
	Created time.Time
}

// SBOMNamespace is the documentNamespace of the SBOM of an archive.
func SBOMNamespace(tag Tag, archiveName string) string {
	return RepositoryURL + "/" + tag.String() + "/" + archiveName
}

type spdxDocument struct {
	Version       string             `json:"spdxVersion"`
	DataLicense   string             `json:"dataLicense"`
	ID            string             `json:"SPDXID"`
	Name          string             `json:"name"`
	Namespace     string             `json:"documentNamespace"`
	CreationInfo  spdxCreationInfo   `json:"creationInfo"`
	Packages      []spdxPackage      `json:"packages"`
	Relationships []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name             string            `json:"name"`
	ID               string            `json:"SPDXID"`
	VersionInfo      string            `json:"versionInfo,omitempty"`
	DownloadLocation string            `json:"downloadLocation"`
	FilesAnalyzed    bool              `json:"filesAnalyzed"`
	LicenseConcluded string            `json:"licenseConcluded"`
	LicenseDeclared  string            `json:"licenseDeclared"`
	CopyrightText    string            `json:"copyrightText"`
	Comment          string            `json:"comment,omitempty"`
	ExternalRefs     []spdxExternalRef `json:"externalRefs,omitempty"`
}

type spdxExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

type spdxRelationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}

// SPDXHeader is the part of an SBOM that a finished dist directory is checked
// against.
type SPDXHeader struct {
	Version   string
	Name      string
	Namespace string
	Created   string
}

// ParseSPDXHeader reads the identifying fields of a rendered SBOM.
func ParseSPDXHeader(data []byte) (SPDXHeader, error) {
	var document spdxDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return SPDXHeader{}, fmt.Errorf("the SBOM is not JSON: %w", err)
	}
	return SPDXHeader{
		Version: document.Version, Name: document.Name,
		Namespace: document.Namespace, Created: document.CreationInfo.Created,
	}, nil
}

// SPDXCreated spells an instant the way an SBOM does.
func SPDXCreated(instant time.Time) string { return instant.UTC().Format(spdxCreatedLayout) }

// RenderSPDX renders the SPDX 2.3 JSON document of one archive: the main module,
// the Go standard library and every module the binary links, in a fixed order.
func RenderSPDX(input SBOMInput) ([]byte, error) {
	dependencies := slices.Clone(input.Build.Dependencies)
	slices.SortFunc(dependencies, func(a, b Dependency) int {
		return strings.Compare(a.Path+"@"+a.Version, b.Path+"@"+b.Version)
	})
	settings := slices.Clone(input.Build.Settings)
	slices.SortFunc(settings, func(a, b BuildSetting) int { return strings.Compare(a.Key, b.Key) })

	main := spdxPackage{
		Name: input.Build.MainPath, ID: spdxMainID, VersionInfo: input.Tag.Semver(),
		DownloadLocation: "git+" + RepositoryURL + "@" + input.Tag.String(),
		LicenseConcluded: LicenseIdentifier, LicenseDeclared: LicenseIdentifier, CopyrightText: spdxUnknown,
		Comment: buildSettingsComment(input.Build.GoVersion, settings),
	}
	goVersion := strings.TrimPrefix(input.Build.GoVersion, goVersionPrefix)
	packages := []spdxPackage{main, {
		Name: goStdlibName, ID: spdxGoID, VersionInfo: goVersion, DownloadLocation: spdxUnknown,
		LicenseConcluded: spdxUnknown, LicenseDeclared: spdxUnknown, CopyrightText: spdxUnknown,
		ExternalRefs: []spdxExternalRef{purlOf("golang/stdlib", goVersion)},
	}}
	relationships := []spdxRelationship{
		{Element: spdxDocumentID, Type: spdxDescribes, Related: spdxMainID},
		{Element: spdxMainID, Type: spdxDependsOn, Related: spdxGoID},
	}
	for index, dependency := range dependencies {
		identifier := fmt.Sprintf(spdxDependencyID, index+1)
		packages = append(packages, spdxPackage{
			Name: dependency.Path, ID: identifier, VersionInfo: dependency.Version, DownloadLocation: spdxUnknown,
			LicenseConcluded: spdxUnknown, LicenseDeclared: spdxUnknown, CopyrightText: spdxUnknown,
			Comment:      goSumComment(dependency.Sum),
			ExternalRefs: []spdxExternalRef{purlOf("golang/"+strings.ToLower(dependency.Path), dependency.Version)},
		})
		relationships = append(relationships, spdxRelationship{Element: spdxMainID, Type: spdxDependsOn, Related: identifier})
	}
	document := spdxDocument{
		Version: spdxVersion, DataLicense: spdxDataLicense, ID: spdxDocumentID,
		Name: input.ArchiveName, Namespace: SBOMNamespace(input.Tag, input.ArchiveName),
		CreationInfo:  spdxCreationInfo{Created: SPDXCreated(input.Created), Creators: []string{spdxCreatorTool}},
		Packages:      packages,
		Relationships: relationships,
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", jsonIndent)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("render the SBOM of %s: %w", input.ArchiveName, err)
	}
	return out.Bytes(), nil
}

func purlOf(name, version string) spdxExternalRef {
	locator := "pkg:" + name
	if version != "" {
		locator += "@" + version
	}
	return spdxExternalRef{Category: spdxPackageManger, Type: spdxPurl, Locator: locator}
}

func goSumComment(sum string) string {
	if sum == "" {
		return ""
	}
	return "go.sum " + sum
}

func buildSettingsComment(goVersion string, settings []BuildSetting) string {
	parts := []string{"built with " + goVersion}
	for _, setting := range settings {
		parts = append(parts, setting.Key+"="+setting.Value)
	}
	return strings.Join(parts, "; ")
}

package domain

import (
	"fmt"
	"strings"
)

// Commit is one change listed in the release notes.
type Commit struct {
	// Hash is the abbreviated commit hash.
	Hash    string
	Subject string
}

// NotesInput is everything the release notes are rendered from.
type NotesInput struct {
	Tag Tag
	// Previous is the tag the changes are counted from, empty for the first release.
	Previous string
	Commits  []Commit
}

// RenderNotes renders the release notes of a tag: the changes since the previous
// tag, and how to install and verify what the release ships.
func RenderNotes(input NotesInput) string {
	tag, module := input.Tag, input.Tag.Module
	var out strings.Builder
	fmt.Fprintf(&out, "## %s %s\n\n", module.Name, tag.Semver())
	out.WriteString("### Changes\n\n")
	if len(input.Commits) == 0 {
		out.WriteString("No change to the module or to `format` since the previous release.\n")
	}
	for _, commit := range input.Commits {
		fmt.Fprintf(&out, "- %s (%s)\n", commit.Subject, commit.Hash)
	}
	if input.Previous != "" {
		fmt.Fprintf(&out, "\nSince `%s`.\n", input.Previous)
	}
	out.WriteString("\n### Install\n\n")
	switch {
	case module.ShipsBinary():
		fmt.Fprintf(&out, "```\ngo install %s/cmd/%s@v%s\n```\n\n", module.ModulePath(), module.Binary, tag.Semver())
		fmt.Fprintf(&out, "Or download `install` from this release and run `bash install %s %s`: it checks the signature, the checksum and the version before it installs.\n", module.Name, tag.Semver())
		fmt.Fprintf(&out, "\n### Verify\n\nDownload `%s`, `%s` and an archive, then:\n\n", ChecksumsFileName, SignatureBundleName)
		fmt.Fprintf(&out, "```\ncosign verify-blob --bundle %s \\\n  --certificate-identity %s \\\n  --certificate-oidc-issuer %s \\\n  %s\nsha256sum --ignore-missing -c %s\ngh attestation verify <archive> --repo %s\n```\n",
			SignatureBundleName, CertificateIdentity(tag), OIDCIssuer, ChecksumsFileName, ChecksumsFileName, RepositorySlug)
	case module.Kind == KindLibrary:
		fmt.Fprintf(&out, "```\ngo get %s@v%s\n```\n", module.ModulePath(), tag.Semver())
	default:
		fmt.Fprintf(&out, "The plugin files are pinned to the tag `%s` in the marketplace of this repository.\n", tag)
	}
	return out.String()
}

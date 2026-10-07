package format

import (
	"strconv"
	"strings"
)

// Input descriptors, one definition per role (ADR 0008). Every module builds its
// inputs through these constructors, and the invariants of this package check
// the shapes they produce.

// SpecInput describes the statement of the criterion a proof is about: named
// `ID~REV`, with the statement digest the subject carries.
func SpecInput(requirementID string, revision int, statementDigest string) Input {
	return Input{Role: RoleSpec, Name: SubjectName(requirementID, revision), Digest: Digest{SHA256: statementDigest}}
}

// FileInput describes one file of a `code_tree` or `test_source` input: the path
// relative to the repository root with `/` separators, and the SHA-256 of the
// file's exact bytes.
func FileInput(role InputRole, path, contentDigest string) Input {
	return Input{Role: role, Name: path, Digest: Digest{SHA256: contentDigest}}
}

// ProofInput describes a consumed Statement: the name is the consumed kind, the
// criterion it is about travels in the annotations, and the digest is the SHA-256
// of the Statement file's exact bytes, which the verified set's manifest lists.
func ProofInput(kind Kind, criterion string, statementFile []byte) Input {
	return Input{
		Role: RoleProof, Name: string(kind), Digest: Digest{SHA256: SHA256Hex(statementFile)},
		Annotations: &Annotations{Criterion: criterion},
	}
}

// IsFileRole reports whether inputs of the role are per-file descriptors.
func (r InputRole) IsFileRole() bool { return r == RoleCodeTree || r == RoleTestSource }

// ParseCriterion splits a criterion `ID~REV`.
func ParseCriterion(text string) (id string, revision int, ok bool) {
	id, revisionText, found := strings.Cut(text, RevisionSeparator)
	if !found || id == "" {
		return "", 0, false
	}
	revision, err := strconv.Atoi(revisionText)
	if err != nil || revision < MinimumRevision || strconv.Itoa(revision) != revisionText {
		return "", 0, false
	}
	return id, revision, true
}

package format

import (
	"slices"
	"strings"
)

// unitKeySeparator joins the parts of an edit's unit key.
const unitKeySeparator = "\x00"

// definitionTarget is the target kind an omitted target stands for; it orders
// edits the way a spelled-out target would.
const definitionTarget = "definition"

// scanEditSet is the wire form a mutant id is the digest of.
type scanEditSet struct {
	Edits []ScanEdit `json:"edits"`
}

// CanonicalScanEdits orders edits by file, target kind and qualname, an omitted
// target ordering as a definition: the order a mutant's edits are written in.
func CanonicalScanEdits(edits []ScanEdit) []ScanEdit {
	ordered := slices.Clone(edits)
	slices.SortFunc(ordered, compareScanEdits)
	return ordered
}

func compareScanEdits(a, b ScanEdit) int {
	return strings.Compare(scanEditKey(a), scanEditKey(b))
}

func scanEditKey(edit ScanEdit) string {
	target := edit.Target
	if target == "" {
		target = definitionTarget
	}
	return strings.Join([]string{edit.File, target, edit.Qualname}, unitKeySeparator)
}

// ScanEditsDigest is the mutant id of a set of edits: the SHA-256 of the RFC 8785
// form of `{"edits": [...]}` with the edits in canonical order and the target of
// a definition omitted.
func ScanEditsDigest(edits []ScanEdit) (string, error) {
	canonical, err := Canonicalize(scanEditSet{Edits: CanonicalScanEdits(edits)})
	if err != nil {
		return "", err
	}
	return SHA256Hex(canonical), nil
}

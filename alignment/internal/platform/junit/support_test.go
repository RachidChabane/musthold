package junit_test

import "strings"

// marker is the word that opens a citation; the test files write it as `cites` so
// that none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

// tagged spells the citations of a test source or a name written with `cites`.
func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

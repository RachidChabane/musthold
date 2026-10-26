// Package digestmanifest checks the statements of a CI run against the digest
// manifest the generate job wrote. The manifest is in the format of sha256sum:
// one `<hex digest><two spaces><path>` line per statement, the path relative to the
// statement directory.
package digestmanifest

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/app"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const lineSeparator = "  "

// File is the digest manifest at a path.
type File struct {
	Path string
}

// Check requires the statements and the manifest to list the same files with the
// same digests: a missing, extra or altered statement refuses the run.
func (f File) Check(statements []app.Statement) error {
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fmt.Errorf("digest manifest: %w", err)
	}
	expected, err := parse(raw)
	if err != nil {
		return err
	}
	var problems []string
	listed := map[string]bool{}
	for _, statement := range statements {
		path := statement.Location.StatementPath()
		listed[path] = true
		digest, found := expected[path]
		switch {
		case !found:
			problems = append(problems, path+" is not in the manifest")
		case digest != format.SHA256Hex(statement.Bytes):
			problems = append(problems, path+" does not match its manifest digest")
		}
	}
	for path := range expected {
		if !listed[path] {
			problems = append(problems, path+" is in the manifest but not in the run")
		}
	}
	slices.Sort(problems)
	if len(problems) > 0 {
		return refusal.Newf("digest manifest: %s", strings.Join(problems, "; "))
	}
	return nil
}

// parse reads the lines of a manifest into a map from path to digest.
func parse(raw []byte) (map[string]string, error) {
	digests := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		digest, path, found := strings.Cut(line, lineSeparator)
		if !found || !format.IsSHA256Hex(digest) || path == "" {
			return nil, refusal.Newf("digest manifest: %q is not <sha256>%s<path>", line, lineSeparator)
		}
		if _, duplicate := digests[path]; duplicate {
			return nil, refusal.Newf("digest manifest: %s is listed twice", path)
		}
		digests[path] = digest
	}
	return digests, scanner.Err()
}

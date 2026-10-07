package format

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// pathSeparator separates the segments of a relative POSIX path.
const pathSeparator = "/"

// fieldTerminator ends the path and the content of one file in the file-set
// digest stream.
const fieldTerminator = 0x00

// File is one file of a file set: a relative POSIX path and its content.
type File struct {
	Path    string
	Content []byte
}

// IsRelativePOSIXPath reports whether p is a non-empty relative path with "/"
// separators and no empty, "." or ".." segment.
func IsRelativePOSIXPath(p string) bool {
	if p == "" || strings.ContainsAny(p, "\\\x00") {
		return false
	}
	for _, segment := range strings.Split(p, pathSeparator) {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// FileSetDigest returns the SHA-256 over the files sorted by path, each written as
// `path NUL content NUL`. The order of files in the argument does not matter.
// A path that is not a relative POSIX path, or that occurs twice, is an error.
func FileSetDigest(files []File) (string, error) {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b File) int { return strings.Compare(a.Path, b.Path) })

	hash := sha256.New()
	for index, file := range sorted {
		if !IsRelativePOSIXPath(file.Path) {
			return "", fmt.Errorf("file set: %q is not a relative POSIX path", file.Path)
		}
		if index > 0 && sorted[index-1].Path == file.Path {
			return "", fmt.Errorf("file set: path %q occurs twice", file.Path)
		}
		hash.Write([]byte(file.Path))
		hash.Write([]byte{fieldTerminator})
		hash.Write(file.Content)
		hash.Write([]byte{fieldTerminator})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ResultDigest returns the digest recorded for a `junit_result` input: the SHA-256
// of the JCS of the canonical map from test identifier to outcome. Raw XML, which
// carries timings, never enters the digest.
func ResultDigest(outcomeByTest map[string]string) (string, error) {
	canonical, err := Canonicalize(outcomeByTest)
	if err != nil {
		return "", err
	}
	return SHA256Hex(canonical), nil
}

// keyedInput is an input as the cache key sees it: the members every reader of
// the format knows. A member a reader does not know is ignored on reading, so it
// never reaches the key, or a reader would recompute another key than the writer.
type keyedInput struct {
	Role        InputRole        `json:"role"`
	Name        string           `json:"name"`
	Digest      Digest           `json:"digest"`
	Annotations *keyedAnnotation `json:"annotations,omitempty"`
}

// keyedAnnotation is the part of the annotations the cache key covers: the globs a
// file-set input was expanded from. The criterion a `proof` input is about is
// informational, and the name and digest already identify what was consumed.
type keyedAnnotation struct {
	Globs []string `json:"globs,omitempty"`
}

func keyedInputOf(input Input) keyedInput {
	keyed := keyedInput{Role: input.Role, Name: input.Name, Digest: input.Digest}
	if input.Annotations != nil {
		keyed.Annotations = &keyedAnnotation{Globs: input.Annotations.Globs}
	}
	return keyed
}

// CacheKey returns the SHA-256 over the JCS of the subject and the inputs. Both
// arrays are sorted first, so the key does not depend on their order. An input
// contributes its role, name, digest and globs.
func CacheKey(subject []Subject, inputs []Input) (string, error) {
	sortedInputs := slices.Clone(inputs)
	slices.SortStableFunc(sortedInputs, compareInputs)
	keyed := struct {
		Inputs  []keyedInput `json:"inputs"`
		Subject []Subject    `json:"subject"`
	}{Inputs: make([]keyedInput, len(sortedInputs)), Subject: slices.Clone(subject)}
	for index, input := range sortedInputs {
		keyed.Inputs[index] = keyedInputOf(input)
	}
	slices.SortStableFunc(keyed.Subject, compareSubjects)
	canonical, err := Canonicalize(keyed)
	if err != nil {
		return "", err
	}
	return SHA256Hex(canonical), nil
}

func compareInputs(a, b Input) int {
	if byRole := strings.Compare(string(a.Role), string(b.Role)); byRole != 0 {
		return byRole
	}
	if byName := strings.Compare(a.Name, b.Name); byName != 0 {
		return byName
	}
	return strings.Compare(a.Digest.SHA256, b.Digest.SHA256)
}

func compareSubjects(a, b Subject) int {
	if byName := strings.Compare(a.Name, b.Name); byName != 0 {
		return byName
	}
	return strings.Compare(a.Digest.SHA256, b.Digest.SHA256)
}

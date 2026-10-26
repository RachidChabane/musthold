package fileset

import (
	"slices"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
)

func sampleTree() fstest.MapFS {
	return fstest.MapFS{
		"src/a.py":          {Data: []byte("a")},
		"src/pkg/b.py":      {Data: []byte("b")},
		"src/pkg/deep/c.py": {Data: []byte("c")},
		"src/readme.md":     {Data: []byte("r")},
		"tests/test_a.py":   {Data: []byte("t")},
	}
}

func paths(files []format.File) []string {
	result := make([]string, 0, len(files))
	for _, file := range files {
		result = append(result, file.Path)
	}
	return result
}

func TestCollectExpandsPatterns(t *testing.T) {
	tests := []struct {
		name     string
		globs    []string
		expected []string
	}{
		{"single pattern [verifies SP-46~1]", []string{"src/*.py"}, []string{"src/a.py"}},
		{"multiple patterns [verifies SP-46~1]", []string{"tests/*.py", "src/*.md"}, []string{"src/readme.md", "tests/test_a.py"}},
		{"recursive pattern [verifies SP-46~1]", []string{"src/**/*.py"}, []string{"src/a.py", "src/pkg/b.py", "src/pkg/deep/c.py"}},
		{"overlapping patterns are de-duplicated [verifies SP-46~1]", []string{"src/**/*.py", "src/pkg/*.py", "src/a.py"}, []string{"src/a.py", "src/pkg/b.py", "src/pkg/deep/c.py"}},
		{"directories are skipped [verifies SP-46~1]", []string{"src/*"}, []string{"src/a.py", "src/readme.md"}},
		{"directory contents by double star [verifies SP-46~1]", []string{"src/pkg/**"}, []string{"src/pkg/b.py", "src/pkg/deep/c.py"}},
		{"no match [verifies SP-46~1]", []string{"missing/**"}, []string{}},
		{"no pattern [verifies SP-46~1]", nil, []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files, err := Collect(sampleTree(), test.globs)
			if err != nil {
				t.Fatalf("Collect: %v", err)
			}
			if files == nil || !slices.Equal(paths(files), test.expected) {
				t.Fatalf("paths = %v, want %v", paths(files), test.expected)
			}
		})
	}
}

func TestCollectRejectsMalformedPattern(t *testing.T) {
	t.Run("unclosed bracket [verifies SP-46~1]", func(t *testing.T) {
		if _, err := Collect(sampleTree(), []string{"src/[a"}); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestCollectReadsContents(t *testing.T) {
	t.Run("content is the file content [verifies SP-46~1]", func(t *testing.T) {
		files, err := Collect(sampleTree(), []string{"src/a.py"})
		if err != nil || len(files) != 1 || string(files[0].Content) != "a" {
			t.Fatalf("files = %+v, err = %v", files, err)
		}
	})
}

func TestCollectDigestEqualsDigestOfDirectFiles(t *testing.T) {
	tests := []struct {
		name   string
		globs  []string
		direct []format.File
	}{
		{"recursive glob [verifies SP-46~1]", []string{"src/**/*.py"}, []format.File{
			{Path: "src/pkg/deep/c.py", Content: []byte("c")},
			{Path: "src/a.py", Content: []byte("a")},
			{Path: "src/pkg/b.py", Content: []byte("b")},
		}},
		{"two globs [verifies SP-46~1]", []string{"tests/*", "src/readme.md"}, []format.File{
			{Path: "tests/test_a.py", Content: []byte("t")},
			{Path: "src/readme.md", Content: []byte("r")},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collected, err := Collect(sampleTree(), test.globs)
			if err != nil {
				t.Fatalf("Collect: %v", err)
			}
			got, err := format.FileSetDigest(collected)
			if err != nil {
				t.Fatal(err)
			}
			want, err := format.FileSetDigest(test.direct)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("digest %s, want %s", got, want)
			}
		})
	}
}

func TestCollectDoesNotDependOnGlobOrder(t *testing.T) {
	patterns := []string{"src/*.py", "src/**/*.py", "tests/**", "src/readme.md", "missing/*", "**/*.md"}
	rapid.Check(t, func(t *rapid.T) {
		chosen := rapid.SliceOf(rapid.SampledFrom(patterns)).Draw(t, "globs")
		shuffled := rapid.Permutation(chosen).Draw(t, "shuffled")

		first, err := Collect(sampleTree(), chosen)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Collect(sampleTree(), shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(paths(first), paths(second)) {
			t.Fatalf("order of globs changed the result: %v vs %v", paths(first), paths(second))
		}
		firstDigest, _ := format.FileSetDigest(first)
		secondDigest, _ := format.FileSetDigest(second)
		if firstDigest != secondDigest {
			t.Fatalf("digests differ")
		}
	})
}

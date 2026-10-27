package product_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// imageDir is the image definition, seen from this package.
const imageDir = "../../../../../experiments/loop-1/image"

func readImageFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(imageDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestImageDefinition(t *testing.T) {
	t.Run("each source image is named by digest [verifies SP-2638~1]", func(t *testing.T) {
		images := readImageFile(t, "images")
		for variable, reference := range map[string]string{
			"GO_IMAGE":   "golang:1.26.4-bookworm",
			"NODE_IMAGE": "node:24-bookworm-slim",
			"UV_IMAGE":   "ghcr.io/astral-sh/uv:0.11.23",
		} {
			pattern := regexp.MustCompile(`(?m)^` + variable + `="` + regexp.QuoteMeta(reference) + `@sha256:[0-9a-f]{64}"$`)
			if !pattern.MatchString(images) {
				t.Errorf("%s is not %s pinned by digest in:\n%s", variable, reference, images)
			}
		}
	})

	t.Run("the Dockerfile builds from the pinned images and installs Python, pytest and Claude Code at their pins [verifies SP-2638~1]", func(t *testing.T) {
		dockerfile := readImageFile(t, "Dockerfile")
		for _, from := range regexp.MustCompile(`(?m)^FROM (\S+)`).FindAllStringSubmatch(dockerfile, -1) {
			if !strings.HasPrefix(from[1], "${") && from[1] != "uv" {
				t.Errorf("FROM %s is not a pinned build argument", from[1])
			}
		}
		for _, wanted := range []string{
			"ARG PYTHON_VERSION=3.13.14",
			"--require-hashes",
			"@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}",
			"DISABLE_AUTOUPDATER=1",
			"COPY --from=uv /uv ",
		} {
			if !strings.Contains(dockerfile, wanted) {
				t.Errorf("the Dockerfile lacks %q", wanted)
			}
		}
	})

	t.Run("pytest 9.1.1 is locked and every locked requirement carries hashes [verifies SP-2638~1]", func(t *testing.T) {
		lock := readImageFile(t, "requirements.lock")
		if !strings.Contains(lock, "\npytest==9.1.1 ") && !strings.HasPrefix(lock, "pytest==9.1.1 ") {
			t.Error("pytest 9.1.1 is not locked")
		}
		blocks := regexp.MustCompile(`(?m)^[A-Za-z0-9_.-]+==[^\n]*(?:\n[ \t]+[^\n]*)*`).FindAllString(lock, -1)
		if len(blocks) == 0 {
			t.Fatal("the lock holds no requirement")
		}
		for _, block := range blocks {
			if !regexp.MustCompile(`--hash=sha256:[0-9a-f]{64}`).MatchString(block) {
				t.Errorf("no hash for:\n%s", block)
			}
		}
	})
}

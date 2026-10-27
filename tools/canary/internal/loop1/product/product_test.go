package product_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
)

const (
	commit  = "0123456789abcdef0123456789abcdef01234567"
	pin     = "c331f9b97d5b96bdc18eba3b17be870c004dc239"
	digest  = "0f0e0d0c0b0a09080706050403020100ffeeddccbbaa99887766554433221100"
	image   = "golang:1.26.4-bookworm@sha256:" + digest
	builtID = "sha256:" + digest
)

func valid() product.Manifest {
	manifest := product.Manifest{
		Commit: commit, ShallnotPin: pin, Arch: product.ArchARM64,
		Binaries: map[string]string{}, Plugins: map[string]string{},
		Images: product.Images{Build: image, Base: "node:24-bookworm-slim@sha256:" + digest, UV: "ghcr.io/astral-sh/uv:0.11.23@sha256:" + digest, Built: builtID},
	}
	for _, name := range product.RequiredBinaries() {
		manifest.Binaries[name] = digest
	}
	for _, path := range product.PluginPaths() {
		manifest.Plugins[path] = digest
	}
	return manifest
}

func TestManifest(t *testing.T) {
	t.Run("a manifest is read back as it was written [verifies SP-2630~1]", func(t *testing.T) {
		written, err := valid().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		read, err := product.Parse(written)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := read.Marshal()
		if !bytes.Equal(written, again) {
			t.Errorf("round trip differs:\n%s\n%s", written, again)
		}
	})

	t.Run("the manifest names the plugin trees of the four-proofs arm and the binaries the study runs [verifies SP-2630~1]", func(t *testing.T) {
		wantPlugins := []string{"alignment/plugin/claude", "completeness/plugin/claude", "discpower/plugin/claude", "musthold", "s1relay/plugin/claude", "shallnot"}
		if got := product.PluginPaths(); !slices.Equal(got, wantPlugins) {
			t.Errorf("plugins = %v", got)
		}
		wantBinaries := []string{"musthold", "musthold-alignment", "musthold-completeness", "musthold-demo-loop", "musthold-dp", "musthold-verify", "shallnot"}
		if got := product.RequiredBinaries(); !slices.Equal(got, wantBinaries) {
			t.Errorf("binaries = %v", got)
		}
	})

	refusals := map[string]struct {
		edit func(m *product.Manifest)
		want error
	}{
		"a missing binary":                {func(m *product.Manifest) { delete(m.Binaries, "shallnot") }, product.ErrIncomplete},
		"a missing plugin tree":           {func(m *product.Manifest) { delete(m.Plugins, "musthold") }, product.ErrIncomplete},
		"a malformed binary digest":       {func(m *product.Manifest) { m.Binaries["musthold"] = "abc" }, product.ErrDigest},
		"a malformed tree digest":         {func(m *product.Manifest) { m.Plugins["shallnot"] = strings.ToUpper(digest) }, product.ErrDigest},
		"a short commit":                  {func(m *product.Manifest) { m.Commit = "0123" }, product.ErrDigest},
		"a short Shallnot pin":            {func(m *product.Manifest) { m.ShallnotPin = "5e4783e" }, product.ErrDigest},
		"an image named by tag":           {func(m *product.Manifest) { m.Images.Build = "golang:1.26.4-bookworm" }, product.ErrDigest},
		"a built image without an id":     {func(m *product.Manifest) { m.Images.Built = "musthold:latest" }, product.ErrDigest},
		"an architecture the study lacks": {func(m *product.Manifest) { m.Arch = "riscv64" }, product.ErrManifest},
	}
	for name, refusal := range refusals {
		t.Run(name+" is refused [verifies SP-2630~1]", func(t *testing.T) {
			manifest := valid()
			refusal.edit(&manifest)
			data, _ := json.Marshal(manifest)
			if _, err := product.Parse(data); !errors.Is(err, refusal.want) {
				t.Errorf("error = %v, want %v", err, refusal.want)
			}
			if _, err := manifest.Marshal(); !errors.Is(err, refusal.want) {
				t.Errorf("Marshal error = %v, want %v", err, refusal.want)
			}
		})
	}

	t.Run("an unknown member, a syntax error and trailing data are refused [verifies SP-2630~1]", func(t *testing.T) {
		good, _ := valid().Marshal()
		for name, data := range map[string][]byte{
			"unknown member": bytes.Replace(good, []byte(`"dirty"`), []byte(`"extra": 1, "dirty"`), 1),
			"syntax":         []byte(`{"commit":`),
			"trailing":       append(append([]byte{}, good...), []byte(`{}`)...),
		} {
			if _, err := product.Parse(data); !errors.Is(err, product.ErrManifest) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})
}

func TestTreeDigest(t *testing.T) {
	t.Run("the digest depends on paths and content, never on the order files were added [verifies SP-2631~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z]{1,4}(/[a-z]{1,4})?`), 2, 6, func(s string) string { return s }).Draw(t, "names")
			files := fstest.MapFS{}
			for _, name := range names {
				files[name] = &fstest.MapFile{Data: []byte(rapid.String().Draw(t, "content")), Mode: 0o644}
			}
			reversed := fstest.MapFS{}
			for _, name := range slices.Backward(names) {
				reversed[name] = &fstest.MapFile{Data: files[name].Data, Mode: 0o755}
			}
			first, err := product.TreeDigest(files)
			if err != nil {
				t.Skip("a name is both a file and a directory")
			}
			second, err := product.TreeDigest(reversed)
			if err != nil || first != second {
				t.Fatalf("digests differ: %q %q (%v)", first, second, err)
			}
			files[names[0]] = &fstest.MapFile{Data: append(files[names[0]].Data, 'x')}
			if changed, _ := product.TreeDigest(files); changed == first {
				t.Fatal("a changed file keeps the digest")
			}
		})
	})

	t.Run("a renamed file changes the digest [verifies SP-2631~1]", func(t *testing.T) {
		before, _ := product.TreeDigest(fstest.MapFS{"a": {Data: []byte("x")}})
		after, _ := product.TreeDigest(fstest.MapFS{"b": {Data: []byte("x")}})
		if before == after {
			t.Error("a rename keeps the digest")
		}
	})

	t.Run("a symbolic link is refused [verifies SP-2631~1]", func(t *testing.T) {
		link := fstest.MapFS{"link": {Data: []byte("target"), Mode: 0o777 | os.ModeSymlink}}
		if _, err := product.TreeDigest(link); !errors.Is(err, product.ErrNotRegular) {
			t.Errorf("error = %v", err)
		}
	})
}

// writeTree writes files, named by slash-separated path, under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// sources writes a staged repository and a Shallnot tree that hold every plugin tree.
func sources(t *testing.T) (repository, shallnot string) {
	repository, shallnot = t.TempDir(), t.TempDir()
	for _, path := range product.PluginPaths() {
		switch path {
		case "shallnot":
			writeTree(t, shallnot, map[string]string{"plugin/.claude-plugin/plugin.json": "{}", "plugin/hooks/stop.sh": "stop"})
		case "musthold":
			writeTree(t, repository, map[string]string{"umbrella/plugins/musthold/.claude-plugin/plugin.json": "{}"})
		default:
			writeTree(t, repository, map[string]string{path + "/.claude-plugin/plugin.json": path})
		}
	}
	return repository, shallnot
}

func TestBuildProduct(t *testing.T) {
	run := func(t *testing.T, productDir string, extra ...string) (int, string) {
		repository, shallnot := sources(t)
		args := append([]string{
			"-product", productDir, "-source", repository, "-shallnot-tree", shallnot, "-commit", commit, "-shallnot-pin", pin,
			"-build-image", image, "-base-image", "node:24-bookworm-slim@sha256:" + digest, "-uv-image", "uv@sha256:" + digest, "-built-image", builtID,
		}, extra...)
		var stdout, stderr bytes.Buffer
		code := product.Main(context.Background(), args, &stdout, &stderr)
		return code, stderr.String()
	}
	binaries := map[string]string{}
	for _, name := range product.RequiredBinaries() {
		binaries["bin/"+name] = "binary " + name
	}

	t.Run("the plugin trees are copied from their sources and every binary and tree is digested into the manifest [verifies SP-2632~1]", func(t *testing.T) {
		productDir := t.TempDir()
		writeTree(t, productDir, binaries)
		writeTree(t, productDir, map[string]string{"plugins/stale/file": "left over"})
		if code, stderr := run(t, productDir); code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		data, err := os.ReadFile(filepath.Join(productDir, product.ManifestFile))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := product.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(productDir, "plugins", "stale")); !errors.Is(err, os.ErrNotExist) {
			t.Error("a plugin tree of an earlier build is still there")
		}
		if _, err := os.Stat(filepath.Join(productDir, "plugins", "shallnot", "hooks", "stop.sh")); err != nil {
			t.Errorf("Shallnot's plugin tree is not copied from the plugin directory of its source: %v", err)
		}
		want, _ := product.FileDigest(os.DirFS(filepath.Join(productDir, "bin")), "musthold")
		if manifest.Binaries["musthold"] != want || manifest.Commit != commit || manifest.Images.Built != builtID {
			t.Errorf("manifest = %+v", manifest)
		}
		tree, _ := product.TreeDigest(os.DirFS(filepath.Join(productDir, "plugins", "musthold")))
		if manifest.Plugins["musthold"] != tree {
			t.Errorf("umbrella tree digest = %s, want %s", manifest.Plugins["musthold"], tree)
		}
	})

	t.Run("a product without binaries, and a missing flag, fail with the matching status [verifies SP-2632~1]", func(t *testing.T) {
		if code, _ := run(t, t.TempDir()); code != 1 {
			t.Errorf("an empty product gives exit %d", code)
		}
		if code := product.Main(context.Background(), []string{"-product", t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Errorf("missing flags give exit %d", code)
		}
	})

	t.Run("a binary that is a symbolic link is refused [verifies SP-2632~1]", func(t *testing.T) {
		productDir := t.TempDir()
		writeTree(t, productDir, binaries)
		link := filepath.Join(productDir, "bin", "musthold")
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/bin/sh", link); err != nil {
			t.Fatal(err)
		}
		if code, _ := run(t, productDir); code != 1 {
			t.Errorf("exit %d", code)
		}
	})
}

package projectconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/adapters/projectconfig"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, projectconfig.Directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, projectconfig.File), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoad(t *testing.T) {
	t.Run("the systemone section of a shared configuration is read and the other sections are ignored [verifies SP-601~1]", func(t *testing.T) {
		root := writeConfig(t, "proofs:\n  discpower: true\nsystemone:\n  endpoint: http://localhost:11434\n  model: tev1:0.8b\nhost:\n  sampling: true\n")
		got, err := projectconfig.Load(root)
		if err != nil || got.Endpoint != "http://localhost:11434" || got.Model != "tev1:0.8b" {
			t.Fatalf("Load = %+v, %v", got, err)
		}
	})

	t.Run("a project without configuration or without the section has none [verifies SP-601~1]", func(t *testing.T) {
		if got, err := projectconfig.Load(t.TempDir()); err != nil || got != (projectconfig.SystemOne{}) {
			t.Fatalf("no file: %+v, %v", got, err)
		}
		if got, err := projectconfig.Load(writeConfig(t, "proofs: {}\n")); err != nil || got != (projectconfig.SystemOne{}) {
			t.Fatalf("no section: %+v, %v", got, err)
		}
	})

	t.Run("a configuration that is not YAML is an error [verifies SP-601~1]", func(t *testing.T) {
		if _, err := projectconfig.Load(writeConfig(t, "systemone: [unclosed")); err == nil {
			t.Fatal("a broken file was read")
		}
	})
}

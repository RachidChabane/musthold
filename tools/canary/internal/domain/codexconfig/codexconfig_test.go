package codexconfig_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/codexconfig"
)

const project = "/private/var/folders/x/T/spec-proof-canary-codex-4821/project"

func TestRemoveProjectTable(t *testing.T) {
	t.Run("the table Codex appends is removed and the file is what it was before the append [verifies SP-1512~1]", func(t *testing.T) {
		before := "model = \"gpt-6-luna\"\n\n[projects.\"/home/u/work\"]\ntrust_level = \"trusted\"\n"
		appended := before + "\n[projects.\"" + project + "\"]\ntrust_level = \"trusted\"\n"
		removal := codexconfig.RemoveProjectTable([]byte(appended), project)
		if !removal.Found || string(removal.Cleaned) != before {
			t.Fatalf("found %v, cleaned %q", removal.Found, removal.Cleaned)
		}
		if removal.Table != "[projects.\""+project+"\"]\ntrust_level = \"trusted\"\n" {
			t.Errorf("table = %q", removal.Table)
		}
	})

	t.Run("a table in the middle of the file takes its own keys and no other table [verifies SP-1512~1]", func(t *testing.T) {
		content := "[tui]\ntheme = \"dark\"\n\n[projects.\"" + project + "\"]\ntrust_level = \"trusted\"\n\n[projects.\"/home/u/work\"]\ntrust_level = \"trusted\"\n"
		removal := codexconfig.RemoveProjectTable([]byte(content), project)
		want := "[tui]\ntheme = \"dark\"\n\n[projects.\"/home/u/work\"]\ntrust_level = \"trusted\"\n"
		if !removal.Found || string(removal.Cleaned) != want {
			t.Fatalf("cleaned %q, want %q", removal.Cleaned, want)
		}
	})

	t.Run("a file that holds only the table becomes empty [verifies SP-1512~1]", func(t *testing.T) {
		removal := codexconfig.RemoveProjectTable([]byte("[projects.\""+project+"\"]\ntrust_level = \"trusted\"\n"), project)
		if !removal.Found || len(removal.Cleaned) != 0 {
			t.Fatalf("cleaned %q", removal.Cleaned)
		}
	})

	t.Run("a literal key and a trailing comment name the same table [verifies SP-1512~1]", func(t *testing.T) {
		content := "a = 1\n[projects.'" + project + "'] # trusted for the run\ntrust_level = \"trusted\"\n"
		removal := codexconfig.RemoveProjectTable([]byte(content), project)
		if !removal.Found || string(removal.Cleaned) != "a = 1\n" {
			t.Fatalf("cleaned %q", removal.Cleaned)
		}
	})

	t.Run("a table of another project, a sub-table and a longer path are left alone [verifies SP-1512~1]", func(t *testing.T) {
		content := "[projects.\"" + project + "-2\"]\ntrust_level = \"trusted\"\n[projects.\"" + project + "\".extra]\nk = 1\n[projects.\"/other\"]\n"
		removal := codexconfig.RemoveProjectTable([]byte(content), project)
		if removal.Found || string(removal.Cleaned) != content {
			t.Fatalf("found %v, cleaned %q", removal.Found, removal.Cleaned)
		}
	})

	t.Run("a file without the table comes back byte for byte, whatever its endings [verifies SP-1512~1]", func(t *testing.T) {
		for _, content := range []string{"", "a = 1", "a = 1\r\n[b]\r\nc = 2\r\n"} {
			removal := codexconfig.RemoveProjectTable([]byte(content), project)
			if removal.Found || string(removal.Cleaned) != content {
				t.Errorf("%q: found %v, cleaned %q", content, removal.Found, removal.Cleaned)
			}
		}
	})
}

func TestNamesProject(t *testing.T) {
	t.Run("a file names the project wherever the path appears [verifies SP-1512~1]", func(t *testing.T) {
		if !codexconfig.NamesProject([]byte("k = \""+project+"\"\n"), project) || codexconfig.NamesProject([]byte("k = 1\n"), project) {
			t.Error("NamesProject misreads the file")
		}
	})
}

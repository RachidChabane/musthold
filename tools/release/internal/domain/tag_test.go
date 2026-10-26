package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const (
	verifiesTags     = " [verifies SP-1800~1]"
	verifiesTable    = " [verifies SP-1801~1]"
	verifiesPrecedes = " [verifies SP-1802~1]"
)

func mustTag(t testing.TB, text string) domain.Tag {
	t.Helper()
	tag, err := domain.ParseTag(text)
	if err != nil {
		t.Fatalf("ParseTag(%q): %v", text, err)
	}
	return tag
}

func TestParseTag(t *testing.T) {
	t.Run("a tag of a released module is accepted with its version"+verifiesTags, func(t *testing.T) {
		cases := map[string]string{
			"discpower/v0.2.0":        "0.2.0",
			"format/v1.10.3":          "1.10.3",
			"s1relay/v0.0.0-e2e":      "0.0.0-e2e",
			"umbrella/v2.0.0-rc.1":    "2.0.0-rc.1",
			"ledger/v10.20.30-alpha1": "10.20.30-alpha1",
		}
		for text, semver := range cases {
			tag := mustTag(t, text)
			if tag.Semver() != semver || tag.String() != text {
				t.Errorf("%s: semver %q, string %q", text, tag.Semver(), tag.String())
			}
		}
	})

	t.Run("a malformed tag, a bare version and a leading zero are refused"+verifiesTags, func(t *testing.T) {
		for _, text := range []string{
			"", "v0.2.0", "discpower", "discpower/0.2.0", "discpower/v0.2", "discpower/v0.2.0.1",
			"discpower/v01.2.3", "discpower/v1.02.3", "discpower/v1.2.03", "discpower/v-1.2.3",
			"discpower/v1.2.3-", "discpower/v1.2.3+build", "Discpower/v1.0.0", "discpower/v1.0.0 ",
			"discpower/v99999999999999999999.0.0",
		} {
			if tag, err := domain.ParseTag(text); err == nil {
				t.Errorf("%q parsed as %v", text, tag)
			}
		}
	})

	t.Run("a module outside the release table is refused, tools included"+verifiesTags, func(t *testing.T) {
		for _, text := range []string{"tools/release/v1.0.0", "tools/canary/v0.1.0", "bench/v1.0.0", "experiments/v1.0.0", "shallnot/v1.0.0"} {
			_, err := domain.ParseTag(text)
			if !errors.Is(err, domain.ErrNotReleased) {
				t.Errorf("%q: %v", text, err)
			}
		}
	})

	t.Run("rendering a parsed tag gives the text back for every module and version"+verifiesTags, func(t *testing.T) {
		names := []string{}
		for _, module := range domain.Modules() {
			names = append(names, module.Name)
		}
		rapid.Check(t, func(t *rapid.T) {
			module := rapid.SampledFrom(names).Draw(t, "module")
			text := fmt.Sprintf("%s/v%d.%d.%d", module, rapid.IntRange(0, 99).Draw(t, "major"), rapid.IntRange(0, 99).Draw(t, "minor"), rapid.IntRange(0, 99).Draw(t, "patch"))
			if rapid.Bool().Draw(t, "pre") {
				text += "-" + rapid.StringMatching(`[0-9A-Za-z][0-9A-Za-z.-]{0,8}`).Draw(t, "suffix")
			}
			tag, err := domain.ParseTag(text)
			if err != nil || tag.String() != text {
				t.Fatalf("%q: tag %v, error %v", text, tag, err)
			}
		})
	})

	t.Run("a number with a leading zero is refused whatever the module"+verifiesTags, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			zeros := rapid.StringMatching(`0[0-9]{1,3}`).Draw(t, "number")
			if _, err := domain.ParseTag("format/v" + zeros + ".0.0"); err == nil {
				t.Fatalf("%s accepted", zeros)
			}
		})
	})
}

func TestReleaseTable(t *testing.T) {
	t.Run("the table lists each module with its kind, binary and plugin"+verifiesTable, func(t *testing.T) {
		type row struct {
			kind   domain.Kind
			binary string
			plugin string
		}
		want := map[string]row{
			"format":       {domain.KindLibrary, "", ""},
			"discpower":    {domain.KindBinaryPlugin, "musthold-dp", "musthold-dp"},
			"alignment":    {domain.KindBinaryPlugin, "musthold-alignment", "musthold-alignment"},
			"completeness": {domain.KindBinaryPlugin, "musthold-completeness", "musthold-completeness"},
			"s1relay":      {domain.KindBinaryPlugin, "musthold-s1", "musthold-s1"},
			"ledger":       {domain.KindBinary, "musthold-ledger", ""},
			"verify":       {domain.KindBinary, "musthold-verify", ""},
			"umbrella":     {domain.KindPlugin, "", "musthold"},
		}
		modules := domain.Modules()
		if len(modules) != len(want) || modules[0].Name != domain.FormatModuleName {
			t.Fatalf("modules = %v", modules)
		}
		for _, module := range modules {
			expected, listed := want[module.Name]
			if !listed || module.Kind != expected.kind || module.Binary != expected.binary || module.PluginName != expected.plugin {
				t.Errorf("%s = %+v, want %+v", module.Name, module, expected)
			}
		}
	})

	t.Run("a module ships a binary or a plugin exactly as its kind says"+verifiesTable, func(t *testing.T) {
		for _, module := range domain.Modules() {
			if module.ShipsBinary() != (module.Binary != "") {
				t.Errorf("%s: ships binary %v with binary %q", module.Name, module.ShipsBinary(), module.Binary)
			}
			if module.ShipsPlugin() != (module.PluginName != "") {
				t.Errorf("%s: ships plugin %v with plugin %q", module.Name, module.ShipsPlugin(), module.PluginName)
			}
			if module.ShipsPlugin() && len(module.PluginRoots) == 0 {
				t.Errorf("%s ships a plugin and names no plugin directory", module.Name)
			}
			if module.ShipsBinary() && module.Summary == "" {
				t.Errorf("%s ships a binary and has no summary", module.Name)
			}
		}
	})

	t.Run("the plan of a tag gives the module, kind, binary, version, parts and CI modules"+verifiesTable, func(t *testing.T) {
		got := map[string]string{}
		for _, output := range domain.PlanOutputs(mustTag(t, "discpower/v0.2.0")) {
			got[output.Key] = output.Value
		}
		want := map[string]string{
			"tag": "discpower/v0.2.0", "module": "discpower", "kind": "binary+plugin", "binary": "musthold-dp",
			"semver": "0.2.0", "ships_binary": "true", "ships_plugin": "true", "ci_modules": "format discpower",
		}
		for key, value := range want {
			if got[key] != value {
				t.Errorf("%s = %q, want %q", key, got[key], value)
			}
		}
	})

	t.Run("the plan of the format library gates it alone and ships nothing built"+verifiesTable, func(t *testing.T) {
		got := map[string]string{}
		for _, output := range domain.PlanOutputs(mustTag(t, "format/v1.0.0")) {
			got[output.Key] = output.Value
		}
		if got["ci_modules"] != "format" || got["ships_binary"] != "false" || got["ships_plugin"] != "false" || got["kind"] != "library" {
			t.Errorf("plan = %v", got)
		}
	})
}

func TestVersionPrecedence(t *testing.T) {
	ordered := []string{"0.9.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}

	t.Run("versions are ordered by semantic-version precedence"+verifiesPrecedes, func(t *testing.T) {
		for low := range ordered {
			for high := low + 1; high < len(ordered); high++ {
				a, b := mustTag(t, "format/v"+ordered[low]).Version, mustTag(t, "format/v"+ordered[high]).Version
				if a.Compare(b) >= 0 || b.Compare(a) <= 0 {
					t.Errorf("%s should precede %s", ordered[low], ordered[high])
				}
			}
		}
	})

	t.Run("comparing a version with itself gives zero and swapping flips the sign"+verifiesPrecedes, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			a := domain.Version{Major: rapid.IntRange(0, 3).Draw(t, "a major"), Minor: rapid.IntRange(0, 3).Draw(t, "a minor"), Patch: rapid.IntRange(0, 3).Draw(t, "a patch")}
			b := domain.Version{Major: rapid.IntRange(0, 3).Draw(t, "b major"), Minor: rapid.IntRange(0, 3).Draw(t, "b minor"), Patch: rapid.IntRange(0, 3).Draw(t, "b patch")}
			if a.Compare(a) != 0 || a.Compare(b) != -b.Compare(a) {
				t.Fatalf("%v and %v", a, b)
			}
		})
	})
}

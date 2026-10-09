package hostsettings_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostsettings"
)

func TestClassifySettings(t *testing.T) {
	t.Run("a file without the switch classifies in auto and opts into no witness [verifies SP-1907~1]", func(t *testing.T) {
		for _, config := range []string{"", "host:\n  sampling: on\n", "systemone:\n  endpoint: http://localhost\n"} {
			settings, err := hostsettings.Parse([]byte(config))
			if err != nil || settings.ClassifyModeFor(format.KindAlignment) != hostproto.ClassifyAuto || settings.ClassifyWitnessDir != "" {
				t.Fatalf("%q: %+v, %v", config, settings, err)
			}
		}
	})

	t.Run("the switch is read from the systemone section next to the relay's own members [verifies SP-1907~1]", func(t *testing.T) {
		for text, want := range map[string]hostproto.ClassifyMode{"auto": hostproto.ClassifyAuto, "shadow": hostproto.ClassifyShadow, "off": hostproto.ClassifyOff} {
			settings, err := hostsettings.Parse([]byte("systemone:\n  endpoint: http://localhost:11434\n  model: example\n  classify: " + text + "\n"))
			if err != nil || settings.ClassifyModeFor(format.KindCompleteness) != want || settings.IsDefault() {
				t.Fatalf("%s: %+v, %v", text, settings, err)
			}
		}
	})

	t.Run("a mapping sets the switch per proof kind with a default for the others [verifies SP-1907~1]", func(t *testing.T) {
		settings, err := hostsettings.Parse([]byte("systemone:\n  classify:\n    default: off\n    completeness: shadow\n"))
		if err != nil {
			t.Fatal(err)
		}
		if settings.ClassifyModeFor(format.KindCompleteness) != hostproto.ClassifyShadow ||
			settings.ClassifyModeFor(format.KindAlignment) != hostproto.ClassifyOff || settings.ClassifyModeFor(format.KindDiscriminatingPower) != hostproto.ClassifyOff {
			t.Fatalf("%+v", settings)
		}
		onlyKind, err := hostsettings.Parse([]byte("systemone:\n  classify:\n    alignment: shadow\n"))
		if err != nil || onlyKind.ClassifyModeFor(format.KindAlignment) != hostproto.ClassifyShadow || onlyKind.ClassifyModeFor(format.KindCompleteness) != hostproto.ClassifyAuto {
			t.Fatalf("%+v, %v", onlyKind, err)
		}
	})

	t.Run("another value of the switch, or a mapping member that names no module, is refused [verifies SP-1907~1]", func(t *testing.T) {
		for _, config := range []string{
			"systemone:\n  classify: on\n", "systemone:\n  classify: true\n", "systemone:\n  classify: Auto\n", "systemone:\n  classify: 1\n",
			"systemone:\n  classify:\n    dp: shadow\n", "systemone:\n  classify:\n    completeness: maybe\n", "systemone:\n  classify: [auto]\n",
		} {
			if _, err := hostsettings.Parse([]byte(config)); !errors.Is(err, hostsettings.ErrInvalidSection) {
				t.Errorf("%q: error %v", config, err)
			}
		}
	})

	t.Run("the witness directory is read from host.classify.witness_dir and nowhere else [verifies SP-1907~1]", func(t *testing.T) {
		settings, err := hostsettings.Parse([]byte("host:\n  classify:\n    witness_dir: /var/log/claude-bodies\n"))
		if err != nil || settings.ClassifyWitnessDir != "/var/log/claude-bodies" || settings.IsDefault() {
			t.Fatalf("%+v, %v", settings, err)
		}
		for _, config := range []string{"host:\n  classify:\n    witness: /x\n", "host:\n  witness_dir: /x\n"} {
			if _, err := hostsettings.Parse([]byte(config)); !errors.Is(err, hostsettings.ErrInvalidSection) {
				t.Errorf("%q: error %v", config, err)
			}
		}
	})
}

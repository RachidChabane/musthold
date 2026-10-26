package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostsettings"
)

// hostSettingsOf reads the settings of the host harness and of the
// classification from the configuration the modules share; a project without one
// has the defaults.
func hostSettingsOf(root string) (hostsettings.Settings, error) {
	data, err := os.ReadFile(filepath.Join(root, projectconfig.Directory, projectconfig.FileName))
	if errors.Is(err, os.ErrNotExist) {
		return hostsettings.Settings{}, nil
	}
	if err != nil {
		return hostsettings.Settings{}, fmt.Errorf("read the configuration: %w", err)
	}
	return hostsettings.Parse(data)
}

// classifiesFirst reports whether a classification may answer a request of this
// module: the switch is `auto` and the admission table holds an entry for the
// module's decision kind. Only then is the first vote of a closed request asked
// on its own.
func (e Env) classifiesFirst(root string) (bool, error) {
	settings, err := hostSettingsOf(root)
	if err != nil {
		return false, err
	}
	if settings.ClassifyModeFor(format.KindDiscriminatingPower) != hostproto.ClassifyAuto {
		return false, nil
	}
	admission, err := e.classifyAdmission()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(admission.Entries, func(entry hostproto.ClassifyAdmissionEntry) bool {
		return entry.Kind == format.KindDiscriminatingPower
	}), nil
}

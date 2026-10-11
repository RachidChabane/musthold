package golang

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/format"
)

var _ ports.Compiling = (*Strategy)(nil)

const (
	jsonFlag           = "-json"
	formatFlag         = "-f"
	importConfigFormat = "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}"
	compileTool        = "compile"
	packageFlag        = "-p"
	importConfigFlag   = "-importcfg"
	assemblyFlag       = "-S"
	languageFlag       = "-lang"
	languagePrefix     = "go"
	importConfigSuffix = ".importcfg"
	objectSuffix       = ".o"
	textSymbolMarker   = " STEXT"
	closureMarker      = ".func"
	siteSeparator      = "::"
	qualnameSeparator  = "."
	formFileMode       = 0o600
)

// positionRegexp matches the source position the compiler prints on every
// instruction, which the compiled form leaves out.
var positionRegexp = regexp.MustCompile(`\([^()\s]*:\d+\)`)

// Compiler compiles the package of each edited file with `go tool compile -S`
// and digests the assembly of each function with every source position left
// out (E0).
func (s *Strategy) Compiler() ports.FormCompiler { return compiler{deps: s.deps} }

type compiler struct {
	deps Dependencies
}

// compiledPackage is what `go list -json` says of a package.
type compiledPackage struct {
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	Dir        string   `json:"Dir"`
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
	Module     *struct {
		GoVersion string `json:"GoVersion"`
	} `json:"Module"`
}

// Forms returns the compiled form of each function site of the packages the
// sites lie in; a package that uses cgo or does not compile gives none.
func (c compiler) Forms(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, sites []string) (differential.CompiledForms, error) {
	byDir := map[string][]string{}
	for _, site := range sites {
		file, _, _ := strings.Cut(site, siteSeparator)
		byDir[path.Dir(file)] = append(byDir[path.Dir(file)], site)
	}
	forms := differential.CompiledForms{}
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		symbols, importPath, err := c.assembly(ctx, workspace, toolchain, dir)
		if err != nil {
			return nil, err
		}
		for _, site := range byDir[dir] {
			if form := formOf(symbols, importPath, site); form != "" {
				forms[site] = form
			}
		}
	}
	return forms, nil
}

// assembly compiles the package of a directory and returns the assembly of each
// text symbol, positions left out, by symbol, with the package's import path;
// no symbol when it cannot be compiled.
func (c compiler) assembly(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, dir string) (map[string]string, string, error) {
	listed, ok, err := c.list(ctx, workspace, toolchain, dir)
	if err != nil || !ok || len(listed.CgoFiles) > 0 || len(listed.GoFiles) == 0 {
		return nil, "", err
	}
	config, err := c.run(ctx, workspace, toolchain, listSubcommand, exportFlag, depsFlag, formatFlag, importConfigFormat, packagePath(dir))
	if err != nil || config.ExitCode != 0 {
		return nil, "", err
	}
	configFile := suiterun.ResultsFile(workspace, importConfigSuffix)
	if err := os.WriteFile(configFile, config.Stdout, formFileMode); err != nil {
		return nil, "", fmt.Errorf("write the import configuration: %w", err)
	}
	args := []string{toolSubcommand, compileTool, packageFlag, listed.ImportPath, importConfigFlag, configFile, outputFlag, suiterun.ResultsFile(workspace, objectSuffix), assemblyFlag}
	if listed.Module != nil && listed.Module.GoVersion != "" {
		args = append(args, languageFlag+"="+languagePrefix+languageVersion(listed.Module.GoVersion))
	}
	for _, file := range listed.GoFiles {
		args = append(args, filepath.Join(listed.Dir, file))
	}
	compiled, err := c.run(ctx, workspace, toolchain, args...)
	if err != nil || compiled.ExitCode != 0 {
		return nil, "", err
	}
	return textSymbols(string(compiled.Stdout)), listed.ImportPath, nil
}

func (c compiler) list(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, dir string) (compiledPackage, bool, error) {
	result, err := c.run(ctx, workspace, toolchain, listSubcommand, jsonFlag, packagePath(dir))
	if err != nil || result.ExitCode != 0 {
		return compiledPackage{}, false, err
	}
	var listed compiledPackage
	if err := json.Unmarshal(result.Stdout, &listed); err != nil {
		return compiledPackage{}, false, nil
	}
	return listed, true, nil
}

func (c compiler) run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, args ...string) (ports.CommandResult, error) {
	argv := append(append([]string{}, toolchain.Command...), args...)
	result, err := c.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: c.deps.Timeout})
	if err != nil {
		return ports.CommandResult{}, fmt.Errorf("run %s: %w", strings.Join(args[:1], " "), err)
	}
	return result, nil
}

// languageVersion keeps the major and minor version of a `go` directive, the
// form the compiler's `-lang` takes: 1.26.4 becomes 1.26.
func languageVersion(version string) string {
	parts := strings.SplitN(version, qualnameSeparator, languageVersionParts+1)
	if len(parts) > languageVersionParts {
		parts = parts[:languageVersionParts]
	}
	return strings.Join(parts, qualnameSeparator)
}

const languageVersionParts = 2

// textSymbols splits the compiler's assembly listing into the instructions of
// each text symbol, with every source position left out.
func textSymbols(listing string) map[string]string {
	symbols := map[string]string{}
	current := ""
	var body strings.Builder
	flush := func() {
		if current != "" {
			symbols[current] = body.String()
		}
		body.Reset()
	}
	for _, line := range strings.Split(listing, "\n") {
		switch {
		case !strings.HasPrefix(line, "\t") && strings.Contains(line, textSymbolMarker):
			flush()
			current = strings.Fields(line)[0]
		case !strings.HasPrefix(line, "\t"):
			flush()
			current = ""
		case current != "":
			body.WriteString(positionRegexp.ReplaceAllString(line, "") + "\n")
		}
	}
	flush()
	return symbols
}

// formOf digests the assembly of a site's function and of its closures, or
// returns "" when the listing holds none of them.
func formOf(symbols map[string]string, importPath, site string) string {
	_, qualname, _ := strings.Cut(site, siteSeparator)
	for _, candidate := range symbolCandidates(qualname) {
		symbol := importPath + qualnameSeparator + candidate
		if _, known := symbols[symbol]; !known {
			continue
		}
		var names []string
		for name := range symbols {
			if name == symbol || strings.HasPrefix(name, symbol+closureMarker) {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		var listing strings.Builder
		for _, name := range names {
			listing.WriteString(strings.TrimPrefix(name, importPath) + "\n" + symbols[name])
		}
		return format.SHA256Hex([]byte(listing.String()))
	}
	return ""
}

// symbolCandidates lists the names the compiler may give the symbol of a
// locator's qualified name after the import path: `(*T).Method` and `T.Method`
// as they are, `pkg.Func` without its package name.
func symbolCandidates(qualname string) []string {
	candidates := []string{qualname}
	if _, name, found := strings.Cut(qualname, qualnameSeparator); found && !strings.HasPrefix(qualname, "(") {
		candidates = append(candidates, name)
	}
	return candidates
}

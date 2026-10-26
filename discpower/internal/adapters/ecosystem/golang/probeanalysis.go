package golang

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang/probeharness"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// ErrUnsupportedTarget is returned for a target the prober cannot call: it is
// missing, generic, or not a function or a method of a type with a zero value.
var ErrUnsupportedTarget = errors.New("unsupported operation target")

const (
	listSubcommand = "list"
	exportFlag     = "-export"
	depsFlag       = "-deps"
	listFields     = "-json=ImportPath,Dir,Export,Error"
	internalDir    = "internal"
	goCompiler     = "gc"
	errorTypeName  = "error"
)

// operation is a bound operation as the type checker sees it.
type operation struct {
	Target probeTarget
	// Dir is the directory of the package of the target.
	Dir          string
	Context      bool
	Params       []probeharness.Param
	ReturnsError bool
	// Sentinels are the exported error variables of the package, sorted.
	Sentinels []string
}

type listedPackage struct {
	ImportPath string
	Dir        string
	Export     string
	Error      *struct{ Err string }
}

// loadOperations derives the operations of the targets from the type information
// of their packages in the workspace, which `go list -export` has the compiler
// produce: each target's parameters, results and the error variables of its
// package.
func (p prober) loadOperations(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, targets []probeTarget) ([]operation, error) {
	listed, err := p.listPackages(ctx, workspace, toolchain, targets)
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	for _, pkg := range listed {
		exports[pkg.ImportPath] = pkg.Export
	}
	imports := importer.ForCompiler(token.NewFileSet(), goCompiler, func(path string) (io.ReadCloser, error) {
		file, found := exports[path]
		if !found || file == "" {
			return nil, fmt.Errorf("no export data for %q", path)
		}
		return os.Open(file)
	})
	operations := make([]operation, 0, len(targets))
	for _, target := range targets {
		loaded, err := loadOperation(imports, listed, workspace, target)
		if err != nil {
			return nil, err
		}
		operations = append(operations, loaded)
	}
	return operations, nil
}

func (p prober) listPackages(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, targets []probeTarget) ([]listedPackage, error) {
	argv := append(append([]string{}, toolchain.Command...), listSubcommand, exportFlag, depsFlag, listFields)
	for _, target := range targets {
		if !slices.Contains(argv, target.ImportPath) {
			argv = append(argv, target.ImportPath)
		}
	}
	result, err := p.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: p.deps.Timeout})
	if err != nil {
		return nil, fmt.Errorf("list the packages of the operations: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return nil, fmt.Errorf("%w: go list: %s", ErrProbeFailed, strings.TrimSpace(string(result.Stderr)))
	}
	var listed []listedPackage
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return listed, nil
		} else if err != nil {
			return nil, fmt.Errorf("%w: unreadable go list output: %v", ErrProbeFailed, err)
		}
		if pkg.Error != nil {
			return nil, fmt.Errorf("%w: %s: %s", ErrProbeFailed, pkg.ImportPath, pkg.Error.Err)
		}
		listed = append(listed, pkg)
	}
}

func loadOperation(imports types.Importer, listed []listedPackage, workspace ecosystem.Workspace, target probeTarget) (operation, error) {
	pkg, err := imports.Import(target.ImportPath)
	if err != nil {
		return operation{}, fmt.Errorf("%w: %s: %v", ErrUnsupportedTarget, target.Text, err)
	}
	dir := directoryOf(listed, target.ImportPath)
	if relative, err := filepath.Rel(workspace.Dir, dir); err != nil || strings.HasPrefix(relative, "..") {
		return operation{}, fmt.Errorf("%w: %s is not in the project", ErrUnsupportedTarget, target.Text)
	}
	signature, err := signatureOf(pkg, target)
	if err != nil {
		return operation{}, err
	}
	loaded := operation{Target: target, Dir: dir, Sentinels: sentinelsOf(pkg)}
	loaded.ReturnsError = returnsError(signature)
	loaded.Context, loaded.Params = parametersOf(signature)
	return loaded, nil
}

func directoryOf(listed []listedPackage, importPath string) string {
	for _, pkg := range listed {
		if pkg.ImportPath == importPath {
			return pkg.Dir
		}
	}
	return ""
}

// signatureOf finds the function or method of the target.
func signatureOf(pkg *types.Package, target probeTarget) (*types.Signature, error) {
	member := pkg.Scope().Lookup(target.Name)
	if target.Receiver != "" {
		return methodSignature(pkg, target)
	}
	function, isFunction := member.(*types.Func)
	if !isFunction {
		return nil, fmt.Errorf("%w: %s is not a function of %s", ErrUnsupportedTarget, target.Name, target.ImportPath)
	}
	return checkedSignature(function, target)
}

func methodSignature(pkg *types.Package, target probeTarget) (*types.Signature, error) {
	declared, isType := pkg.Scope().Lookup(target.Receiver).(*types.TypeName)
	if !isType {
		return nil, fmt.Errorf("%w: %s is not a type of %s", ErrUnsupportedTarget, target.Receiver, target.ImportPath)
	}
	named, isNamed := declared.Type().(*types.Named)
	if !isNamed || named.TypeParams().Len() > 0 {
		return nil, fmt.Errorf("%w: %s is generic or not a defined type", ErrUnsupportedTarget, target.Text)
	}
	if _, isInterface := named.Underlying().(*types.Interface); isInterface {
		return nil, fmt.Errorf("%w: %s is an interface with no zero value to call", ErrUnsupportedTarget, target.Text)
	}
	method, _, _ := types.LookupFieldOrMethod(types.NewPointer(named), true, pkg, target.Name)
	function, isFunction := method.(*types.Func)
	if !isFunction {
		return nil, fmt.Errorf("%w: %s has no method %s", ErrUnsupportedTarget, target.Receiver, target.Name)
	}
	return checkedSignature(function, target)
}

func checkedSignature(function *types.Func, target probeTarget) (*types.Signature, error) {
	signature := function.Type().(*types.Signature)
	if signature.TypeParams().Len() > 0 {
		return nil, fmt.Errorf("%w: %s is generic", ErrUnsupportedTarget, target.Text)
	}
	return signature, nil
}

// parametersOf lists the parameters of the spec: a leading context.Context is
// supplied by the program and a variadic parameter is left out.
func parametersOf(signature *types.Signature) (bool, []probeharness.Param) {
	var params []probeharness.Param
	count := signature.Params().Len()
	if signature.Variadic() {
		count--
	}
	withContext := count > 0 && isContext(signature.Params().At(0).Type())
	first := 0
	if withContext {
		first = 1
	}
	for index := first; index < count; index++ {
		variable := signature.Params().At(index)
		kind, pointer := classify(variable.Type())
		params = append(params, probeharness.Param{Name: parameterName(variable.Name(), index-first), Kind: kind, Pointer: pointer})
	}
	return withContext, params
}

func returnsError(signature *types.Signature) bool {
	results := signature.Results()
	return results.Len() > 0 && types.Identical(results.At(results.Len()-1).Type(), types.Universe.Lookup(errorTypeName).Type())
}

// sentinelsOf lists the exported variables of the package that hold an error.
func sentinelsOf(pkg *types.Package) []string {
	errorInterface := types.Universe.Lookup(errorTypeName).Type().Underlying().(*types.Interface)
	var names []string
	for _, name := range pkg.Scope().Names() {
		variable, isVariable := pkg.Scope().Lookup(name).(*types.Var)
		if isVariable && variable.Exported() && types.Implements(variable.Type(), errorInterface) {
			names = append(names, name)
		}
	}
	return names
}

// visibilityRoot is the directory whose subtree may import the package at dir:
// the parent of the innermost `internal` directory under the workspace, else the
// workspace itself.
func visibilityRoot(workspace ecosystem.Workspace, dir string) string {
	relative, err := filepath.Rel(workspace.Dir, dir)
	if err != nil {
		return workspace.Dir
	}
	elements := strings.Split(filepath.ToSlash(relative), "/")
	for index := len(elements) - 1; index >= 0; index-- {
		if elements[index] == internalDir {
			return filepath.Join(append([]string{workspace.Dir}, elements[:index]...)...)
		}
	}
	return workspace.Dir
}

package jvm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm/probe"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/naming"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

// ErrInvalidTarget is returned for a bound operation that does not name a method
// of a class the project declares.
var ErrInvalidTarget = errors.New("invalid operation target")

const (
	targetSeparator = ":"
	kotlinFileClass = "Kt"

	requestExtension  = ".tsv"
	responseExtension = ".jsonl"
	sourceExtension   = ".java"

	javaHomeVariable = "JAVA_HOME"
	javaHomeBinary   = "bin"
	javaLauncher     = "java"

	// raisesArgument is what a call raises when its arguments do not fit the
	// parameters of its operation.
	raisesArgument = "ArgumentError"
)

// probeFlags run the probe program in a small, deterministic JVM: UTC times, an
// English locale and the bytecode of one thread of compilation.
var probeFlags = []string{
	"-Xmx256m", "-XX:+UseSerialGC", "-XX:TieredStopAtLevel=1", "-Xshare:auto",
	"-Duser.timezone=UTC", "-Duser.language=en", "-Duser.country=US", "-Dfile.encoding=UTF-8",
}

// skippedDirectories are never searched for the source of a class.
var skippedDirectories = []string{".git", ".gradle", ".kotlin", "build", "target", "node_modules"}

type prober struct {
	s *Strategy
}

// operation is a bound operation resolved against the project's sources.
type operation struct {
	target string
	class  string
	method string
	params []probe.Parameter
}

// names are the parameter names the spec writes the operation's arguments by.
func (o operation) names() []string {
	names := make([]string, len(o.params))
	for index, param := range o.params {
		names[index] = naming.SnakeCase(param.Name)
	}
	return names
}

// Signatures reads the parameters of each target from its source declaration.
func (p prober) Signatures(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	operations, err := resolveOperations(workspace, targets)
	if err != nil {
		return nil, err
	}
	signatures := make([]ports.ProbeSignature, 0, len(operations))
	for _, operation := range operations {
		params := make([]differential.Param, len(operation.params))
		for index, param := range operation.params {
			params[index] = differential.Param{Name: operation.names()[index], Hint: probe.HintOf(param.Type)}
		}
		signatures = append(signatures, ports.ProbeSignature{Target: operation.target, Params: params})
	}
	return signatures, nil
}

// Invoke observes what each call returns or throws, in order. A call that kills
// the probe program or outlives its deadline is an observation of its own, and
// the calls after it run in a fresh JVM, so no call hides another.
func (p prober) Invoke(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	var targets []string
	for _, call := range calls {
		if !slices.Contains(targets, call.Target) {
			targets = append(targets, call.Target)
		}
	}
	operations, err := resolveOperations(workspace, targets)
	if err != nil {
		return nil, err
	}
	observations := make([]spec.Observation, len(calls))
	var batch []probe.Call
	var batched []int
	for index, call := range calls {
		encoded, fits := encodeCall(operationFor(operations, call.Target), call)
		if !fits {
			observations[index] = spec.Observation{Kind: spec.Raises, Value: raisesArgument}
			continue
		}
		batch = append(batch, encoded)
		batched = append(batched, index)
	}
	answers, err := p.run(ctx, workspace, toolchain, batch)
	if err != nil {
		return nil, err
	}
	for position, index := range batched {
		observations[index] = answers[position]
	}
	return observations, nil
}

func operationFor(operations []operation, target string) operation {
	return operations[slices.IndexFunc(operations, func(o operation) bool { return o.target == target })]
}

// encodeCall puts the arguments of a call in parameter order; an argument the
// call leaves out stays nil. A call that names a parameter the operation does not
// have does not fit it.
func encodeCall(operation operation, call ports.ProbeCall) (probe.Call, bool) {
	names := operation.names()
	args := make([]*spec.Literal, len(names))
	for name, literal := range call.Args {
		position := slices.Index(names, name)
		if position < 0 {
			return probe.Call{}, false
		}
		args[position] = &literal
	}
	return probe.Call{Class: operation.class, Method: operation.method, Args: args}, true
}

func (p prober) run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, calls []probe.Call) ([]spec.Observation, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	classpath, err := p.s.classpath(ctx, workspace, toolchain)
	if err != nil {
		return nil, err
	}
	source := suiterun.ResultsFile(workspace, sourceExtension)
	if err := os.WriteFile(source, probe.Source(), privateFileMode); err != nil {
		return nil, fmt.Errorf("write the probe program: %w", err)
	}
	observed := make([]spec.Observation, 0, len(calls))
	for len(observed) < len(calls) {
		run, err := p.execute(ctx, workspace, classpath, source, calls[len(observed):])
		if err != nil {
			return nil, err
		}
		ended := run.endedCall()
		if len(run.observations)+len(ended) == 0 {
			return nil, fmt.Errorf("%w: the probe program answered nothing", ErrProbeFailed)
		}
		observed = append(observed, run.observations...)
		if len(observed) < len(calls) {
			observed = append(observed, ended...)
		}
	}
	return observed, nil
}

// probeRun is what one process of the probe program left behind.
type probeRun struct {
	observations []spec.Observation
	exitCode     int
	timedOut     bool
}

// endedCall is the observation of the call a process died or was killed in, which
// is the one after the calls it answered. A process that stopped by itself ended
// its last call by a timeout it already reported.
func (r probeRun) endedCall() []spec.Observation {
	switch {
	case r.timedOut:
		return []spec.Observation{{Kind: spec.Raises, Value: probe.RaisesTimeout}}
	case r.exitCode != probe.ExitClean:
		return []spec.Observation{{Kind: spec.Raises, Value: probe.RaisesCrash}}
	}
	return nil
}

func (p prober) execute(ctx context.Context, workspace ecosystem.Workspace, classpath, source string, calls []probe.Call) (probeRun, error) {
	requestFile := suiterun.ResultsFile(workspace, requestExtension)
	responseFile := suiterun.ResultsFile(workspace, responseExtension)
	if err := os.WriteFile(requestFile, probe.Encode(calls), privateFileMode); err != nil {
		return probeRun{}, fmt.Errorf("write the probe request: %w", err)
	}
	launcher, err := p.s.javaLauncher()
	if err != nil {
		return probeRun{}, err
	}
	argv := append(append(append([]string{}, launcher...), probeFlags...), "-cp", classpath, source, requestFile, responseFile)
	result, err := p.s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: ProbeTimeout})
	if err != nil {
		return probeRun{}, fmt.Errorf("run the probe program: %w", err)
	}
	if result.ExitCode == probe.ExitFailure {
		return probeRun{}, fmt.Errorf("%w: %s", ErrProbeFailed, strings.TrimSpace(string(result.Stderr)))
	}
	observations, err := readObservations(responseFile)
	if err != nil {
		return probeRun{}, err
	}
	if len(observations) == 0 && result.ExitCode != probe.ExitClean && !result.TimedOut && isLaunchFailure(result.Stderr) {
		return probeRun{}, fmt.Errorf("%w: %s", ErrProbeFailed, strings.TrimSpace(string(result.Stderr)))
	}
	if len(observations) > len(calls) {
		return probeRun{}, fmt.Errorf("%w: %d observations for %d calls", ErrProbeFailed, len(observations), len(calls))
	}
	return probeRun{observations: observations, exitCode: result.ExitCode, timedOut: result.TimedOut}, nil
}

// launchFailureMarkers are what the java launcher prints when it cannot compile or
// start the probe program at all, as opposed to the operation under test dying.
var launchFailureMarkers = []string{"error: ", "Error: Could not find or load main class", "Exception in thread \"main\" java.lang.NoClassDefFoundError: Probe"}

func isLaunchFailure(stderr []byte) bool {
	return slices.ContainsFunc(launchFailureMarkers, func(marker string) bool { return bytes.Contains(stderr, []byte(marker)) })
}

// javaLauncher is the command that starts a JVM: the configured one, the java of
// JAVA_HOME, then the java on PATH.
func (s *Strategy) javaLauncher() ([]string, error) {
	if len(s.deps.Java) > 0 {
		return slices.Clone(s.deps.Java), nil
	}
	if home := s.deps.Getenv(javaHomeVariable); home != "" {
		launcher := filepath.Join(home, javaHomeBinary, javaLauncher)
		if info, err := os.Stat(launcher); err == nil && !info.IsDir() {
			return []string{launcher}, nil
		}
	}
	found, err := s.deps.LookPath(javaLauncher)
	if err != nil {
		return nil, fmt.Errorf("%w: no java launcher found", ErrProbeFailed)
	}
	return []string{found}, nil
}

// readObservations reads the complete lines of the response file; a missing file
// holds none, and a line the process was killed in the middle of is dropped.
func readObservations(file string) ([]spec.Observation, error) {
	content, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the probe response: %w", err)
	}
	complete := content[:bytes.LastIndexByte(content, '\n')+1]
	var observations []spec.Observation
	for _, line := range bytes.Split(bytes.TrimSuffix(complete, []byte{'\n'}), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var observation spec.Observation
		if err := json.Unmarshal(line, &observation); err != nil {
			return nil, fmt.Errorf("%w: unreadable response line: %v", ErrProbeFailed, err)
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

// resolveOperations reads the declarations of the targets from the sources of the
// workspace.
func resolveOperations(workspace ecosystem.Workspace, targets []string) ([]operation, error) {
	operations := make([]operation, 0, len(targets))
	for _, target := range targets {
		resolved, err := resolveOperation(workspace.Dir, target)
		if err != nil {
			return nil, err
		}
		operations = append(operations, resolved)
	}
	return operations, nil
}

// resolveOperation parses `package.Class:method` (a nested class is written
// `package.Outer$Inner`) and reads the method's parameters from the source of the
// outermost class; a Kotlin file `Name.kt` holds its top-level functions in the
// class `NameKt`.
func resolveOperation(directory, target string) (operation, error) {
	separator := strings.LastIndex(target, targetSeparator)
	if separator <= 0 || separator == len(target)-len(targetSeparator) {
		return operation{}, fmt.Errorf("%w: %q is not `package.Class:method`", ErrInvalidTarget, target)
	}
	class, method := target[:separator], target[separator+len(targetSeparator):]
	file, classes, err := sourceOf(directory, class)
	if err != nil {
		return operation{}, fmt.Errorf("%w: %q: %v", ErrInvalidTarget, target, err)
	}
	content, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file)))
	if err != nil {
		return operation{}, fmt.Errorf("read %s: %w", file, err)
	}
	params, err := probe.Parameters(file, content, classes, method)
	if errors.Is(err, probe.ErrNoMethod) && path.Ext(file) == probe.KotlinExtension && len(classes) == 1 {
		params, err = probe.Parameters(file, content, nil, method)
	}
	if err != nil {
		return operation{}, fmt.Errorf("%w: %q: %v", ErrInvalidTarget, target, err)
	}
	return operation{target: target, class: class, method: method, params: params}, nil
}

// sourceOf finds the file that declares the outermost class of a binary class
// name, and the chain of class names down to the class itself.
func sourceOf(directory, class string) (file string, classes []string, err error) {
	outer, nested, _ := strings.Cut(class, nestedSeparator)
	dot := strings.LastIndex(outer, packageSeparator)
	packagePath := strings.ReplaceAll(outer[:max(dot, 0)], packageSeparator, "/")
	simple := outer[dot+1:]
	names := []string{simple + probe.JavaExtension, simple + probe.KotlinExtension}
	if stem, isFileClass := strings.CutSuffix(simple, kotlinFileClass); isFileClass {
		names = append(names, stem+probe.KotlinExtension)
	}
	file, err = findSource(directory, packagePath, names)
	if err != nil {
		return "", nil, err
	}
	classes = []string{simple}
	if nested != "" {
		classes = append(classes, strings.Split(nested, nestedSeparator)...)
	}
	return file, classes, nil
}

// findSource searches the workspace for a file with one of the names inside a
// directory that ends with the package path.
func findSource(directory, packagePath string, names []string) (string, error) {
	var found string
	err := fs.WalkDir(os.DirFS(directory), ".", func(entry string, info fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case info.IsDir() && slices.Contains(skippedDirectories, info.Name()):
			return fs.SkipDir
		case info.IsDir() || found != "":
			return nil
		}
		if slices.Contains(names, path.Base(entry)) && (packagePath == "" || strings.HasSuffix(path.Dir(entry), packagePath)) {
			found = entry
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no source declares %s", strings.Join(names, " or "))
	}
	return found, nil
}

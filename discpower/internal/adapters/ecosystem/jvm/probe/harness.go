// Package probe is the runtime side of the JVM prober: the Java program that
// calls the bound operations of a project with typed literals and reports what
// each call returned or threw, the wire format it speaks, and the reading of
// operation signatures from Java and Kotlin sources.
package probe

import (
	_ "embed"
	"time"
)

//go:embed Probe.java
var harnessSource []byte

// Source is the Java source of the probe program. The program is a single file
// that imports the JDK only, so it runs from source against the project's test
// classpath.
func Source() []byte { return append([]byte(nil), harnessSource...) }

const (
	// FileName is the name the probe program's source file is written under; it
	// names the public class the file holds.
	FileName = "Probe.java"

	// ExitClean is the exit status of a run that ended by itself, whether it
	// answered every call or stopped after a timeout.
	ExitClean = 0
	// ExitFailure is the exit status of a run that could not read its request. It
	// is distinct from the statuses a crashing operation leaves, so a call that
	// kills the process is not mistaken for a broken harness.
	ExitFailure = 70

	// CallTimeout bounds one call inside the probe program.
	CallTimeout = 5 * time.Second
)

// Names of what a call raises when it did not end in an exception of the project.
const (
	RaisesTimeout = "Timeout"
	RaisesCrash   = "Crash"
)

//go:build windows

package process

import "os/exec"

// isolateGroup keeps the default cancellation, which kills the process itself;
// Windows has no process group signal in the standard library.
func isolateGroup(*exec.Cmd) {}

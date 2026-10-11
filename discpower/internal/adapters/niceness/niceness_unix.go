//go:build unix

// Package niceness lowers the scheduling priority of the process, so that the
// test runs a scan starts, which inherit it, leave the machine responsive.
package niceness

import "syscall"

// currentProcess addresses the calling process in setpriority(2).
const currentProcess = 0

// Lower sets the niceness of the process to the given level. The system
// refuses to raise the priority of a process already nicer, which then keeps
// its niceness.
func Lower(level int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, currentProcess, level)
}

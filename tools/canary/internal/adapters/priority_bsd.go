//go:build darwin || freebsd || netbsd || openbsd

package adapters

import "syscall"

// currentNiceness is the niceness of the process, which getpriority returns as is.
func currentNiceness() (int, error) {
	return syscall.Getpriority(syscall.PRIO_PROCESS, 0)
}

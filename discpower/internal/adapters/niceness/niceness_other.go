//go:build !unix

package niceness

// Lower leaves the priority of the process as it is on a system without
// niceness.
func Lower(int) {}

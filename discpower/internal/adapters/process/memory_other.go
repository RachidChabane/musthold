//go:build !linux

package process

// preferAsMemoryVictim does nothing: only Linux lets a process volunteer its
// children to the out-of-memory killer.
func preferAsMemoryVictim(int) {}

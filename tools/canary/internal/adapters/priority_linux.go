package adapters

import "syscall"

// linuxPriorityBase is the offset of the raw getpriority value: the kernel
// returns 20 - niceness so that the value is never negative.
const linuxPriorityBase = 20

// currentNiceness is the niceness of the process.
func currentNiceness() (int, error) {
	raw, err := syscall.Getpriority(syscall.PRIO_PROCESS, 0)
	if err != nil {
		return 0, err
	}
	return linuxPriorityBase - raw, nil
}

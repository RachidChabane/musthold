// Package capacity reads what the machine has for a scan's runs: the
// processors, the memory and the share of it that is free, as the memory
// guard reads it (`memory_pressure` and `sysctl` on macOS, /proc/meminfo
// elsewhere).
package capacity

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

// ErrUnreadable is returned when the memory of the machine cannot be read.
var ErrUnreadable = errors.New("the memory of the machine cannot be read")

const (
	osDarwin              = "darwin"
	memoryPressureCommand = "memory_pressure"
	sysctlCommand         = "sysctl"
	sysctlValueFlag       = "-n"
	memorySizeKey         = "hw.memsize"
	memInfoPath           = "/proc/meminfo"
	memTotalField         = "MemTotal:"
	memAvailableField     = "MemAvailable:"
	kibibyte              = 1024
	percentBase           = 100
	decimalBase           = 10
	sizeBits              = 64
)

var freePattern = regexp.MustCompile(`System-wide memory free percentage:\s*(\d+)%`)

// Run gives the output of a command.
type Run func(ctx context.Context, name string, args ...string) ([]byte, error)

// Reader reads the capacity of this machine.
type Reader struct {
	// OS is the operating system, as runtime.GOOS names it.
	OS         string
	Processors int
	Run        Run
	// MemInfo reads /proc/meminfo.
	MemInfo func() ([]byte, error)
}

var _ ports.CapacityReader = Reader{}

// New reads the capacity of the machine the scan runs on.
func New() Reader {
	return Reader{
		OS: runtime.GOOS, Processors: runtime.NumCPU(),
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		MemInfo: func() ([]byte, error) { return os.ReadFile(memInfoPath) },
	}
}

// Capacity reads the processors and the memory.
func (r Reader) Capacity(ctx context.Context) (ports.Capacity, error) {
	capacity := ports.Capacity{Processors: r.Processors}
	var err error
	if r.OS == osDarwin {
		capacity.TotalMemory, capacity.FreePercent, err = r.darwinMemory(ctx)
	} else {
		capacity.TotalMemory, capacity.FreePercent, err = r.procMemory()
	}
	return capacity, err
}

func (r Reader) darwinMemory(ctx context.Context) (uint64, int, error) {
	size, err := r.Run(ctx, sysctlCommand, sysctlValueFlag, memorySizeKey)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(size)), decimalBase, sizeBits)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	pressure, err := r.Run(ctx, memoryPressureCommand)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	match := freePattern.FindSubmatch(pressure)
	if match == nil {
		return 0, 0, fmt.Errorf("%w: no free percentage in the %s output", ErrUnreadable, memoryPressureCommand)
	}
	free, err := strconv.Atoi(string(match[1]))
	if err != nil || free > percentBase {
		return 0, 0, fmt.Errorf("%w: %q is not a percentage", ErrUnreadable, match[1])
	}
	return total, free, nil
}

// procMemory reads the total and available memory of /proc/meminfo, in KiB.
func (r Reader) procMemory() (uint64, int, error) {
	content, err := r.MemInfo()
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	fields := map[string]uint64{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			if value, err := strconv.ParseUint(parts[1], decimalBase, sizeBits); err == nil {
				fields[parts[0]] = value
			}
		}
	}
	total, available := fields[memTotalField], fields[memAvailableField]
	if total == 0 {
		return 0, 0, fmt.Errorf("%w: no %s in %s", ErrUnreadable, memTotalField, memInfoPath)
	}
	return total * kibibyte, int(available * percentBase / total), nil
}

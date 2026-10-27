// Package capacity reads what the machine has for a scan's runs: the
// processors, the memory and the share of it that is free, as the memory
// guard reads it (`memory_pressure` and `sysctl` on macOS, /proc/meminfo
// elsewhere), within the memory limit of the control group the scan runs in
// when one bounds it below the machine's.
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

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
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

// cgroupFiles are the files of one version of the control groups that give
// the memory limit of the group, the memory it uses and the statistics whose
// inactive file cache it can reclaim.
type cgroupFiles struct {
	limit, usage, stat, inactiveFile string
}

// cgroupVersions are the control group files a scan reads, version 2 first.
var cgroupVersions = []cgroupFiles{
	{limit: "/sys/fs/cgroup/memory.max", usage: "/sys/fs/cgroup/memory.current", stat: "/sys/fs/cgroup/memory.stat", inactiveFile: "inactive_file"},
	{limit: "/sys/fs/cgroup/memory/memory.limit_in_bytes", usage: "/sys/fs/cgroup/memory/memory.usage_in_bytes", stat: "/sys/fs/cgroup/memory/memory.stat", inactiveFile: "total_inactive_file"},
}

var freePattern = regexp.MustCompile(`System-wide memory free percentage:\s*(\d+)%`)

// Run gives the output of a command.
type Run func(ctx context.Context, name string, args ...string) ([]byte, error)

// Reader reads the capacity of this machine.
type Reader struct {
	// OS is the operating system, as runtime.GOOS names it.
	OS         string
	Processors int
	Run        Run
	// ReadFile reads /proc/meminfo and the files of the control groups.
	ReadFile func(name string) ([]byte, error)
}

var _ ports.CapacityReader = Reader{}

// New reads the capacity of the machine the scan runs on.
func New() Reader {
	return Reader{
		OS: runtime.GOOS, Processors: runtime.NumCPU(),
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		ReadFile: os.ReadFile,
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

// procMemory reads the total and available memory of /proc/meminfo, in KiB,
// and gives in their place the limit of the control group and what it leaves
// available when the group bounds the memory below the machine's.
func (r Reader) procMemory() (uint64, int, error) {
	content, err := r.ReadFile(memInfoPath)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	fields := fieldsOf(content)
	total, available := fields[memTotalField]*kibibyte, fields[memAvailableField]*kibibyte
	if total == 0 {
		return 0, 0, fmt.Errorf("%w: no %s in %s", ErrUnreadable, memTotalField, memInfoPath)
	}
	if limit, left, bounded := r.cgroupMemory(); bounded && limit < total {
		total, available = limit, min(left, available)
	}
	return total, int(available * percentBase / total), nil
}

// cgroupMemory reads the memory limit of the control group and what it leaves
// available: the limit less the group's usage, the inactive file cache it can
// reclaim aside; false when no control group bounds the memory.
func (r Reader) cgroupMemory() (uint64, uint64, bool) {
	for _, files := range cgroupVersions {
		limit, limited := r.number(files.limit)
		usage, used := r.number(files.usage)
		if !limited || !used || limit == 0 {
			continue
		}
		if stat, err := r.ReadFile(files.stat); err == nil {
			usage -= min(usage, fieldsOf(stat)[files.inactiveFile])
		}
		return limit, limit - min(limit, usage), true
	}
	return 0, 0, false
}

// number reads a file that holds one number; false when it holds none, as
// `max` stands for no limit.
func (r Reader) number(name string) (uint64, bool) {
	content, err := r.ReadFile(name)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(content)), decimalBase, sizeBits)
	return value, err == nil
}

// fieldsOf reads the lines `name value` of a statistics file into numbers by
// name.
func fieldsOf(content []byte) map[string]uint64 {
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
	return fields
}

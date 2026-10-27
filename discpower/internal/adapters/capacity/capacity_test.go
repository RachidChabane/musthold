package capacity_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/capacity"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

const verifiesCapacity = " [verifies SP-2130~2]"

const gibibyte = 1 << 30

// hostMemInfo is the /proc/meminfo of a machine of 16 GB, half of it
// available.
const hostMemInfo = "MemTotal:       16000000 kB\nMemFree:         1000000 kB\nMemAvailable:    8000000 kB\n"

// files reads the given files; any other is missing.
func files(contents map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		content, found := contents[name]
		if !found {
			return nil, fs.ErrNotExist
		}
		return []byte(content), nil
	}
}

func TestCapacity(t *testing.T) {
	t.Run("on macOS the memory size and the free share come from sysctl and memory_pressure"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "darwin", Processors: 8, Run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "sysctl" {
				return []byte("25769803776\n"), nil
			}
			return []byte("The system has 25769803776 (6291456 pages with a page size of 4096).\nSystem-wide memory free percentage: 72%\n"), nil
		}}
		got, err := reader.Capacity(context.Background())
		if err != nil || got != (ports.Capacity{Processors: 8, TotalMemory: 24 * gibibyte, FreePercent: 72}) {
			t.Fatalf("capacity %+v, %v", got, err)
		}
	})
	t.Run("elsewhere the memory comes from /proc/meminfo"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "linux", Processors: 4, ReadFile: files(map[string]string{"/proc/meminfo": hostMemInfo})}
		got, err := reader.Capacity(context.Background())
		if err != nil || got != (ports.Capacity{Processors: 4, TotalMemory: 16000000 * 1024, FreePercent: 50}) {
			t.Fatalf("capacity %+v, %v", got, err)
		}
	})
	t.Run("in a control group of version 2 whose limit is below the machine's, the memory is the limit and the free share what the group leaves, its inactive file cache reclaimable"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "linux", Processors: 4, ReadFile: files(map[string]string{
			"/proc/meminfo":                 hostMemInfo,
			"/sys/fs/cgroup/memory.max":     "2684354560\n",
			"/sys/fs/cgroup/memory.current": "1610612736\n",
			"/sys/fs/cgroup/memory.stat":    "anon 805306368\nfile 805306368\ninactive_file 268435456\n",
		})}
		got, err := reader.Capacity(context.Background())
		if err != nil || got != (ports.Capacity{Processors: 4, TotalMemory: 2684354560, FreePercent: 50}) {
			t.Fatalf("capacity %+v, %v", got, err)
		}
	})
	t.Run("in a control group of version 1 the limit and the usage come from its memory controller"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "linux", Processors: 4, ReadFile: files(map[string]string{
			"/proc/meminfo": hostMemInfo,
			"/sys/fs/cgroup/memory/memory.limit_in_bytes": "4294967296\n",
			"/sys/fs/cgroup/memory/memory.usage_in_bytes": "3221225472\n",
		})}
		got, err := reader.Capacity(context.Background())
		if err != nil || got != (ports.Capacity{Processors: 4, TotalMemory: 4294967296, FreePercent: 25}) {
			t.Fatalf("capacity %+v, %v", got, err)
		}
	})
	t.Run("a control group without a limit, or one above the machine's memory, leaves /proc/meminfo's"+verifiesCapacity, func(t *testing.T) {
		for name, limit := range map[string]map[string]string{
			"no limit":     {"/sys/fs/cgroup/memory.max": "max\n", "/sys/fs/cgroup/memory.current": "1610612736\n"},
			"above it":     {"/sys/fs/cgroup/memory/memory.limit_in_bytes": "9223372036854771712\n", "/sys/fs/cgroup/memory/memory.usage_in_bytes": "1610612736\n"},
			"no cgroup fs": {},
		} {
			limit["/proc/meminfo"] = hostMemInfo
			got, err := capacity.Reader{OS: "linux", Processors: 4, ReadFile: files(limit)}.Capacity(context.Background())
			if err != nil || got != (ports.Capacity{Processors: 4, TotalMemory: 16000000 * 1024, FreePercent: 50}) {
				t.Errorf("%s: capacity %+v, %v", name, got, err)
			}
		}
	})
	t.Run("a memory that cannot be read is an error"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "darwin", Run: func(context.Context, string, ...string) ([]byte, error) { return []byte("nothing"), nil }}
		if _, err := reader.Capacity(context.Background()); !errors.Is(err, capacity.ErrUnreadable) {
			t.Fatalf("error %v", err)
		}
	})
}

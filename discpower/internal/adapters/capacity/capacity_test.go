package capacity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/capacity"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

const verifiesCapacity = " [verifies SP-2130~1]"

const gibibyte = 1 << 30

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
		reader := capacity.Reader{OS: "linux", Processors: 4, MemInfo: func() ([]byte, error) {
			return []byte("MemTotal:       16000000 kB\nMemFree:         1000000 kB\nMemAvailable:    8000000 kB\n"), nil
		}}
		got, err := reader.Capacity(context.Background())
		if err != nil || got != (ports.Capacity{Processors: 4, TotalMemory: 16000000 * 1024, FreePercent: 50}) {
			t.Fatalf("capacity %+v, %v", got, err)
		}
	})
	t.Run("a memory that cannot be read is an error"+verifiesCapacity, func(t *testing.T) {
		reader := capacity.Reader{OS: "darwin", Run: func(context.Context, string, ...string) ([]byte, error) { return []byte("nothing"), nil }}
		if _, err := reader.Capacity(context.Background()); !errors.Is(err, capacity.ErrUnreadable) {
			t.Fatalf("error %v", err)
		}
	})
}

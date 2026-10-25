package workerpool_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/platform/workerpool"
)

const verifiesWorkers = " [verifies SP-2299~1]"

func TestRun(t *testing.T) {
	t.Run("every job runs once, on at most the given number of workers"+verifiesWorkers, func(t *testing.T) {
		var mutex sync.Mutex
		done := map[int]int{}
		var running, peak atomic.Int32
		err := workerpool.Run(context.Background(), 2, 9, func(_ context.Context, index int) error {
			now := running.Add(1)
			for {
				seen := peak.Load()
				if now <= seen || peak.CompareAndSwap(seen, now) {
					break
				}
			}
			mutex.Lock()
			done[index]++
			mutex.Unlock()
			running.Add(-1)
			return nil
		})
		if err != nil || len(done) != 9 || peak.Load() > 2 {
			t.Fatalf("err %v, done %v, peak %d", err, done, peak.Load())
		}
		for index, times := range done {
			if times != 1 {
				t.Fatalf("job %d ran %d times", index, times)
			}
		}
	})
	t.Run("no job starts after the first error, which is returned, and below one worker one runs"+verifiesWorkers, func(t *testing.T) {
		failure := errors.New("the recording failed")
		var started atomic.Int32
		err := workerpool.Run(context.Background(), 0, 5, func(_ context.Context, index int) error {
			started.Add(1)
			if index == 1 {
				return failure
			}
			return nil
		})
		if !errors.Is(err, failure) || started.Load() != 2 {
			t.Fatalf("err %v, %d started", err, started.Load())
		}
	})
}

// Package workerpool runs indexed jobs on a bounded number of goroutines.
package workerpool

import (
	"context"
	"sync"
)

// Run does each of count jobs, by index, on at most the given number of
// workers, at least one; it starts no job after the first error and returns
// that error once every started job has ended.
func Run(ctx context.Context, workers, count int, do func(ctx context.Context, index int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mutex sync.Mutex
	var failure error
	jobs := make(chan int)
	var wait sync.WaitGroup
	for range max(workers, 1) {
		wait.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if err := do(ctx, index); err != nil {
					mutex.Lock()
					if failure == nil {
						failure = err
						cancel()
					}
					mutex.Unlock()
				}
			}
		})
	}
	for index := range count {
		if ctx.Err() != nil {
			break
		}
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	return failure
}

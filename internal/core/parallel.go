package core

import (
	"context"
	"sync"
)

// ParallelMap executes fn for each input in parallel with bounded concurrency.
// Results are collected into a map keyed by the input. If fn returns an error
// or nil result, that input is omitted from the results.
func ParallelMap[K comparable, V any](
	ctx context.Context,
	inputs []K,
	concurrency int,
	fn func(ctx context.Context, input K) (*V, error),
) map[K]*V {
	results := make(map[K]*V)
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, input := range inputs {
		wg.Add(1)
		go func(k K) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			result, err := fn(ctx, k)
			if err == nil && result != nil {
				mu.Lock()
				results[k] = result
				mu.Unlock()
			}
		}(input)
	}

	wg.Wait()
	return results
}

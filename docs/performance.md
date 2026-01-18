# Performance

Benchmark results and optimization notes.

## Benchmark Results

Tested on Apple M1 Pro with mock HTTP server.

### Registry Operations

| Operation | Time | Memory | Allocs |
|-----------|------|--------|--------|
| `New()` | 101 ns | 161 B | 4 |
| `DefaultURL()` | 14 ns | 0 B | 0 |
| `SupportedEcosystems()` | 256 ns | 352 B | 1 |
| URL builder (3 calls) | 362 ns | 264 B | 11 |
| Create 9 registries | 954 ns | 1.4 KB | 36 |

### HTTP Operations

| Operation | Time | Memory | Allocs |
|-----------|------|--------|--------|
| FetchPackage (Cargo) | 68 µs | 13 KB | 179 |
| FetchPackage (npm) | 63 µs | 12 KB | 164 |
| FetchVersions (Cargo) | 89 µs | 14 KB | 186 |
| FetchPackage (parallel) | 47 µs | 17 KB | 198 |
| GetJSON (raw client) | 57 µs | 9.5 KB | 126 |
| GetBody (raw client) | 52 µs | 7.3 KB | 79 |

### JSON Parsing

| Payload Size | Time | Memory | Allocs |
|--------------|------|--------|--------|
| Small (~500B) | 5.2 µs | 3 KB | 92 |
| Large (500 versions) | 508 µs | 488 KB | 9,061 |

## CPU Profile Breakdown

For a typical `FetchPackage` call:

| Component | % Time |
|-----------|--------|
| syscall (HTTP) | 36% |
| pthread_cond_signal | 19% |
| kevent (I/O) | 18% |
| pthread_cond_wait | 11% |
| JSON encode/decode | <3% |

The HTTP round-trip dominates. JSON parsing is not a bottleneck.

## Memory Allocation Breakdown

| Component | % Allocations |
|-----------|---------------|
| HTTP headers | 8.5% |
| MIME header parsing | 8.4% |
| io.ReadAll | 7.6% |
| JSON map encoding | 7% |
| FetchPackage (total) | ~29% |

## Optimization Opportunities

### Already Optimized

1. **Connection reuse:** The default `http.Client` reuses TCP connections
2. **Lazy initialization:** Registries created on-demand, not at import
3. **Minimal allocations:** URL builders use `fmt.Sprintf` (efficient for this use case)

### Possible Future Optimizations

1. **Response caching:** Cache `FetchPackage` results with TTL
   ```go
   type CachedRegistry struct {
       Registry
       cache *lru.Cache
       ttl   time.Duration
   }
   ```

2. **Parallel version fetching:** Some registries require multiple requests
   ```go
   func (r *Registry) FetchVersions(ctx context.Context, name string) ([]Version, error) {
       // Fetch list, then fan out for details
       g, ctx := errgroup.WithContext(ctx)
       // ...
   }
   ```

3. **JSON streaming:** For very large responses, use `json.Decoder`
   ```go
   decoder := json.NewDecoder(resp.Body)
   for decoder.More() {
       // Process incrementally
   }
   ```

4. **Struct field caching:** Pre-allocate structs for hot paths

### Not Worth Optimizing

1. **String building in URL methods:** Already fast (362ns for 3 URLs)
2. **Registry lookup:** Map access is O(1), ~100ns
3. **JSON unmarshaling:** Standard library is already well-optimized

## Running Benchmarks

```bash
# All benchmarks
go test -bench=. -benchmem ./...

# Specific benchmark with count
go test -bench=BenchmarkFetchPackage -benchmem -count=5 .

# With CPU profile
go test -bench=BenchmarkFetchPackage -cpuprofile=cpu.prof .
go tool pprof -top cpu.prof

# With memory profile
go test -bench=BenchmarkFetchPackage -memprofile=mem.prof .
go tool pprof -top -alloc_space mem.prof

# Compare before/after
go test -bench=. -count=10 . > old.txt
# make changes
go test -bench=. -count=10 . > new.txt
benchstat old.txt new.txt
```

## Real-World Performance

Mock server benchmarks show library overhead. Real performance depends on:

1. **Network latency:** 50-200ms to registry APIs
2. **Registry response time:** Varies by load
3. **Response size:** npm packages can have 1000+ versions
4. **Rate limiting:** Some registries throttle requests

For bulk operations, consider:
- Parallel requests with worker pool
- Request batching where API supports it
- Local caching layer
- Rate limiter to avoid 429s

## Memory Usage

A typical `FetchPackage` call allocates ~13KB. For processing many packages:

```go
// Process 1000 packages
// Memory: ~13MB peak (with GC)

// With pooling (if needed):
var pkgPool = sync.Pool{
    New: func() interface{} {
        return &core.Package{}
    },
}
```

In practice, Go's GC handles this well. Only pool if profiling shows GC pressure.

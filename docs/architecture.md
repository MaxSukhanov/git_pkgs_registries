# Architecture

The registries library uses a plugin-style architecture where each ecosystem registers itself at init time. The public API in `registries.go` delegates to internal implementations.

## Package Structure

```
registries/
├── registries.go          # Public API, re-exports from internal/core
├── benchmark_test.go      # Performance benchmarks
├── all/
│   └── all.go             # Convenience import for all ecosystems
├── internal/
│   ├── core/
│   │   ├── registry.go    # Registration system, Registry interface
│   │   ├── types.go       # Package, Version, Dependency, Maintainer
│   │   ├── client.go      # HTTP client with retry logic
│   │   └── errors.go      # HTTPError, NotFoundError
│   ├── cargo/
│   │   ├── cargo.go       # Cargo implementation
│   │   └── cargo_test.go
│   ├── npm/
│   │   ├── npm.go
│   │   └── npm_test.go
│   └── ...                # Other ecosystems
└── docs/
    └── ...
```

## Registration System

Each ecosystem package registers itself using an `init()` function:

```go
// internal/cargo/cargo.go
func init() {
    core.Register(ecosystem, DefaultURL, func(baseURL string, client *core.Client) core.Registry {
        return New(baseURL, client)
    })
}
```

The `core.Register` function stores a factory in a global map:

```go
// internal/core/registry.go
var (
    registries = make(map[string]registryEntry)
    mu         sync.RWMutex
)

type registryEntry struct {
    defaultURL string
    factory    RegistryFactory
}

func Register(ecosystem, defaultURL string, factory RegistryFactory) {
    mu.Lock()
    defer mu.Unlock()
    registries[ecosystem] = registryEntry{defaultURL: defaultURL, factory: factory}
}
```

When a user calls `registries.New("cargo", "", client)`, the public API looks up the factory and invokes it:

```go
// registries.go
func New(ecosystem, baseURL string, client *Client) (Registry, error) {
    return core.New(ecosystem, baseURL, client)
}

// internal/core/registry.go
func New(ecosystem, baseURL string, client *Client) (Registry, error) {
    mu.RLock()
    entry, ok := registries[ecosystem]
    mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("unknown ecosystem: %s", ecosystem)
    }

    if baseURL == "" {
        baseURL = entry.defaultURL
    }
    if client == nil {
        client = DefaultClient()
    }

    return entry.factory(baseURL, client), nil
}
```

## Import Side Effects

Users must import ecosystem packages for their `init()` functions to run:

```go
import (
    "github.com/git-pkgs/registries"
    _ "github.com/git-pkgs/registries/internal/cargo"  // Registers "cargo"
)
```

Or import all ecosystems at once:

```go
import (
    "github.com/git-pkgs/registries"
    _ "github.com/git-pkgs/registries/all"
)
```

The `all` package just has blank imports:

```go
// all/all.go
import (
    _ "github.com/git-pkgs/registries/internal/cargo"
    _ "github.com/git-pkgs/registries/internal/npm"
    // ... all other ecosystems
)
```

## Data Flow

```
User Code
    │
    ▼
registries.New("cargo", "", client)
    │
    ▼
core.New() ─────► looks up factory in registries map
    │
    ▼
cargo.New(baseURL, client) ─────► returns *cargo.Registry
    │
    ▼
Registry interface returned to user
    │
    ▼
reg.FetchPackage(ctx, "serde")
    │
    ▼
cargo.Registry.FetchPackage()
    │
    ▼
client.GetJSON() ─────► HTTP GET with retries
    │
    ▼
Parse JSON into cargo-specific structs
    │
    ▼
Convert to core.Package and return
```

## Interface Satisfaction

Each ecosystem's Registry struct must implement `core.Registry`:

```go
type Registry interface {
    Ecosystem() string
    FetchPackage(ctx context.Context, name string) (*Package, error)
    FetchVersions(ctx context.Context, name string) ([]Version, error)
    FetchDependencies(ctx context.Context, name, version string) ([]Dependency, error)
    FetchMaintainers(ctx context.Context, name string) ([]Maintainer, error)
    URLs() URLBuilder
}
```

And a URLs struct implementing `core.URLBuilder`:

```go
type URLBuilder interface {
    Registry(name, version string) string
    Download(name, version string) string
    Documentation(name, version string) string
    PURL(name, version string) string
}
```

## Thread Safety

The registration map uses a `sync.RWMutex` for thread-safe reads and writes. In practice, all registrations happen during `init()` before any goroutines are spawned, so contention is minimal.

The HTTP client is safe for concurrent use. Each `FetchX` method creates its own request and can be called from multiple goroutines simultaneously.

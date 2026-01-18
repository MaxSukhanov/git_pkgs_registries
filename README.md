# registries

Go library for fetching package metadata from registry APIs. Supports 25 ecosystems including npm, PyPI, Cargo, RubyGems, Go modules, Maven, NuGet, Packagist, Hex, Pub, CocoaPods, Clojars, CPAN, Hackage, CRAN, Conda, Julia, Elm, Dub, LuaRocks, Nimble, Haxelib, Homebrew, Deno, and Terraform with a unified interface.

## Installation

```bash
go get github.com/git-pkgs/registries
```

## Usage

Import the ecosystems you need, then create a registry client:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/git-pkgs/registries"
    _ "github.com/git-pkgs/registries/internal/cargo"
)

func main() {
    reg, err := registries.New("cargo", "", nil)
    if err != nil {
        log.Fatal(err)
    }

    pkg, err := reg.FetchPackage(context.Background(), "serde")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(pkg.Name)        // serde
    fmt.Println(pkg.Repository)  // https://github.com/serde-rs/serde
    fmt.Println(pkg.Licenses)    // MIT OR Apache-2.0
}
```

To import all ecosystems at once:

```go
import (
    "github.com/git-pkgs/registries"
    _ "github.com/git-pkgs/registries/all"
)
```

## Supported Ecosystems

| Ecosystem | PURL Type | Default Registry |
|-----------|-----------|------------------|
| Cargo | `cargo` | https://crates.io |
| npm | `npm` | https://registry.npmjs.org |
| RubyGems | `gem` | https://rubygems.org |
| PyPI | `pypi` | https://pypi.org |
| Go | `golang` | https://proxy.golang.org |
| Maven | `maven` | https://repo1.maven.org/maven2 |
| NuGet | `nuget` | https://api.nuget.org/v3 |
| Packagist | `composer` | https://packagist.org |
| Hex | `hex` | https://hex.pm |
| Pub | `pub` | https://pub.dev |
| CocoaPods | `cocoapods` | https://trunk.cocoapods.org |
| Clojars | `clojars` | https://clojars.org |
| CPAN | `cpan` | https://fastapi.metacpan.org |
| Hackage | `hackage` | https://hackage.haskell.org |
| CRAN | `cran` | https://cran.r-project.org |
| Conda | `conda` | https://api.anaconda.org |
| Julia | `julia` | https://raw.githubusercontent.com/JuliaRegistries/General/master |
| Elm | `elm` | https://package.elm-lang.org |
| Dub | `dub` | https://code.dlang.org |
| LuaRocks | `luarocks` | https://luarocks.org |
| Nimble | `nimble` | https://nimble.directory |
| Haxelib | `haxelib` | https://lib.haxe.org |
| Homebrew | `brew` | https://formulae.brew.sh |
| Deno | `deno` | https://apiland.deno.dev |
| Terraform | `terraform` | https://registry.terraform.io |

## API

### Creating a Registry

```go
// Use default registry URL and client
reg, err := registries.New("npm", "", nil)

// Use custom registry URL
reg, err := registries.New("npm", "https://npm.pkg.github.com", nil)

// Use custom client with options
client := registries.NewClient(
    registries.WithTimeout(60 * time.Second),
    registries.WithMaxRetries(3),
)
reg, err := registries.New("npm", "", client)
```

### Fetching Package Metadata

```go
pkg, err := reg.FetchPackage(ctx, "lodash")

// Package fields:
// - Name        string
// - Description string
// - Homepage    string
// - Repository  string
// - Licenses    string
// - Keywords    []string
// - Namespace   string         (e.g., "@babel" for npm scoped packages)
// - Metadata    map[string]any (registry-specific data)
```

### Fetching Versions

```go
versions, err := reg.FetchVersions(ctx, "lodash")

for _, v := range versions {
    fmt.Printf("%s published %s\n", v.Number, v.PublishedAt)

    if v.Status == registries.StatusYanked {
        fmt.Println("  (yanked)")
    }
}

// Version fields:
// - Number      string
// - PublishedAt time.Time
// - Licenses    string
// - Integrity   string        (e.g., "sha256-...")
// - Status      VersionStatus ("", "yanked", "deprecated", "retracted")
// - Metadata    map[string]any
```

### Fetching Dependencies

```go
deps, err := reg.FetchDependencies(ctx, "express", "4.19.0")

for _, d := range deps {
    fmt.Printf("%s %s (%s)\n", d.Name, d.Requirements, d.Scope)
}

// Dependency fields:
// - Name         string
// - Requirements string
// - Scope        Scope (runtime, development, test, build, optional)
// - Optional     bool
```

### Fetching Maintainers

```go
maintainers, err := reg.FetchMaintainers(ctx, "rails")

for _, m := range maintainers {
    fmt.Printf("%s <%s>\n", m.Login, m.Email)
}

// Maintainer fields:
// - UUID, Login, Name, Email, URL, Role string
```

### URL Builder

Each registry provides URLs for packages:

```go
urls := reg.URLs()

urls.Registry("serde", "1.0.0")      // https://crates.io/crates/serde/1.0.0
urls.Download("serde", "1.0.0")      // https://static.crates.io/crates/serde/serde-1.0.0.crate
urls.Documentation("serde", "1.0.0") // https://docs.rs/serde/1.0.0
urls.PURL("serde", "1.0.0")          // pkg:cargo/serde@1.0.0
```

## Error Handling

```go
pkg, err := reg.FetchPackage(ctx, "nonexistent")
if err != nil {
    var notFound *registries.NotFoundError
    if errors.As(err, &notFound) {
        fmt.Printf("Package %s not found in %s\n", notFound.Name, notFound.Ecosystem)
    }
}
```

## HTTP Client

The default client includes:

- 30 second timeout
- 5 retries with exponential backoff (50ms base)
- Automatic retry on 429 and 5xx responses

You can provide a custom rate limiter:

```go
type myLimiter struct{}

func (l *myLimiter) Wait(ctx context.Context) error {
    // implement rate limiting
    return nil
}

client := registries.DefaultClient()
client = client.WithRateLimiter(&myLimiter{})
```

## Scoped Packages

npm scoped packages work as expected:

```go
pkg, _ := reg.FetchPackage(ctx, "@babel/core")
fmt.Println(pkg.Namespace) // "babel"
fmt.Println(reg.URLs().PURL("@babel/core", "7.24.0")) // pkg:npm/@babel/core@7.24.0
```

## PyPI Name Normalization

PyPI package names are normalized according to PEP 503:

```go
// These all resolve to the same package:
reg.FetchPackage(ctx, "typing-extensions")
reg.FetchPackage(ctx, "typing_extensions")
reg.FetchPackage(ctx, "Typing-Extensions")
```

## Go Module Proxy Encoding

Capital letters in Go module paths are encoded per the goproxy protocol:

```go
// github.com/Azure/go-sdk becomes github.com/!azure/go-sdk in URLs
urls.Download("github.com/Azure/go-sdk", "v1.0.0")
// https://proxy.golang.org/github.com/!azure/go-sdk/@v/v1.0.0.zip
```

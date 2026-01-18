# Adding a New Registry

This guide walks through adding support for a new package ecosystem.

## 1. Create the Package

Create a new directory under `internal/`:

```
internal/
└── myregistry/
    ├── myregistry.go
    └── myregistry_test.go
```

## 2. Define Constants

```go
package myregistry

import (
    "context"
    "fmt"
    "strings"

    "github.com/git-pkgs/registries/internal/core"
)

const (
    DefaultURL = "https://api.myregistry.org"
    ecosystem  = "myregistry"  // Used in PURL: pkg:myregistry/name@version
)
```

## 3. Register at Init

```go
func init() {
    core.Register(ecosystem, DefaultURL, func(baseURL string, client *core.Client) core.Registry {
        return New(baseURL, client)
    })
}
```

## 4. Define the Registry Struct

```go
type Registry struct {
    baseURL string
    client  *core.Client
    urls    *URLs
}

func New(baseURL string, client *core.Client) *Registry {
    if baseURL == "" {
        baseURL = DefaultURL
    }
    r := &Registry{
        baseURL: strings.TrimSuffix(baseURL, "/"),
        client:  client,
    }
    r.urls = &URLs{baseURL: r.baseURL}
    return r
}

func (r *Registry) Ecosystem() string {
    return ecosystem
}

func (r *Registry) URLs() core.URLBuilder {
    return r.urls
}
```

## 5. Define API Response Structs

Map the registry's JSON API to Go structs:

```go
type packageResponse struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Homepage    string   `json:"homepage"`
    Repository  string   `json:"repository"`
    License     string   `json:"license"`
    Keywords    []string `json:"keywords"`
    Versions    []versionInfo `json:"versions"`
}

type versionInfo struct {
    Number       string            `json:"version"`
    PublishedAt  string            `json:"published_at"`
    Dependencies map[string]string `json:"dependencies"`
}
```

## 6. Implement FetchPackage

```go
func (r *Registry) FetchPackage(ctx context.Context, name string) (*core.Package, error) {
    url := fmt.Sprintf("%s/packages/%s", r.baseURL, name)

    var resp packageResponse
    if err := r.client.GetJSON(ctx, url, &resp); err != nil {
        if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
            return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name}
        }
        return nil, err
    }

    return &core.Package{
        Name:        resp.Name,
        Description: resp.Description,
        Homepage:    resp.Homepage,
        Repository:  resp.Repository,
        Licenses:    resp.License,
        Keywords:    resp.Keywords,
    }, nil
}
```

## 7. Implement FetchVersions

```go
func (r *Registry) FetchVersions(ctx context.Context, name string) ([]core.Version, error) {
    url := fmt.Sprintf("%s/packages/%s", r.baseURL, name)

    var resp packageResponse
    if err := r.client.GetJSON(ctx, url, &resp); err != nil {
        if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
            return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name}
        }
        return nil, err
    }

    versions := make([]core.Version, 0, len(resp.Versions))
    for _, v := range resp.Versions {
        var publishedAt time.Time
        if v.PublishedAt != "" {
            publishedAt, _ = time.Parse(time.RFC3339, v.PublishedAt)
        }

        versions = append(versions, core.Version{
            Number:      v.Number,
            PublishedAt: publishedAt,
            Licenses:    resp.License,
        })
    }

    return versions, nil
}
```

## 8. Implement FetchDependencies

```go
func (r *Registry) FetchDependencies(ctx context.Context, name, version string) ([]core.Dependency, error) {
    url := fmt.Sprintf("%s/packages/%s/versions/%s", r.baseURL, name, version)

    var resp versionInfo
    if err := r.client.GetJSON(ctx, url, &resp); err != nil {
        if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
            return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name, Version: version}
        }
        return nil, err
    }

    var deps []core.Dependency
    for depName, constraint := range resp.Dependencies {
        deps = append(deps, core.Dependency{
            Name:         depName,
            Requirements: constraint,
            Scope:        core.Runtime,
        })
    }

    // Sort for consistent output
    sort.Slice(deps, func(i, j int) bool {
        return deps[i].Name < deps[j].Name
    })

    return deps, nil
}
```

## 9. Implement FetchMaintainers

```go
func (r *Registry) FetchMaintainers(ctx context.Context, name string) ([]core.Maintainer, error) {
    // If the API doesn't expose maintainers, return nil
    // Otherwise fetch and convert to []core.Maintainer

    return nil, nil
}
```

## 10. Implement URLBuilder

```go
type URLs struct {
    baseURL string
}

func (u *URLs) Registry(name, version string) string {
    if version != "" {
        return fmt.Sprintf("%s/packages/%s/%s", u.baseURL, name, version)
    }
    return fmt.Sprintf("%s/packages/%s", u.baseURL, name)
}

func (u *URLs) Download(name, version string) string {
    if version == "" {
        return ""
    }
    return fmt.Sprintf("%s/packages/%s/%s/download", u.baseURL, name, version)
}

func (u *URLs) Documentation(name, version string) string {
    return fmt.Sprintf("%s/packages/%s/docs", u.baseURL, name)
}

func (u *URLs) PURL(name, version string) string {
    if version != "" {
        return fmt.Sprintf("pkg:%s/%s@%s", ecosystem, name, version)
    }
    return fmt.Sprintf("pkg:%s/%s", ecosystem, name)
}
```

## 11. Write Tests

Use `httptest.NewServer` to mock API responses:

```go
func TestFetchPackage(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/packages/mypackage" {
            w.WriteHeader(404)
            return
        }

        resp := packageResponse{
            Name:        "mypackage",
            Description: "A test package",
            License:     "MIT",
        }
        json.NewEncoder(w).Encode(resp)
    }))
    defer server.Close()

    reg := New(server.URL, core.DefaultClient())
    pkg, err := reg.FetchPackage(context.Background(), "mypackage")
    if err != nil {
        t.Fatalf("FetchPackage failed: %v", err)
    }

    if pkg.Name != "mypackage" {
        t.Errorf("expected name 'mypackage', got %q", pkg.Name)
    }
}
```

Test each method:
- `TestFetchPackage`
- `TestFetchVersions`
- `TestFetchDependencies`
- `TestFetchMaintainers`
- `TestURLBuilder`
- `TestEcosystem`

## 12. Add to all/all.go

```go
import (
    // ... existing imports
    _ "github.com/git-pkgs/registries/internal/myregistry"
)
```

Update the comment listing all ecosystems.

## 13. Update registries_test.go

Add the new ecosystem to:
- `TestSupportedEcosystems` expected list
- `TestNew` test cases
- `TestDefaultURL` test cases

## 14. Update Documentation

- Add to README.md ecosystem table
- Add to TODO.md implemented list

## Common Patterns

**Namespaced packages** (npm scopes, Maven groupId):

```go
func (r *Registry) FetchPackage(ctx context.Context, name string) (*core.Package, error) {
    namespace, pkgName := parsePackageName(name)
    // ...
    return &core.Package{
        Name:      name,
        Namespace: namespace,
        // ...
    }, nil
}
```

**Non-JSON APIs** (CRAN, Hackage):

```go
func (r *Registry) FetchPackage(ctx context.Context, name string) (*core.Package, error) {
    url := fmt.Sprintf("%s/packages/%s/DESCRIPTION", r.baseURL, name)

    body, err := r.client.GetBody(ctx, url)
    if err != nil {
        // handle error
    }

    desc := parseDescription(string(body))
    // convert to core.Package
}
```

**Filtering dependencies** (exclude language runtime):

```go
for _, dep := range resp.Dependencies {
    if dep == "python" || dep == "lua" || dep == "R" {
        continue  // Skip runtime itself
    }
    deps = append(deps, core.Dependency{Name: dep})
}
```

**Mapping dependency scopes**:

```go
func mapScope(depType string) core.Scope {
    switch depType {
    case "dependencies":
        return core.Runtime
    case "devDependencies":
        return core.Development
    case "peerDependencies":
        return core.Runtime  // or core.Optional
    case "optionalDependencies":
        return core.Optional
    default:
        return core.Runtime
    }
}
```

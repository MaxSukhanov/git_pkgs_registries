# Core Types

All registry implementations return these normalized types from `internal/core/types.go`.

## Package

Represents a package's metadata.

```go
type Package struct {
    Name        string         // Package name
    Description string         // Short description or summary
    Homepage    string         // Project homepage URL
    Repository  string         // Source repository URL (GitHub, GitLab, etc.)
    Licenses    string         // License identifier(s)
    Keywords    []string       // Tags/categories
    Namespace   string         // Scope/owner (@babel for npm, groupId for Maven)
    Metadata    map[string]any // Registry-specific extra data
}
```

**Field Mapping by Ecosystem:**

| Field | npm | PyPI | Cargo | Maven |
|-------|-----|------|-------|-------|
| Name | name | info.name | crate.name | artifactId |
| Description | description | info.summary | crate.description | description |
| Homepage | homepage | info.home_page | crate.homepage | url |
| Repository | repository.url | info.project_urls.Source | crate.repository | scm.url |
| Licenses | license | info.license | versions[0].license | licenses[0].name |
| Keywords | keywords | info.keywords | crate.keywords | - |
| Namespace | scope (from name) | - | - | groupId |

## Version

Represents a specific version release.

```go
type Version struct {
    Number      string         // Version string ("1.2.3", "0.1.0-beta")
    PublishedAt time.Time      // Release timestamp
    Licenses    string         // License for this version (may differ)
    Integrity   string         // Hash for verification ("sha256-abc123")
    Status      VersionStatus  // "", "yanked", "deprecated", "retracted"
    Metadata    map[string]any // Downloads, size, etc.
}
```

**Status Values:**

```go
const (
    StatusYanked     VersionStatus = "yanked"     // Cargo, RubyGems
    StatusDeprecated VersionStatus = "deprecated" // npm
    StatusRetracted  VersionStatus = "retracted"  // Go modules
)
```

**Integrity Format:**

```
sha256-<hex>
sha512-<hex>
md5-<hex>
```

## Dependency

Represents a package dependency.

```go
type Dependency struct {
    Name         string // Dependency package name
    Requirements string // Version constraint ("^1.0.0", ">=2.0,<3.0")
    Scope        Scope  // runtime, development, test, build, optional
    Optional     bool   // Can be omitted during install
}
```

**Scope Values:**

```go
const (
    Runtime     Scope = "runtime"     // Required at runtime
    Development Scope = "development" // Dev tools, linters
    Test        Scope = "test"        // Test frameworks
    Build       Scope = "build"       // Build-time only
    Optional    Scope = "optional"    // Optional features
)
```

**Scope Mapping by Ecosystem:**

| Ecosystem | Runtime | Development | Test | Build | Optional |
|-----------|---------|-------------|------|-------|----------|
| npm | dependencies | devDependencies | - | - | optionalDependencies |
| PyPI | install_requires | - | tests_require | setup_requires | extras_require |
| Cargo | dependencies | dev-dependencies | - | build-dependencies | - |
| Maven | compile | - | test | provided | - |
| Go | require | - | - | - | - |
| CRAN | Imports | - | - | LinkingTo | Suggests |

## Maintainer

Represents a package maintainer or contributor.

```go
type Maintainer struct {
    UUID  string // Unique identifier (if available)
    Login string // Username/handle
    Name  string // Display name
    Email string // Email address
    URL   string // Profile URL
    Role  string // "owner", "maintainer", "contributor"
}
```

Not all fields are available from every registry. Common patterns:

| Ecosystem | Available Fields |
|-----------|-----------------|
| npm | Login, Email |
| PyPI | Name, Email |
| RubyGems | Login, Email |
| Cargo | Login, URL |
| Maven | Name, Email, URL |
| CRAN | Name, Email |

## URLBuilder

Interface for generating URLs related to a package.

```go
type URLBuilder interface {
    Registry(name, version string) string      // Web page on registry
    Download(name, version string) string      // Direct download URL
    Documentation(name, version string) string // Docs page
    PURL(name, version string) string          // Package URL spec
}
```

**PURL Format:**

```
pkg:<type>/<namespace>/<name>@<version>
```

Examples:
- `pkg:cargo/serde@1.0.195`
- `pkg:npm/@babel/core@7.24.0`
- `pkg:maven/org.apache.commons/commons-lang3@3.14.0`
- `pkg:pypi/requests@2.31.0`

## NotFoundError

Returned when a package or version doesn't exist.

```go
type NotFoundError struct {
    Ecosystem string
    Name      string
    Version   string // Empty for package-level lookups
}

func (e *NotFoundError) Error() string {
    if e.Version != "" {
        return fmt.Sprintf("%s package %s@%s not found", e.Ecosystem, e.Name, e.Version)
    }
    return fmt.Sprintf("%s package %s not found", e.Ecosystem, e.Name)
}
```

Usage:

```go
pkg, err := reg.FetchPackage(ctx, "nonexistent")
if err != nil {
    var notFound *registries.NotFoundError
    if errors.As(err, &notFound) {
        fmt.Printf("Package %s not found in %s\n", notFound.Name, notFound.Ecosystem)
    }
}
```

## Type Conversions

When implementing a registry, convert API responses to core types:

```go
// API response struct
type apiPackage struct {
    PkgName    string   `json:"package_name"`
    Desc       string   `json:"description"`
    Repo       string   `json:"source_url"`
    LicenseID  string   `json:"license"`
    Tags       []string `json:"tags"`
}

// Convert to core.Package
func toPackage(api apiPackage) *core.Package {
    return &core.Package{
        Name:        api.PkgName,
        Description: api.Desc,
        Repository:  api.Repo,
        Licenses:    api.LicenseID,
        Keywords:    api.Tags,
    }
}
```

Keep Metadata for fields that don't map directly:

```go
return &core.Package{
    Name: api.Name,
    Metadata: map[string]any{
        "downloads":    api.DownloadCount,
        "stars":        api.StarCount,
        "last_updated": api.UpdatedAt,
    },
}
```

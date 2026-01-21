package core

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/git-pkgs/purl"
)

const defaultConcurrency = 15

// NewFromPURL creates a registry client from a PURL and returns the parsed components.
// Returns the registry, full package name, and version (empty if not in PURL).
// If the PURL has a repository_url qualifier, it's used as the base URL for private registries.
func NewFromPURL(purlStr string, client *Client) (Registry, string, string, error) {
	p, err := purl.Parse(purlStr)
	if err != nil {
		return nil, "", "", err
	}

	// Extract repository_url qualifier for private registry support
	baseURL := p.RepositoryURL()

	reg, err := New(p.Type, baseURL, client)
	if err != nil {
		return nil, "", "", err
	}

	return reg, p.FullName(), p.Version, nil
}

// FetchPackageFromPURL fetches package metadata using a PURL.
func FetchPackageFromPURL(ctx context.Context, purlStr string, client *Client) (*Package, error) {
	reg, name, _, err := NewFromPURL(purlStr, client)
	if err != nil {
		return nil, err
	}

	return reg.FetchPackage(ctx, name)
}

// FetchVersionFromPURL fetches a specific version's metadata using a PURL.
// Returns an error if the PURL doesn't include a version.
func FetchVersionFromPURL(ctx context.Context, purlStr string, client *Client) (*Version, error) {
	p, err := purl.Parse(purlStr)
	if err != nil {
		return nil, err
	}

	if p.Version == "" {
		return nil, fmt.Errorf("PURL has no version: %s", purlStr)
	}

	baseURL := p.RepositoryURL()
	reg, err := New(p.Type, baseURL, client)
	if err != nil {
		return nil, err
	}

	versions, err := reg.FetchVersions(ctx, p.FullName())
	if err != nil {
		return nil, err
	}

	for _, v := range versions {
		if v.Number == p.Version {
			return &v, nil
		}
	}

	return nil, &NotFoundError{
		Ecosystem: p.Type,
		Name:      p.FullName(),
		Version:   p.Version,
	}
}

// FetchDependenciesFromPURL fetches dependencies for a specific version using a PURL.
// Returns an error if the PURL doesn't include a version.
func FetchDependenciesFromPURL(ctx context.Context, purlStr string, client *Client) ([]Dependency, error) {
	p, err := purl.Parse(purlStr)
	if err != nil {
		return nil, err
	}

	if p.Version == "" {
		return nil, fmt.Errorf("PURL has no version: %s", purlStr)
	}

	baseURL := p.RepositoryURL()
	reg, err := New(p.Type, baseURL, client)
	if err != nil {
		return nil, err
	}

	return reg.FetchDependencies(ctx, p.FullName(), p.Version)
}

// FetchMaintainersFromPURL fetches maintainer information using a PURL.
func FetchMaintainersFromPURL(ctx context.Context, purlStr string, client *Client) ([]Maintainer, error) {
	reg, name, _, err := NewFromPURL(purlStr, client)
	if err != nil {
		return nil, err
	}

	return reg.FetchMaintainers(ctx, name)
}

// FetchLatestVersion returns the latest non-yanked/retracted/deprecated version.
// Returns nil if no valid versions exist.
func FetchLatestVersion(ctx context.Context, reg Registry, name string) (*Version, error) {
	versions, err := reg.FetchVersions(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, nil
	}

	// Filter out yanked/retracted/deprecated versions
	var valid []Version
	for _, v := range versions {
		if v.Status == StatusNone {
			valid = append(valid, v)
		}
	}

	if len(valid) == 0 {
		return nil, nil
	}

	// Sort by PublishedAt descending (newest first)
	// If PublishedAt is zero, fall back to assuming the list order is correct
	hasTimestamps := false
	for _, v := range valid {
		if !v.PublishedAt.IsZero() {
			hasTimestamps = true
			break
		}
	}

	if hasTimestamps {
		sort.Slice(valid, func(i, j int) bool {
			return valid[i].PublishedAt.After(valid[j].PublishedAt)
		})
	}

	return &valid[0], nil
}

// FetchLatestVersionFromPURL returns the latest non-yanked version for a PURL.
func FetchLatestVersionFromPURL(ctx context.Context, purl string, client *Client) (*Version, error) {
	reg, name, _, err := NewFromPURL(purl, client)
	if err != nil {
		return nil, err
	}
	return FetchLatestVersion(ctx, reg, name)
}

// BulkFetchPackages fetches package metadata for multiple PURLs in parallel.
// Individual fetch errors are silently ignored - those PURLs are omitted from results.
// Returns a map of PURL to Package.
func BulkFetchPackages(ctx context.Context, purls []string, client *Client) map[string]*Package {
	return BulkFetchPackagesWithConcurrency(ctx, purls, client, defaultConcurrency)
}

// BulkFetchPackagesWithConcurrency fetches packages with a custom concurrency limit.
func BulkFetchPackagesWithConcurrency(ctx context.Context, purls []string, client *Client, concurrency int) map[string]*Package {
	results := make(map[string]*Package)
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, purl := range purls {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			pkg, err := FetchPackageFromPURL(ctx, p, client)
			if err == nil && pkg != nil {
				mu.Lock()
				results[p] = pkg
				mu.Unlock()
			}
		}(purl)
	}

	wg.Wait()
	return results
}

// BulkFetchVersions fetches version metadata for multiple versioned PURLs in parallel.
// PURLs without versions are silently skipped.
// Individual fetch errors are silently ignored - those PURLs are omitted from results.
// Returns a map of PURL to Version.
func BulkFetchVersions(ctx context.Context, purls []string, client *Client) map[string]*Version {
	return BulkFetchVersionsWithConcurrency(ctx, purls, client, defaultConcurrency)
}

// BulkFetchVersionsWithConcurrency fetches versions with a custom concurrency limit.
func BulkFetchVersionsWithConcurrency(ctx context.Context, purls []string, client *Client, concurrency int) map[string]*Version {
	results := make(map[string]*Version)
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, purl := range purls {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			version, err := FetchVersionFromPURL(ctx, p, client)
			if err == nil && version != nil {
				mu.Lock()
				results[p] = version
				mu.Unlock()
			}
		}(purl)
	}

	wg.Wait()
	return results
}

// BulkFetchLatestVersions fetches the latest version for multiple PURLs in parallel.
// Returns a map of PURL to the latest non-yanked Version.
func BulkFetchLatestVersions(ctx context.Context, purls []string, client *Client) map[string]*Version {
	return BulkFetchLatestVersionsWithConcurrency(ctx, purls, client, defaultConcurrency)
}

// BulkFetchLatestVersionsWithConcurrency fetches latest versions with a custom concurrency limit.
func BulkFetchLatestVersionsWithConcurrency(ctx context.Context, purls []string, client *Client, concurrency int) map[string]*Version {
	results := make(map[string]*Version)
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, purl := range purls {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			version, err := FetchLatestVersionFromPURL(ctx, p, client)
			if err == nil && version != nil {
				mu.Lock()
				results[p] = version
				mu.Unlock()
			}
		}(purl)
	}

	wg.Wait()
	return results
}

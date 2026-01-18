// Package maven provides a registry client for Maven Central (Java).
package maven

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/git-pkgs/registries/internal/core"
)

const (
	DefaultURL    = "https://repo1.maven.org/maven2"
	SearchURL     = "https://search.maven.org"
	ecosystem     = "maven"
	maxParentDepth = 5
)

func init() {
	core.Register(ecosystem, DefaultURL, func(baseURL string, client *core.Client) core.Registry {
		return New(baseURL, client)
	})
}

type Registry struct {
	baseURL   string
	searchURL string
	client    *core.Client
	urls      *URLs
}

func New(baseURL string, client *core.Client) *Registry {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	r := &Registry{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		searchURL: SearchURL,
		client:    client,
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

// searchResponse represents the Maven Central Search API response
type searchResponse struct {
	Response searchResponseBody `json:"response"`
}

type searchResponseBody struct {
	NumFound int         `json:"numFound"`
	Docs     []searchDoc `json:"docs"`
}

type searchDoc struct {
	ID        string `json:"id"`
	GroupID   string `json:"g"`
	ArtifactID string `json:"a"`
	Version    string `json:"latestVersion"`
	Timestamp  int64  `json:"timestamp"`
	VersionCount int  `json:"versionCount"`
}

// POM XML structures
type pomXML struct {
	XMLName     xml.Name    `xml:"project"`
	GroupID     string      `xml:"groupId"`
	ArtifactID  string      `xml:"artifactId"`
	Version     string      `xml:"version"`
	Name        string      `xml:"name"`
	Description string      `xml:"description"`
	URL         string      `xml:"url"`
	Licenses    []pomLicense `xml:"licenses>license"`
	SCM         pomSCM      `xml:"scm"`
	Parent      *pomParent  `xml:"parent"`
	Dependencies []pomDep   `xml:"dependencies>dependency"`
	DependencyManagement struct {
		Dependencies []pomDep `xml:"dependencies>dependency"`
	} `xml:"dependencyManagement"`
	Developers []pomDeveloper `xml:"developers>developer"`
	Properties map[string]string
}

type pomParent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type pomLicense struct {
	Name string `xml:"name"`
	URL  string `xml:"url"`
}

type pomSCM struct {
	URL        string `xml:"url"`
	Connection string `xml:"connection"`
	DevConnection string `xml:"developerConnection"`
}

type pomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Optional   string `xml:"optional"`
	Type       string `xml:"type"`
}

type pomDeveloper struct {
	ID    string `xml:"id"`
	Name  string `xml:"name"`
	Email string `xml:"email"`
	URL   string `xml:"url"`
}

// ParseCoordinates parses a Maven coordinate string (groupId:artifactId or groupId:artifactId:version)
func ParseCoordinates(coord string) (groupID, artifactID, version string) {
	parts := strings.Split(coord, ":")
	if len(parts) >= 2 {
		groupID = parts[0]
		artifactID = parts[1]
	}
	if len(parts) >= 3 {
		version = parts[2]
	}
	return
}

func (r *Registry) FetchPackage(ctx context.Context, name string) (*core.Package, error) {
	groupID, artifactID, _ := ParseCoordinates(name)
	if groupID == "" || artifactID == "" {
		return nil, fmt.Errorf("invalid Maven coordinate: %s (expected groupId:artifactId)", name)
	}

	// First try the search API to get basic metadata
	searchURL := fmt.Sprintf("%s/solrsearch/select?q=g:%s+AND+a:%s&core=gav&rows=1&wt=json",
		r.searchURL, url.QueryEscape(groupID), url.QueryEscape(artifactID))

	var searchResp searchResponse
	if err := r.client.GetJSON(ctx, searchURL, &searchResp); err == nil && searchResp.Response.NumFound > 0 {
		doc := searchResp.Response.Docs[0]
		// Fetch the POM for more details
		pom, _ := r.fetchPOM(ctx, groupID, artifactID, doc.Version, 0)
		return r.packageFromSearchAndPOM(doc, pom), nil
	}

	// Fallback: try to get maven-metadata.xml
	metadataURL := fmt.Sprintf("%s/%s/%s/maven-metadata.xml",
		r.baseURL, groupIDToPath(groupID), artifactID)

	body, err := r.client.GetBody(ctx, metadataURL)
	if err != nil {
		if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
			return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name}
		}
		return nil, err
	}

	var metadata mavenMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}

	// Get the latest version's POM
	latestVersion := metadata.Versioning.Latest
	if latestVersion == "" && len(metadata.Versioning.Versions) > 0 {
		latestVersion = metadata.Versioning.Versions[len(metadata.Versioning.Versions)-1]
	}

	pom, _ := r.fetchPOM(ctx, groupID, artifactID, latestVersion, 0)
	return r.packageFromMetadataAndPOM(metadata, pom), nil
}

type mavenMetadata struct {
	GroupID    string     `xml:"groupId"`
	ArtifactID string     `xml:"artifactId"`
	Versioning versioning `xml:"versioning"`
}

type versioning struct {
	Latest   string   `xml:"latest"`
	Release  string   `xml:"release"`
	Versions []string `xml:"versions>version"`
}

func (r *Registry) fetchPOM(ctx context.Context, groupID, artifactID, version string, depth int) (*pomXML, error) {
	if depth > maxParentDepth {
		return nil, fmt.Errorf("max parent depth exceeded")
	}

	pomURL := fmt.Sprintf("%s/%s/%s/%s/%s-%s.pom",
		r.baseURL, groupIDToPath(groupID), artifactID, version, artifactID, version)

	body, err := r.client.GetBody(ctx, pomURL)
	if err != nil {
		return nil, err
	}

	var pom pomXML
	if err := xml.Unmarshal(body, &pom); err != nil {
		return nil, err
	}

	// Resolve parent POM if present
	if pom.Parent != nil && depth < maxParentDepth {
		parentPOM, err := r.fetchPOM(ctx, pom.Parent.GroupID, pom.Parent.ArtifactID, pom.Parent.Version, depth+1)
		if err == nil {
			mergePOMs(&pom, parentPOM)
		}
	}

	// Fill in groupID/version from parent if not set
	if pom.GroupID == "" && pom.Parent != nil {
		pom.GroupID = pom.Parent.GroupID
	}
	if pom.Version == "" && pom.Parent != nil {
		pom.Version = pom.Parent.Version
	}

	return &pom, nil
}

func mergePOMs(child, parent *pomXML) {
	if child.Description == "" {
		child.Description = parent.Description
	}
	if child.URL == "" {
		child.URL = parent.URL
	}
	if len(child.Licenses) == 0 {
		child.Licenses = parent.Licenses
	}
	if child.SCM.URL == "" {
		child.SCM = parent.SCM
	}
	if len(child.Developers) == 0 {
		child.Developers = parent.Developers
	}
}

func (r *Registry) packageFromSearchAndPOM(doc searchDoc, pom *pomXML) *core.Package {
	pkg := &core.Package{
		Name:      fmt.Sprintf("%s:%s", doc.GroupID, doc.ArtifactID),
		Namespace: doc.GroupID,
		Metadata: map[string]any{
			"group_id":      doc.GroupID,
			"artifact_id":   doc.ArtifactID,
			"version_count": doc.VersionCount,
		},
	}

	if pom != nil {
		pkg.Description = pom.Description
		pkg.Homepage = pom.URL
		pkg.Repository = extractRepository(pom)
		pkg.Licenses = formatLicenses(pom.Licenses)
	}

	return pkg
}

func (r *Registry) packageFromMetadataAndPOM(metadata mavenMetadata, pom *pomXML) *core.Package {
	pkg := &core.Package{
		Name:      fmt.Sprintf("%s:%s", metadata.GroupID, metadata.ArtifactID),
		Namespace: metadata.GroupID,
		Metadata: map[string]any{
			"group_id":    metadata.GroupID,
			"artifact_id": metadata.ArtifactID,
		},
	}

	if pom != nil {
		pkg.Description = pom.Description
		pkg.Homepage = pom.URL
		pkg.Repository = extractRepository(pom)
		pkg.Licenses = formatLicenses(pom.Licenses)
	}

	return pkg
}

func extractRepository(pom *pomXML) string {
	if pom.SCM.URL != "" {
		url := pom.SCM.URL
		// Clean up common SCM URL prefixes
		url = strings.TrimPrefix(url, "scm:git:")
		url = strings.TrimPrefix(url, "scm:svn:")
		url = strings.TrimSuffix(url, ".git")
		if strings.HasPrefix(url, "git://") {
			url = "https://" + strings.TrimPrefix(url, "git://")
		}
		return url
	}
	if pom.SCM.Connection != "" {
		conn := pom.SCM.Connection
		conn = strings.TrimPrefix(conn, "scm:git:")
		conn = strings.TrimPrefix(conn, "scm:svn:")
		if strings.HasPrefix(conn, "git://") || strings.HasPrefix(conn, "https://") || strings.HasPrefix(conn, "http://") {
			return strings.TrimSuffix(conn, ".git")
		}
	}
	return ""
}

func formatLicenses(licenses []pomLicense) string {
	names := make([]string, len(licenses))
	for i, l := range licenses {
		names[i] = l.Name
	}
	return strings.Join(names, ",")
}

func (r *Registry) FetchVersions(ctx context.Context, name string) ([]core.Version, error) {
	groupID, artifactID, _ := ParseCoordinates(name)
	if groupID == "" || artifactID == "" {
		return nil, fmt.Errorf("invalid Maven coordinate: %s (expected groupId:artifactId)", name)
	}

	// Use search API to get all versions
	searchURL := fmt.Sprintf("%s/solrsearch/select?q=g:%s+AND+a:%s&core=gav&rows=200&wt=json",
		r.searchURL, url.QueryEscape(groupID), url.QueryEscape(artifactID))

	var searchResp searchResponse
	if err := r.client.GetJSON(ctx, searchURL, &searchResp); err == nil && searchResp.Response.NumFound > 0 {
		versions := make([]core.Version, len(searchResp.Response.Docs))
		for i, doc := range searchResp.Response.Docs {
			var publishedAt time.Time
			if doc.Timestamp > 0 {
				publishedAt = time.UnixMilli(doc.Timestamp)
			}
			versions[i] = core.Version{
				Number:      doc.Version,
				PublishedAt: publishedAt,
			}
		}
		return versions, nil
	}

	// Fallback: maven-metadata.xml
	metadataURL := fmt.Sprintf("%s/%s/%s/maven-metadata.xml",
		r.baseURL, groupIDToPath(groupID), artifactID)

	body, err := r.client.GetBody(ctx, metadataURL)
	if err != nil {
		if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
			return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name}
		}
		return nil, err
	}

	var metadata mavenMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}

	versions := make([]core.Version, len(metadata.Versioning.Versions))
	for i, v := range metadata.Versioning.Versions {
		versions[i] = core.Version{
			Number: v,
		}
	}

	return versions, nil
}

func (r *Registry) FetchDependencies(ctx context.Context, name, version string) ([]core.Dependency, error) {
	groupID, artifactID, _ := ParseCoordinates(name)
	if groupID == "" || artifactID == "" {
		return nil, fmt.Errorf("invalid Maven coordinate: %s (expected groupId:artifactId)", name)
	}

	pom, err := r.fetchPOM(ctx, groupID, artifactID, version, 0)
	if err != nil {
		if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
			return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name, Version: version}
		}
		return nil, err
	}

	var deps []core.Dependency
	for _, d := range pom.Dependencies {
		scope := mapMavenScope(d.Scope)
		optional := d.Optional == "true"

		if optional {
			scope = core.Optional
		}

		deps = append(deps, core.Dependency{
			Name:         fmt.Sprintf("%s:%s", d.GroupID, d.ArtifactID),
			Requirements: d.Version,
			Scope:        scope,
			Optional:     optional,
		})
	}

	return deps, nil
}

func mapMavenScope(scope string) core.Scope {
	switch strings.ToLower(scope) {
	case "compile", "":
		return core.Runtime
	case "test":
		return core.Test
	case "provided":
		return core.Build
	case "runtime":
		return core.Runtime
	default:
		return core.Runtime
	}
}

func (r *Registry) FetchMaintainers(ctx context.Context, name string) ([]core.Maintainer, error) {
	groupID, artifactID, _ := ParseCoordinates(name)
	if groupID == "" || artifactID == "" {
		return nil, fmt.Errorf("invalid Maven coordinate: %s (expected groupId:artifactId)", name)
	}

	// Get latest version first
	versions, err := r.FetchVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}

	latestVersion := versions[0].Number
	pom, err := r.fetchPOM(ctx, groupID, artifactID, latestVersion, 0)
	if err != nil {
		return nil, err
	}

	maintainers := make([]core.Maintainer, len(pom.Developers))
	for i, dev := range pom.Developers {
		maintainers[i] = core.Maintainer{
			UUID:  dev.ID,
			Login: dev.ID,
			Name:  dev.Name,
			Email: dev.Email,
			URL:   dev.URL,
		}
	}

	return maintainers, nil
}

func groupIDToPath(groupID string) string {
	return strings.ReplaceAll(groupID, ".", "/")
}

type URLs struct {
	baseURL string
}

func (u *URLs) Registry(name, version string) string {
	groupID, artifactID, _ := ParseCoordinates(name)
	if version != "" {
		return fmt.Sprintf("https://search.maven.org/artifact/%s/%s/%s/jar", groupID, artifactID, version)
	}
	return fmt.Sprintf("https://search.maven.org/artifact/%s/%s", groupID, artifactID)
}

func (u *URLs) Download(name, version string) string {
	if version == "" {
		return ""
	}
	groupID, artifactID, _ := ParseCoordinates(name)
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		u.baseURL, groupIDToPath(groupID), artifactID, version, artifactID, version)
}

func (u *URLs) Documentation(name, version string) string {
	groupID, artifactID, _ := ParseCoordinates(name)
	if version != "" {
		return fmt.Sprintf("https://javadoc.io/doc/%s/%s/%s", groupID, artifactID, version)
	}
	return fmt.Sprintf("https://javadoc.io/doc/%s/%s", groupID, artifactID)
}

func (u *URLs) PURL(name, version string) string {
	groupID, artifactID, _ := ParseCoordinates(name)
	if version != "" {
		return fmt.Sprintf("pkg:maven/%s/%s@%s", groupID, artifactID, version)
	}
	return fmt.Sprintf("pkg:maven/%s/%s", groupID, artifactID)
}

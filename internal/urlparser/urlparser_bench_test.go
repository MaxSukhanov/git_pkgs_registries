package urlparser

import "testing"

var benchURLs = []string{
	"https://github.com/owner/repo",
	"git@github.com:owner/repo.git",
	"git+https://github.com/owner/repo.git",
	"scm:git:https://github.com/owner/repo.git",
	"https://user:pass@github.com/owner/repo.git",
	"https://gitlab.com/owner/repo",
	"https://bitbucket.org/owner/repo",
	"https://git.example.com/owner/repo",
	"foo.github.io/bar",
}

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, url := range benchURLs {
			Parse(url)
		}
	}
}

func BenchmarkParseSingle(b *testing.B) {
	url := "scm:git:https://user@github.com/owner/repo.git"
	for i := 0; i < b.N; i++ {
		Parse(url)
	}
}

func BenchmarkClean(b *testing.B) {
	url := "scm:git:https://user@github.com/owner/repo.git#anchor?query=1"
	for i := 0; i < b.N; i++ {
		Clean(url)
	}
}

func BenchmarkExtractOwnerRepo(b *testing.B) {
	url := "https://github.com/owner/repo/tree/main"
	for i := 0; i < b.N; i++ {
		ExtractOwnerRepo(url)
	}
}

func BenchmarkNormalize(b *testing.B) {
	url := "git+https://github.com/owner/repo.git"
	for i := 0; i < b.N; i++ {
		Normalize(url)
	}
}

func BenchmarkIsKnownHost(b *testing.B) {
	url := "https://github.com/owner/repo"
	for i := 0; i < b.N; i++ {
		IsKnownHost(url)
	}
}

func BenchmarkFirstRepoURL(b *testing.B) {
	urls := []string{"", "https://example.com", "https://github.com/owner/repo"}
	for i := 0; i < b.N; i++ {
		FirstRepoURL(urls...)
	}
}

func BenchmarkParseURL(b *testing.B) {
	url := "https://github.com/owner/repo"
	for i := 0; i < b.N; i++ {
		ParseURL(url)
	}
}

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T005: deletion guards (FR-007a, FR-013). Deleted symbols must stay dead.
// If this test fails, someone re-introduced a fallback/lexicon — fix the code,
// never the test.
func TestDeletedSymbolsStayDeleted(t *testing.T) {
	banned := []string{
		"fallbackHeuristic",
		"AnalyzeNewsSentiment",
		"bullishTerms",
		"bearishTerms",
		"// Quantitative fallback",
		"Using fallback",
		"Using fallback heuristic",
	}
	repoRoot := findRepoRoot(t)
	var hits []string
	_ = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "deleted_test.go") {
			return nil
		}
		// specs/ and research docs may quote removed names — only source matters.
		rel, _ := filepath.Rel(repoRoot, path)
		if strings.HasPrefix(rel, string(filepath.Separator)+"specs") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, b := range banned {
			if strings.Contains(string(data), b) {
				hits = append(hits, rel+" :: "+b)
			}
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("deleted symbols reappeared (explicit-error policy violation): %v", hits)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}

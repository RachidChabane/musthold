package cli

import "testing"

func TestIndexersServeEveryLanguage(t *testing.T) {
	t.Run("every source file of a supported language is served by exactly one indexer [verifies SP-1311~1]", func(t *testing.T) {
		files := []string{
			"src/totals.py", "totals.go",
			"src/totals.js", "src/totals.jsx", "src/totals.mjs", "src/totals.cjs",
			"src/totals.ts", "src/totals.tsx", "src/totals.mts",
			"src/main/java/ledger/Totals.java", "src/main/kotlin/ledger/Totals.kt",
		}
		for _, file := range files {
			served := 0
			for _, indexer := range indexers() {
				if indexer.Handles(file) {
					served++
				}
			}
			if served != 1 {
				t.Errorf("%s is served by %d indexers", file, served)
			}
		}
	})

	t.Run("a file of no supported language is served by none [verifies SP-1311~1]", func(t *testing.T) {
		for _, file := range []string{"README.md", "main.rs", "specs/invoicing.md", "build.gradle", "pom.xml"} {
			for _, indexer := range indexers() {
				if indexer.Handles(file) {
					t.Errorf("%s is served", file)
				}
			}
		}
	})
}

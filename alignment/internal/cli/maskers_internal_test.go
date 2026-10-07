package cli

import "testing"

func TestMaskersServeEveryLanguage(t *testing.T) {
	t.Run("every test file of a supported language is served by exactly one masker [verifies SP-1230~1]", func(t *testing.T) {
		files := []string{
			"tests/test_pricing.py", "pricing_test.go",
			"test/rates.test.js", "test/rates.test.jsx", "test/rates.test.mjs", "test/rates.test.cjs",
			"test/rates.test.ts", "test/rates.test.tsx", "test/rates.test.mts",
			"src/test/java/shop/PricingTest.java", "src/test/kotlin/shop/PricingTest.kt",
		}
		for _, file := range files {
			served := 0
			for _, masker := range maskers() {
				if masker.Handles(file) {
					served++
				}
			}
			if served != 1 {
				t.Errorf("%s is served by %d maskers", file, served)
			}
		}
	})

	t.Run("a file of no supported language is served by none [verifies SP-1230~1]", func(t *testing.T) {
		for _, file := range []string{"README.md", "main.rs", "specs/spec.md", "build.gradle"} {
			for _, masker := range maskers() {
				if masker.Handles(file) {
					t.Errorf("%s is served", file)
				}
			}
		}
	})
}

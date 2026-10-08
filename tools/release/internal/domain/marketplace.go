package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseMarketplace reads the plugins of a Claude Code marketplace file. A plugin
// whose source is a string is a path inside the marketplace; one whose source is
// an object names a repository and may pin a ref.
func ParseMarketplace(data []byte) ([]MarketplaceEntry, error) {
	var document struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("the marketplace is not JSON: %w", err)
	}
	entries := make([]MarketplaceEntry, 0, len(document.Plugins))
	for _, plugin := range document.Plugins {
		entry := MarketplaceEntry{Name: plugin.Name}
		var path string
		if json.Unmarshal(plugin.Source, &path) == nil {
			entry.Relative = true
			entries = append(entries, entry)
			continue
		}
		var source struct {
			URL string `json:"url"`
			Ref string `json:"ref"`
		}
		if err := json.Unmarshal(plugin.Source, &source); err != nil {
			return nil, fmt.Errorf("the source of marketplace plugin %q is neither a path nor an object: %w", plugin.Name, err)
		}
		entry.Repository = repositorySlug(source.URL)
		entry.Ref = source.Ref
		entries = append(entries, entry)
	}
	return entries, nil
}

// repositorySlug reduces a repository URL or `owner/name` to `owner/name`.
func repositorySlug(url string) string {
	slug := strings.TrimSuffix(url, ".git")
	slug = strings.TrimPrefix(slug, "https://github.com/")
	return strings.Trim(slug, "/")
}

// ParseManifestVersion reads the version a plugin.json carries.
func ParseManifestVersion(data []byte) (string, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("the plugin manifest is not JSON: %w", err)
	}
	return manifest.Version, nil
}

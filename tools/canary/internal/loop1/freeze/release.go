package freeze

import (
	"context"
	"regexp"
	"slices"
	"strconv"
)

// NoRelease is recorded when no public release contains the pin.
const NoRelease = "none"

var releaseTag = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

type releaseVersion struct {
	tag                 string
	major, minor, patch int
}

// FirstRelease is the lowest release tag, by version, whose history contains the
// commit, or NoRelease. Tags that are not plain versions are ignored.
func FirstRelease(ctx context.Context, releases Releases, commit string) (string, error) {
	tags, err := releases.Tags(ctx)
	if err != nil {
		return "", err
	}
	var versions []releaseVersion
	for _, tag := range tags {
		if match := releaseTag.FindStringSubmatch(tag); match != nil {
			versions = append(versions, releaseVersion{tag, atoi(match[1]), atoi(match[2]), atoi(match[3])})
		}
	}
	slices.SortFunc(versions, compareVersions)
	for _, version := range versions {
		contains, err := releases.Contains(ctx, version.tag, commit)
		if err != nil {
			return "", err
		}
		if contains {
			return version.tag, nil
		}
	}
	return NoRelease, nil
}

func atoi(digits string) int {
	value, _ := strconv.Atoi(digits)
	return value
}

func compareVersions(a, b releaseVersion) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] != pair[1] {
			return pair[0] - pair[1]
		}
	}
	return 0
}

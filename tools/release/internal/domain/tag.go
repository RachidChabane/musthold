package domain

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// tagPattern is the shape of a release tag: `<module>/vMAJOR.MINOR.PATCH[-pre]`,
// with no leading zero in a number.
var tagPattern = regexp.MustCompile(`^([a-z0-9]+(?:/[a-z0-9]+)*)/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// Positions of the groups of tagPattern.
const (
	groupModule = iota + 1
	groupMajor
	groupMinor
	groupPatch
	groupPrerelease
)

// ErrNotReleased is wrapped by the error of a tag whose module the release table
// does not list.
var ErrNotReleased = errors.New("not a released module")

// Version is a semantic version.
type Version struct {
	Major, Minor, Patch int
	// Prerelease is the pre-release suffix without its leading hyphen.
	Prerelease string
}

// String renders the version without a leading v.
func (v Version) String() string {
	text := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		text += "-" + v.Prerelease
	}
	return text
}

// Compare orders two versions by semantic-version precedence: the numbers first,
// then a pre-release below the release it precedes, then the pre-release
// identifiers one by one, numeric ones below alphanumeric ones.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] != pair[1] {
			return cmp.Compare(pair[0], pair[1])
		}
	}
	switch {
	case v.Prerelease == other.Prerelease:
		return 0
	case v.Prerelease == "":
		return 1
	case other.Prerelease == "":
		return -1
	}
	return comparePrerelease(strings.Split(v.Prerelease, "."), strings.Split(other.Prerelease, "."))
}

func comparePrerelease(left, right []string) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := compareIdentifier(left[index], right[index]); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}

func compareIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	switch {
	case leftErr == nil && rightErr == nil:
		return cmp.Compare(leftNumber, rightNumber)
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	}
	return strings.Compare(left, right)
}

// Tag is a parsed release tag.
type Tag struct {
	Module  Module
	Version Version
}

// String renders the tag as it is spelled in the repository.
func (t Tag) String() string { return t.Module.Name + "/v" + t.Version.String() }

// Semver is the version of the tag without its module and its v.
func (t Tag) Semver() string { return t.Version.String() }

// ParseTag parses a release tag. A tag of a module outside the release table, a
// bare version and a version with a leading zero are refused.
func ParseTag(text string) (Tag, error) {
	groups := tagPattern.FindStringSubmatch(text)
	if groups == nil {
		return Tag{}, fmt.Errorf("tag %q is not <module>/vMAJOR.MINOR.PATCH[-pre]", text)
	}
	module, known := LookupModule(groups[groupModule])
	if !known {
		return Tag{}, fmt.Errorf("tag %q: %q is %w", text, groups[groupModule], ErrNotReleased)
	}
	numbers := make([]int, 0, 3)
	for _, group := range []int{groupMajor, groupMinor, groupPatch} {
		number, err := strconv.Atoi(groups[group])
		if err != nil {
			return Tag{}, fmt.Errorf("tag %q: %w", text, err)
		}
		numbers = append(numbers, number)
	}
	version := Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if suffix := groups[groupPrerelease]; suffix != "" {
		version.Prerelease = suffix[1:]
	}
	return Tag{Module: module, Version: version}, nil
}

// ParseFormatTag parses a tag of the format library, which the release of every
// other module requires.
func ParseFormatTag(text string) (Tag, error) {
	tag, err := ParseTag(text)
	if err != nil {
		return Tag{}, err
	}
	if tag.Module.Name != FormatModuleName {
		return Tag{}, fmt.Errorf("tag %q is not a tag of %s", text, FormatModuleName)
	}
	return tag, nil
}

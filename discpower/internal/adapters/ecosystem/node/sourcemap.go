package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ErrSourceMap is returned for a source map this reader cannot decode.
var ErrSourceMap = errors.New("unreadable source map")

const (
	sourceMapVersion = 3
	// vlqShift is how many bits of a value a base64 digit carries, the bit
	// above them marking that another digit follows.
	vlqShift        = 5
	vlqContinuation = 1 << vlqShift
	vlqMask         = vlqContinuation - 1
	vlqAlphabet     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	// segmentSeparator and lineSeparator split the mappings of a source map.
	segmentSeparator = ','
	lineSeparator    = ';'
	// mappedFields is how many fields a segment that names a source position
	// has, the name aside.
	mappedFields = 4
)

// vlqDigits maps a base64 digit to its value, -1 for a byte that is none.
var vlqDigits = func() [256]int {
	var digits [256]int
	for index := range digits {
		digits[index] = -1
	}
	for value, digit := range vlqAlphabet {
		digits[digit] = value
	}
	return digits
}()

// sourceMap is a decoded source map of version 3: the segments of each
// generated line, in column order.
type sourceMap struct {
	sourceRoot string
	sources    []string
	lines      [][]mapSegment
}

// mapSegment is where a generated column starts mapping: to a line of a
// source, or to nothing.
type mapSegment struct {
	column int
	mapped bool
	source int
	line   int
}

type sourceMapJSON struct {
	Version    int      `json:"version"`
	SourceRoot string   `json:"sourceRoot"`
	Sources    []string `json:"sources"`
	Mappings   string   `json:"mappings"`
}

// parseSourceMap decodes the mappings of a source map. A map of sections or of
// another version is refused.
func parseSourceMap(raw string) (sourceMap, error) {
	var decoded sourceMapJSON
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return sourceMap{}, fmt.Errorf("%w: %v", ErrSourceMap, err)
	}
	if decoded.Version != sourceMapVersion {
		return sourceMap{}, fmt.Errorf("%w: version %d", ErrSourceMap, decoded.Version)
	}
	lines, err := decodeMappings(decoded.Mappings, len(decoded.Sources))
	if err != nil {
		return sourceMap{}, err
	}
	return sourceMap{sourceRoot: decoded.SourceRoot, sources: decoded.Sources, lines: lines}, nil
}

// decodeMappings reads the base64 VLQ mappings: per generated line, segments
// of one, four or five fields, each but the generated column relative to the
// previous segment of the map.
func decodeMappings(mappings string, sources int) ([][]mapSegment, error) {
	lines := [][]mapSegment{nil}
	var source, line int
	column := 0
	position := 0
	for position < len(mappings) {
		switch mappings[position] {
		case lineSeparator:
			lines = append(lines, nil)
			column = 0
			position++
			continue
		case segmentSeparator:
			position++
			continue
		}
		var fields []int
		for position < len(mappings) && mappings[position] != segmentSeparator && mappings[position] != lineSeparator {
			value, next, err := decodeVLQ(mappings, position)
			if err != nil {
				return nil, err
			}
			fields = append(fields, value)
			position = next
		}
		current := len(lines) - 1
		column += fields[0]
		segment := mapSegment{column: column}
		if len(fields) >= mappedFields {
			source += fields[1]
			line += fields[2]
			if source < 0 || source >= sources || line < 0 {
				return nil, fmt.Errorf("%w: a segment maps outside its sources", ErrSourceMap)
			}
			segment.mapped, segment.source, segment.line = true, source, line
		}
		lines[current] = append(lines[current], segment)
	}
	for index := range lines {
		slices.SortStableFunc(lines[index], func(a, b mapSegment) int { return a.column - b.column })
	}
	return lines, nil
}

// decodeVLQ reads one value from the position and returns it with the position
// after it.
func decodeVLQ(mappings string, position int) (int, int, error) {
	value, shift := 0, 0
	for {
		if position >= len(mappings) {
			return 0, 0, fmt.Errorf("%w: a value ends early", ErrSourceMap)
		}
		digit := vlqDigits[mappings[position]]
		if digit < 0 {
			return 0, 0, fmt.Errorf("%w: %q is no base64 digit", ErrSourceMap, mappings[position])
		}
		position++
		value += (digit & vlqMask) << shift
		if digit&vlqContinuation == 0 {
			break
		}
		shift += vlqShift
	}
	if value&1 == 1 {
		return -(value >> 1), position, nil
	}
	return value >> 1, position, nil
}

// lookup maps a generated position, zero-based, to the zero-based line of the
// source it comes from: the last segment of its line that starts at or before
// the column decides, and a column before the first segment maps to nothing.
func (m sourceMap) lookup(line, column int) (source, sourceLine int, mapped bool) {
	if line < 0 || line >= len(m.lines) {
		return 0, 0, false
	}
	segments := m.lines[line]
	index, _ := slices.BinarySearchFunc(segments, column+1, func(segment mapSegment, target int) int { return segment.column - target })
	if index == 0 {
		return 0, 0, false
	}
	segment := segments[index-1]
	return segment.source, segment.line, segment.mapped
}

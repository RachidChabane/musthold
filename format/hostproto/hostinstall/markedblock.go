package hostinstall

import (
	"fmt"
	"strings"
)

// blockGap separates a module's block from the text around it.
const blockGap = "\n\n"

// markedBlock is a block of text a module keeps inside a file other tools write
// too, delimited by a begin and an end marker. Setting it replaces a previous
// copy and removing it takes it out; the rest of the file stays byte for byte.
type markedBlock struct {
	begin string
	end   string
	// text is the block with its markers and a final newline.
	text string
}

func (b markedBlock) unbalanced() error {
	return fmt.Errorf("the %s and %s markers are unbalanced", b.begin, b.end)
}

// set puts the block in the file.
func (b markedBlock) set(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, b.begin), strings.Index(text, b.end)
	switch {
	case begin >= 0 && end > begin:
		after := strings.TrimPrefix(text[end+len(b.end):], "\n")
		return []byte(text[:begin] + b.text + after), nil
	case begin >= 0 || end >= 0:
		return nil, b.unbalanced()
	case strings.TrimSpace(text) == "":
		return []byte(b.text), nil
	default:
		return []byte(strings.TrimRight(text, "\n") + blockGap + b.text), nil
	}
}

// remove takes the block out of the file, with the blank line that set it apart.
// A file that held nothing else comes out empty.
func (b markedBlock) remove(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, b.begin), strings.Index(text, b.end)
	switch {
	case begin < 0 && end < 0:
		return current, nil
	case begin < 0 || end < begin:
		return nil, b.unbalanced()
	}
	before := strings.TrimRight(text[:begin], "\n")
	after := strings.TrimLeft(text[end+len(b.end):], "\n")
	switch {
	case before == "" && after == "":
		return nil, nil
	case after == "":
		return []byte(before + "\n"), nil
	case before == "":
		return []byte(after), nil
	default:
		return []byte(before + blockGap + after), nil
	}
}

package hostinstall

import (
	"fmt"
	"strings"
)

// Markers of the module's block of AGENTS.md. Shallnot keeps its own block in the
// same file; neither touches the other's.
const (
	sectionBegin = "<!-- spec-proof-dp:begin -->"
	sectionEnd   = "<!-- spec-proof-dp:end -->"
	blockGap     = "\n\n"
)

// agentsSection is the text the module asks agents to follow.
const agentsSection = sectionBegin + `
## spec-proof-dp: discriminating power

After any change to code, tests or specs, run ` + "`spec-proof-dp prove --changed`" + ` before finishing. When it exits with code 3, run the ` + "`spec-proof-dp-fulfil`" + ` skill and follow it to the end. Use the executor subagents the skill names; never answer a ticket yourself while they are available.

On a harness without that skill or those subagents, answer each ticket that ` + "`spec-proof-dp request list`" + ` prints: read it with ` + "`spec-proof-dp request show <ticket>`" + `, submit the JSON answer with ` + "`spec-proof-dp request submit <ticket> --nonce <nonce>`" + ` on standard input, then run ` + "`spec-proof-dp prove --resume`" + `.
` + sectionEnd + "\n"

// setAgentsBlock puts the module's block in the file, replacing a previous copy
// of it and leaving the rest of the file alone.
func setAgentsBlock(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, sectionBegin), strings.Index(text, sectionEnd)
	switch {
	case begin >= 0 && end > begin:
		after := strings.TrimPrefix(text[end+len(sectionEnd):], "\n")
		return []byte(text[:begin] + agentsSection + after), nil
	case begin >= 0 || end >= 0:
		return nil, fmt.Errorf("the %s and %s markers are unbalanced", sectionBegin, sectionEnd)
	case strings.TrimSpace(text) == "":
		return []byte(agentsSection), nil
	default:
		return []byte(strings.TrimRight(text, "\n") + blockGap + agentsSection), nil
	}
}

// removeAgentsBlock takes the module's block out of the file, with the blank line
// that set it apart. A file that held nothing else comes out empty.
func removeAgentsBlock(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, sectionBegin), strings.Index(text, sectionEnd)
	switch {
	case begin < 0 && end < 0:
		return current, nil
	case begin < 0 || end < begin:
		return nil, fmt.Errorf("the %s and %s markers are unbalanced", sectionBegin, sectionEnd)
	}
	before := strings.TrimRight(text[:begin], "\n")
	after := strings.TrimLeft(text[end+len(sectionEnd):], "\n")
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

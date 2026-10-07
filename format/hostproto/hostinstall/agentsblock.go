package hostinstall

import (
	"fmt"
	"strings"
)

// blockGap separates the module's block from the text around it.
const blockGap = "\n\n"

// Advisory sentence a module whose proof Shallnot cannot require adds to its block.
const advisorySentence = "\n\nThe result is advisory: Shallnot reports it and never blocks on it."

func (m Module) sectionBegin() string { return "<!-- " + m.Files.Plugin() + ":begin -->" }
func (m Module) sectionEnd() string   { return "<!-- " + m.Files.Plugin() + ":end -->" }

// agentsSection is the text the module asks agents to follow, between its markers.
// Shallnot keeps its own block in the same file; neither touches the other's.
func (m Module) agentsSection() string {
	binary, plugin := m.Files.Binary, m.Files.Plugin()
	advisory := ""
	if !m.Required {
		advisory = advisorySentence
	}
	return m.sectionBegin() + "\n" +
		"## " + plugin + ": " + m.Files.Subject + "\n\n" +
		"After any change to code, tests or specs, run `" + binary + " prove --changed` before finishing. " +
		"When it exits with code 3, run the `" + m.Files.SkillName() + "` skill and follow it to the end. " +
		"Use the executor subagents the skill names; never answer a ticket yourself while they are available.\n\n" +
		"On a harness without that skill or those subagents, answer each ticket that `" + binary + " request list` prints: " +
		"read it with `" + binary + " request show <ticket>`, " +
		"submit the JSON answer with `" + binary + " request submit <ticket> --nonce <nonce>` on standard input, " +
		"then run `" + binary + " prove --resume`." + advisory + "\n" +
		m.sectionEnd() + "\n"
}

// setAgentsBlock puts the module's block in the file, replacing a previous copy
// of it and leaving the rest of the file alone.
func (m Module) setAgentsBlock(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, m.sectionBegin()), strings.Index(text, m.sectionEnd())
	section := m.agentsSection()
	switch {
	case begin >= 0 && end > begin:
		after := strings.TrimPrefix(text[end+len(m.sectionEnd()):], "\n")
		return []byte(text[:begin] + section + after), nil
	case begin >= 0 || end >= 0:
		return nil, m.unbalanced()
	case strings.TrimSpace(text) == "":
		return []byte(section), nil
	default:
		return []byte(strings.TrimRight(text, "\n") + blockGap + section), nil
	}
}

func (m Module) unbalanced() error {
	return fmt.Errorf("the %s and %s markers are unbalanced", m.sectionBegin(), m.sectionEnd())
}

// removeAgentsBlock takes the module's block out of the file, with the blank line
// that set it apart. A file that held nothing else comes out empty.
func (m Module) removeAgentsBlock(current []byte) ([]byte, error) {
	text := string(current)
	begin, end := strings.Index(text, m.sectionBegin()), strings.Index(text, m.sectionEnd())
	switch {
	case begin < 0 && end < 0:
		return current, nil
	case begin < 0 || end < begin:
		return nil, m.unbalanced()
	}
	before := strings.TrimRight(text[:begin], "\n")
	after := strings.TrimLeft(text[end+len(m.sectionEnd()):], "\n")
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

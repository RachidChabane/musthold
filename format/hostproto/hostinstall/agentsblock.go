package hostinstall

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

// agentsBlock is the module's block of AGENTS.md.
func (m Module) agentsBlock() markedBlock {
	return markedBlock{begin: m.sectionBegin(), end: m.sectionEnd(), text: m.agentsSection()}
}

// setAgentsBlock puts the module's block in the file, replacing a previous copy
// of it and leaving the rest of the file alone.
func (m Module) setAgentsBlock(current []byte) ([]byte, error) { return m.agentsBlock().set(current) }

// WithAgentsBlocks returns the AGENTS.md that enabling each of the modules, in
// order, leaves in a project whose file held current: every module's block, with
// the rest of the file kept.
func WithAgentsBlocks(current []byte, modules ...Module) ([]byte, error) {
	content := current
	for _, module := range modules {
		next, err := module.setAgentsBlock(content)
		if err != nil {
			return nil, err
		}
		content = next
	}
	return content, nil
}

// AgentsPath is the file that holds the agents' instructions of a project.
const AgentsPath = agentsPath

// removeAgentsBlock takes the module's block out of the file, with the blank line
// that set it apart. A file that held nothing else comes out empty.
func (m Module) removeAgentsBlock(current []byte) ([]byte, error) {
	return m.agentsBlock().remove(current)
}

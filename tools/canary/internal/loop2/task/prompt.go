package task

import (
	"fmt"
	"strings"
)

// The sentences a task's prompt follows: loop-1's, for two criteria, and with
// the revision wording when one of them is revised. Each has four parts: the
// criteria, the spec path, a statement in a few words and the package.
const (
	newHead     = "Implement requirements %s and %s of %s, "
	newTail     = ", in %s/, and add tests that verify them. Tell me when the task is done."
	revisedHead = "Implement the revised requirement %s and the new requirement %s of %s, "
	revisedTail = ", in %s/, and update and add the tests that verify them. Tell me when the task is done."
	// MaxStatementWords bounds the statement of a prompt.
	MaxStatementWords = 12
)

// The prompt templates with their parts named, as the author is told them.
const (
	NewPromptTemplate     = "Implement requirements <ID1> and <ID2> of <spec path>, <statement in a few words>, in <package>/, and add tests that verify them. Tell me when the task is done."
	RevisedPromptTemplate = "Implement the revised requirement <revised ID> and the new requirement <new ID> of <spec path>, <statement in a few words>, in <package>/, and update and add the tests that verify them. Tell me when the task is done."
)

// promptFrame is the head and the tail of the task's prompt around its
// statement, and the template it follows.
func (t Task) promptFrame() (head, tail, template string) {
	if revised, found := t.Revised(); found {
		fresh := t.New()[0]
		return fmt.Sprintf(revisedHead, revised.ID, fresh.ID, t.Spec), fmt.Sprintf(revisedTail, t.Package), RevisedPromptTemplate
	}
	ids := t.IDs()
	return fmt.Sprintf(newHead, ids[0], ids[1], t.Spec), fmt.Sprintf(newTail, t.Package), NewPromptTemplate
}

// CheckPrompt reports how the task's prompt departs from its template, or nil.
// The task's criteria must be valid.
func (t Task) CheckPrompt() error {
	head, tail, template := t.promptFrame()
	prompt := t.Prompt
	if !strings.HasPrefix(prompt, head) || !strings.HasSuffix(prompt, tail) || len(prompt) < len(head)+len(tail) {
		return fmt.Errorf("the prompt is not %q", template)
	}
	statement := prompt[len(head) : len(prompt)-len(tail)]
	if strings.TrimSpace(statement) == "" || strings.ContainsAny(statement, "\r\n") {
		return fmt.Errorf("the statement of the prompt must be a few words on one line")
	}
	if words := len(strings.Fields(statement)); words > MaxStatementWords {
		return fmt.Errorf("the statement of the prompt has %d words, at most %d are a few", words, MaxStatementWords)
	}
	return nil
}

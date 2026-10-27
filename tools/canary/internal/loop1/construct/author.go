package construct

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The task whose fixture and reference are fixed (PREREG §3.2).
const (
	BoxofficeName      = "boxoffice"
	BoxofficeFixture   = "demo/loop/fixture/boxoffice"
	BoxofficeReference = "tools/canary/internal/demoloop/fixturegate/testdata/task/boxoffice"
	boxofficeSpec      = "specs/tickets.md"
	boxofficeSource    = "boxoffice/prices.py"
	boxofficeCriterion = "BOX-4"
	boxofficeModule    = "boxoffice.prices"
	boxofficeOperation = "ticket_price"
	boxofficeSignature = "ticket_price(age)"
	fixturePrefix      = "fixture/"
)

// BoxofficeTask is the description of the boxoffice task: its prompt and
// operations are those of the run configuration, the rest is fixed.
func BoxofficeTask(configData []byte) (study.Task, error) {
	var document struct {
		Tasks []config.Task `json:"tasks"`
	}
	if err := json.Unmarshal(configData, &document); err != nil {
		return study.Task{}, fmt.Errorf("read the run configuration: %w", err)
	}
	for _, entry := range document.Tasks {
		if entry.Name != BoxofficeName {
			continue
		}
		task := accept.WithStandardPaths(study.Task{
			Name: BoxofficeName, Spec: boxofficeSpec, Source: boxofficeSource, Criterion: boxofficeCriterion,
			Prompt: entry.Prompt, Operations: entry.Operations,
			Interface: study.Interface{Module: boxofficeModule, Operation: boxofficeOperation, Signature: boxofficeSignature},
		})
		task.Fixture, task.Truth.Reference = BoxofficeFixture, BoxofficeReference
		return task, nil
	}
	return study.Task{}, fmt.Errorf("the run configuration lists no task %s", BoxofficeName)
}

// authorRole is what differs between the two ways of calling the author: a whole
// task, or the violations and refactors of a task whose project is fixed.
type authorRole interface {
	// prompt renders the call, with the files of the previous answer and the
	// problems found in it when repairing.
	prompt(previous []File, problems []string) (string, error)
	// apply checks a set of files and writes them into the store.
	apply(store Store, files []File) error
	// tests returns what the test writer is told: the spec and the interface.
	tests(store Store) (specText string, iface study.Interface, err error)
	// fixed reports whether the fixture and the reference cannot change.
	fixed() bool
	name() string
}

// wholeTask has the author write a fixture, a reference, violations, refactors
// and the task description.
type wholeTask struct {
	domain        Domain
	prompts       Prompts
	examplePrompt string
	example       []File
}

func (w wholeTask) name() string { return w.domain.Name }
func (w wholeTask) fixed() bool  { return false }

func (w wholeTask) prompt(previous []File, problems []string) (string, error) {
	return w.prompts.Author(AuthorInput{Domain: w.domain, ExamplePrompt: w.examplePrompt, Example: w.example, Previous: previous, Problems: problems})
}

func (w wholeTask) apply(store Store, files []File) error {
	if err := CheckAuthorFiles(files); err != nil {
		return err
	}
	description, _ := findFile(files, study.TaskFile)
	task, err := NormaliseTask(description.Content, w.domain.Name)
	if err != nil {
		return err
	}
	return store.WriteAuthor(w.domain.Name, files, task)
}

func (w wholeTask) tests(store Store) (string, study.Interface, error) {
	data, err := store.Read(w.domain.Name, study.TaskFile)
	if err != nil {
		return "", study.Interface{}, fmt.Errorf("%w: %v", ErrBadOutput, err)
	}
	task, err := study.ParseTask(data)
	if err != nil {
		return "", study.Interface{}, fmt.Errorf("%w: %v", ErrBadOutput, err)
	}
	spec, err := store.Read(w.domain.Name, path.Join(accept.FixtureSubdir, task.Spec))
	if err != nil {
		return "", study.Interface{}, fmt.Errorf("%w: the spec %s is missing: %v", ErrBadOutput, task.Spec, err)
	}
	return string(spec), task.Interface, nil
}

// variantsOnly has the author write the violations and refactors of the fixed
// task from its spec and reference source.
type variantsOnly struct {
	task      study.Task
	prompts   Prompts
	reference []File
	spec      string
}

func (v variantsOnly) name() string { return v.task.Name }
func (v variantsOnly) fixed() bool  { return true }

func (v variantsOnly) prompt(previous []File, problems []string) (string, error) {
	return v.prompts.Variants(VariantsInput{
		Spec: v.spec, Source: v.task.Source, ModuleFile: path.Base(v.task.Source),
		Reference: v.reference, Previous: previous, Problems: problems,
	})
}

func (v variantsOnly) apply(store Store, files []File) error {
	if err := CheckVariantFiles(files); err != nil {
		return err
	}
	return store.WriteVariants(v.task.Name, files)
}

func (v variantsOnly) tests(Store) (string, study.Interface, error) {
	return v.spec, v.task.Interface, nil
}

// unfixable reports whether a finding asks for a change that the role cannot
// make because the fixture and the reference are fixed.
func unfixable(author authorRole, finding records.Finding) bool {
	if !author.fixed() || finding.Role != records.RoleAuthor {
		return false
	}
	return slices.Contains([]records.Target{records.TargetFixture, records.TargetReference, records.TargetTask}, finding.Target)
}

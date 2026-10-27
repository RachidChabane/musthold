package construct

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// ErrTooFewTasks is returned when the domains are exhausted before enough of
// them pass acceptance.
var ErrTooFewTasks = errors.New("fewer domains passed acceptance than the study needs")

// Summary is what construction produced.
type Summary struct {
	// Accepted lists the tasks that passed acceptance in this run, in order.
	Accepted []string
	// Dropped lists the domains that failed it.
	Dropped []records.Drop
}

// Run builds the fixed task, then the domains in order, until PassTarget of them
// have passed acceptance. A domain whose task directory is already built and
// listed in the run configuration counts as passed.
func (c *Constructor) Run(ctx context.Context) (Summary, error) {
	var summary Summary
	listed, err := c.listedTasks()
	if err != nil {
		return summary, err
	}
	if !c.built(BoxofficeName, listed) {
		if err := c.BuildFixed(ctx); err != nil {
			return summary, err
		}
	}
	target := c.target()
	passed := 0
	for _, domain := range c.Domains {
		if passed >= target {
			break
		}
		if c.built(domain.Name, listed) {
			passed++
			continue
		}
		outcome, err := c.buildDomain(ctx, domain)
		if err != nil {
			return summary, err
		}
		if outcome.drop != nil {
			summary.Dropped = append(summary.Dropped, *outcome.drop)
			continue
		}
		summary.Accepted = append(summary.Accepted, domain.Name)
		passed++
	}
	if passed < target {
		return summary, fmt.Errorf("%w: %d of %d", ErrTooFewTasks, passed, target)
	}
	return summary, nil
}

// target is how many domains must pass: PassTarget unless the constructor sets
// another number.
func (c *Constructor) target() int {
	if c.Target > 0 {
		return c.Target
	}
	return PassTarget
}

// built reports whether a task is complete on disk and listed in the run
// configuration.
func (c *Constructor) built(name string, listed []string) bool {
	return slices.Contains(listed, name) && c.Store.Complete(name)
}

func (c *Constructor) listedTasks() ([]string, error) { return accept.ListedTasks(c.Config) }

// BuildFixed writes the violations, refactors and tests of the task whose
// fixture and reference are fixed. It stops with ErrFixtureAmendment when a check
// can only be satisfied by changing either, and with ErrFixedTaskDropped when the
// task fails acceptance after its repairs.
func (c *Constructor) BuildFixed(ctx context.Context) error {
	data, err := os.ReadFile(c.Config)
	if err != nil {
		return fmt.Errorf("read the run configuration: %w", err)
	}
	task, err := BoxofficeTask(data)
	if err != nil {
		return err
	}
	if err := c.Store.Remove(task.Name); err != nil {
		return err
	}
	if err := c.Store.WriteTask(task); err != nil {
		return err
	}
	reference, err := c.Store.ReadFiles(BoxofficeReference, "")
	if err != nil {
		return err
	}
	spec, err := os.ReadFile(c.Store.abs(path.Join(BoxofficeFixture, task.Spec)))
	if err != nil {
		return fmt.Errorf("read the spec of %s: %w", task.Name, err)
	}
	role := variantsOnly{task: task, prompts: c.Prompts, reference: reference, spec: string(spec)}
	c.progress("%s: writing the violations and refactors", task.Name)
	outcome, err := newBuilder(c, role).run(ctx)
	if err == nil && outcome.accepted {
		return nil
	}
	if removed := c.Store.Remove(task.Name); removed != nil {
		return removed
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s: %s", ErrFixedTaskDropped, outcome.drop.Check, outcome.drop.Reason)
}

func (c *Constructor) buildDomain(ctx context.Context, domain Domain) (outcome, error) {
	data, err := os.ReadFile(c.Config)
	if err != nil {
		return outcome{}, fmt.Errorf("read the run configuration: %w", err)
	}
	example, err := BoxofficeTask(data)
	if err != nil {
		return outcome{}, err
	}
	files, err := c.Store.ReadFiles(BoxofficeFixture, fixturePrefix)
	if err != nil {
		return outcome{}, err
	}
	role := wholeTask{domain: domain, prompts: c.Prompts, examplePrompt: example.Prompt, example: files}
	if err := c.Store.Remove(domain.Name); err != nil {
		return outcome{}, err
	}
	c.progress("%s: writing the task", domain.Name)
	result, err := newBuilder(c, role).run(ctx)
	if err != nil {
		return outcome{}, err
	}
	if result.accepted {
		c.progress("%s: accepted", domain.Name)
		return result, nil
	}
	c.progress("%s: dropped at %s", domain.Name, result.drop.Check)
	return result, c.Store.Remove(domain.Name)
}

// NewConstructor builds the constructor of a repository from the flags of the
// verb and the adapters of the acceptance checks.
func NewConstructor(flags accept.Flags, wiring accept.Wiring, model Model, progress io.Writer) (*Constructor, error) {
	prompts, err := NewPrompts()
	if err != nil {
		return nil, err
	}
	writer := records.NewFileWriter(flags.Layout())
	return &Constructor{
		Repo: flags.Repo, Config: flags.ConfigPath(), Store: Store{Repo: flags.Repo},
		Model: model, Acceptor: accept.NewAcceptor(flags, wiring, writer),
		Records: writer, Prompts: prompts, Domains: DomainOrder(), Now: time.Now, Progress: progress,
	}, nil
}

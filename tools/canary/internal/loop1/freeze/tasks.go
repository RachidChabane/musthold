package freeze

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The keys of a task's truth digests.
const (
	TruthOracle     = "oracle"
	TruthReference  = "reference"
	TruthViolations = "violations"
	TruthRefactors  = "refactors"
)

func (r Resolver) tasks(ctx context.Context, manifest product.Manifest) ([]study.TaskInputs, error) {
	var tasks []study.TaskInputs
	for _, configured := range r.Config.Tasks {
		entry, err := r.task(ctx, configured, manifest)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", configured.Name, err)
		}
		tasks = append(tasks, entry)
	}
	return tasks, nil
}

func (r Resolver) task(ctx context.Context, configured config.Task, manifest product.Manifest) (study.TaskInputs, error) {
	description, err := r.description(configured)
	if err != nil {
		return study.TaskInputs{}, err
	}
	fixture, err := r.treeDigest(description.Fixture)
	if err != nil {
		return study.TaskInputs{}, err
	}
	truth, err := r.truthDigests(description.Truth)
	if err != nil {
		return study.TaskInputs{}, err
	}
	entry := study.TaskInputs{
		Name:         configured.Name,
		FixtureTree:  fixture,
		Criterion:    description.Criterion,
		PromptSHA256: digestOf([]byte(configured.Prompt)),
		EquippedTree: map[string]string{},
		Instructions: map[string]study.ArmInstructions{},
		Truth:        truth,
	}
	for _, arm := range r.Config.Arms {
		prepared, err := r.Preparer.Prepare(ctx, configured, arm)
		if err != nil {
			return study.TaskInputs{}, fmt.Errorf("arm %s: %w", arm, err)
		}
		if entry.BaseCommit != "" && entry.BaseCommit != prepared.BaseCommit {
			return study.TaskInputs{}, fmt.Errorf("%w: the arms start from different commits", ErrTask)
		}
		entry.BaseCommit = prepared.BaseCommit
		entry.EquippedTree[string(arm)] = prepared.EquippedTree
		entry.Instructions[string(arm)] = study.ArmInstructions{
			Prompt:          entry.PromptSHA256,
			Files:           maps.Clone(prepared.Files),
			SessionSettings: prepared.SessionSettings,
			Plugins:         maps.Clone(manifest.Plugins),
		}
	}
	return entry, nil
}

// description reads the task's task.json and checks it against the configuration
// the runs use.
func (r Resolver) description(configured config.Task) (study.Task, error) {
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path.Join(TasksDir, configured.Name, study.TaskFile))))
	if err != nil {
		return study.Task{}, err
	}
	description, err := study.ParseTask(data)
	if err != nil {
		return study.Task{}, err
	}
	if description.Name != configured.Name || description.Fixture != configured.Fixture || description.Prompt != configured.Prompt {
		return study.Task{}, fmt.Errorf("%w: %s and the configuration differ in name, fixture or prompt", ErrTask, study.TaskFile)
	}
	return description, nil
}

func (r Resolver) truthDigests(truth study.Truth) (map[string]string, error) {
	digests := map[string]string{}
	for key, directory := range map[string]string{
		TruthOracle: truth.Oracle, TruthReference: truth.Reference,
		TruthViolations: truth.Violations, TruthRefactors: truth.Refactors,
	} {
		digest, err := r.treeDigest(directory)
		if err != nil {
			return nil, err
		}
		digests[key] = digest
	}
	return digests, nil
}

func (r Resolver) treeDigest(directory string) (string, error) {
	return product.TreeDigest(os.DirFS(filepath.Join(r.Root, filepath.FromSlash(directory))))
}

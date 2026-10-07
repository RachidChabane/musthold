package mutantrun_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

const syntaxError = "def f():\n    SYNTAX"

var errUse = errors.New("the use failed")

// usage records how a use callback was called.
type usage struct {
	calls         int
	workspace     ecosystem.Workspace
	source        string
	releasedInUse []string
}

func (u *usage) callback(workspaces *memoryWorkspaces, result error) func(context.Context, ecosystem.Workspace) error {
	return func(_ context.Context, mutant ecosystem.Workspace) error {
		u.calls++
		u.workspace = mutant
		u.source = string(workspaces.files[mutant.Dir][sourceFile])
		u.releasedInUse = slices.Clone(workspaces.released)
		return result
	}
}

func TestMutate(t *testing.T) {
	ctx := context.Background()

	t.Run("a mutant that splices and builds is handed to the use live and released afterwards [verifies SP-194~1]", func(t *testing.T) {
		executor, reference, _, workspaces := setup(t)
		var used usage
		status, err := executor.Mutate(ctx, reference, request(mutated).Edits, used.callback(workspaces, nil))
		if err != nil || !status.Built || status.Diagnostics != "" {
			t.Fatalf("status %+v, error %v", status, err)
		}
		if used.calls != 1 || used.source != mutated || used.workspace.Dir == reference.Workspace.Dir {
			t.Fatalf("use %+v", used)
		}
		if len(used.releasedInUse) != 0 || !slices.Equal(workspaces.released, []string{used.workspace.Dir}) {
			t.Fatalf("released during the use %v, after %v", used.releasedInUse, workspaces.released)
		}
		if string(workspaces.files[reference.Workspace.Dir][sourceFile]) != original {
			t.Fatal("the reference workspace was changed")
		}
	})

	t.Run("the error of the use is returned and the workspace is still released [verifies SP-194~1]", func(t *testing.T) {
		executor, reference, _, workspaces := setup(t)
		var used usage
		status, err := executor.Mutate(ctx, reference, request(mutated).Edits, used.callback(workspaces, errUse))
		if !errors.Is(err, errUse) || !status.Built || len(workspaces.released) != 1 {
			t.Fatalf("status %+v, error %v, released %v", status, err, workspaces.released)
		}
	})

	failures := []struct {
		name        string
		source      string
		diagnostics string
	}{
		{"a splice that does not parse", syntaxError, "syntax error"},
		{"a build that fails", broken, "NameError"},
	}
	for _, tt := range failures {
		t.Run(tt.name+" is reported in the status without calling the use [verifies SP-194~1]", func(t *testing.T) {
			executor, reference, _, workspaces := setup(t)
			var used usage
			status, err := executor.Mutate(ctx, reference, request(tt.source).Edits, used.callback(workspaces, nil))
			if err != nil || status.Built || status.Diagnostics == "" {
				t.Fatalf("status %+v, error %v", status, err)
			}
			if tt.name == failures[1].name && status.Diagnostics != tt.diagnostics {
				t.Fatalf("diagnostics %q", status.Diagnostics)
			}
			if used.calls != 0 {
				t.Fatalf("the use was called %d times", used.calls)
			}
			if len(workspaces.released) != 1 {
				t.Fatalf("released %v", workspaces.released)
			}
		})
	}

	t.Run("a path the policy refuses is an error before any clone and the use is not called [verifies SP-194~1]", func(t *testing.T) {
		executor, reference, _, workspaces := setup(t)
		edits := request(mutated).Edits
		edits.Edits[0].File = testFile
		var used usage
		_, err := executor.Mutate(ctx, reference, edits, used.callback(workspaces, nil))
		if !mutation.IsRefusal(err) || used.calls != 0 || workspaces.next != 0 {
			t.Fatalf("error %v, calls %d, clones %d", err, used.calls, workspaces.next)
		}
	})
}

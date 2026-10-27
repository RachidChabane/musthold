package golang_test

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
)

func TestWorkspaceModules(t *testing.T) {
	t.Run("a workspace file lists the modules of its use directives in their order [verifies SP-2425~2]", func(t *testing.T) {
		tree := fstest.MapFS{"go.work": {Data: []byte("go 1.26\n\nuse ./zeta\n\nuse (\n\t./alpha\n\tnested/beta/\n)\n")}}
		modules, found, err := golang.WorkspaceModules(tree)
		if err != nil || !found {
			t.Fatalf("found %v, err %v", found, err)
		}
		if want := []string{"zeta", "alpha", "nested/beta"}; !reflect.DeepEqual(modules, want) {
			t.Fatalf("modules %q, want %q", modules, want)
		}
	})
	t.Run("a tree without a workspace file holds no workspace [verifies SP-2425~2]", func(t *testing.T) {
		modules, found, err := golang.WorkspaceModules(fstest.MapFS{"go.mod": {Data: []byte("module example.com/m\n")}})
		if err != nil || found || modules != nil {
			t.Fatalf("modules %q, found %v, err %v", modules, found, err)
		}
	})
	t.Run("a workspace file that does not parse is an error [verifies SP-2425~2]", func(t *testing.T) {
		if _, found, err := golang.WorkspaceModules(fstest.MapFS{"go.work": {Data: []byte("use (\n")}}); err == nil || !found {
			t.Fatalf("found %v, err %v", found, err)
		}
	})
}

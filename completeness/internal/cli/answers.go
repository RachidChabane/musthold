package cli

import (
	"path/filepath"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/pendingfile"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/responsecheck"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// pendingLockFile serializes the writers of the pending files, in the host
// directory outside the work tree.
const pendingLockFile = "pending.lock"

// pendingDirectory is the pending directory of the project, locked through the
// host directory of its store.
func pendingDirectory(root string, store *hoststore.Store) pendingfile.Directory {
	return pendingfile.Directory{Root: root, Lock: filepath.Join(store.Dir(), pendingLockFile)}
}

// newRequestService is the request service of the project's host store: every
// answer it accepts is tallied in the module's pending file.
func (e Env) newRequestService(root string, store *hoststore.Store) requests.Service {
	return requests.Service{
		Plugin: pluginName, Store: store, Validator: responsecheck.New(), Clock: e.Now,
		Tally: pendingfile.Tally{Directory: pendingDirectory(root, store), Module: pendingModule},
	}
}

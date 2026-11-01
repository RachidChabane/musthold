package cli

import (
	"path/filepath"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/alignment/internal/adapters/pendingfile"
	"github.com/RachidChabane/musthold/alignment/internal/adapters/responsecheck"
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

// newRequestService is the request service of the project's host store.
func (e Env) newRequestService(store *hoststore.Store) requests.Service {
	return requests.Service{Plugin: pluginName, Store: store, Validator: responsecheck.New(), Clock: e.Now}
}

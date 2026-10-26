package executorhost

import (
	"context"
	"errors"

	"github.com/RachidChabane/musthold/format/hostproto/mcpstdio"
)

var _ mcpstdio.ExecutorService = (*LazyService)(nil)

// LazyService is an executor service whose project is learned from the client's
// roots at the first tool call. A server a client of the Agent Plugins standard
// starts in the plugin root has no other way to find the project it serves.
// Like the server that calls it, it is not safe for concurrent use.
type LazyService struct {
	// Build makes the service of the project from the local directories of the
	// client's roots, empty when the client declared none or listed none.
	Build func(roots []string) (Service, error)

	built *Service
}

// ErrNoProject is returned when the directory a portable server resolved holds
// no configuration of the module.
var ErrNoProject = errors.New("no project of musthold here: start the client in the project directory, or pass --root")

// PickRoot chooses the project directory of a server: the directory given, then
// the one the client exports, then the first root the client lists. It returns
// an empty string when none of them is known, which stands for the working
// directory.
func PickRoot(given, exported string, roots []string) string {
	switch {
	case given != "":
		return given
	case exported != "":
		return exported
	case len(roots) > 0:
		return roots[0]
	default:
		return ""
	}
}

// resolve builds the service on first use; a failure is not remembered, so the
// next call tries again.
func (l *LazyService) resolve(ctx context.Context) (Service, error) {
	if l.built != nil {
		return *l.built, nil
	}
	var directories []string
	if roots, ok := mcpstdio.RootsFrom(ctx); ok {
		listed, err := roots.List(ctx)
		if err != nil {
			return Service{}, err
		}
		directories = listed
	}
	service, err := l.Build(directories)
	if err != nil {
		return Service{}, err
	}
	l.built = &service
	return service, nil
}

// FetchRequest implements mcpstdio.ExecutorService.
func (l *LazyService) FetchRequest(ctx context.Context, args mcpstdio.FetchArgs) (mcpstdio.FetchResult, error) {
	service, err := l.resolve(ctx)
	if err != nil {
		return mcpstdio.FetchResult{}, err
	}
	return service.FetchRequest(ctx, args)
}

// SubmitResponse implements mcpstdio.ExecutorService.
func (l *LazyService) SubmitResponse(ctx context.Context, args mcpstdio.SubmitArgs) (mcpstdio.SubmitResult, error) {
	service, err := l.resolve(ctx)
	if err != nil {
		return mcpstdio.SubmitResult{}, err
	}
	return service.SubmitResponse(ctx, args)
}

// Drain implements mcpstdio.ExecutorService.
func (l *LazyService) Drain(ctx context.Context, args mcpstdio.DrainArgs, sampler mcpstdio.Sampler) (mcpstdio.DrainResult, error) {
	service, err := l.resolve(ctx)
	if err != nil {
		return mcpstdio.DrainResult{}, err
	}
	return service.Drain(ctx, args, sampler)
}

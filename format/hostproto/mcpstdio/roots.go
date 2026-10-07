package mcpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	fileScheme    = "file://"
	localHostName = "localhost"
	percentSign   = '%'
	hexDigits     = 2
)

// ErrRootsRefused is returned when the client answers a roots request with an
// error or with something that is not a list of roots.
var ErrRootsRefused = errors.New("roots refused by the client")

// Roots asks the client for the roots of its session. A Roots is valid only
// during the tool call it was given to.
type Roots interface {
	// List returns the local directories of the client's `file://` roots, in the
	// order the client lists them.
	List(ctx context.Context) ([]string, error)
}

type rootsKey struct{}

// WithRoots returns a context that carries the session's roots.
func WithRoots(ctx context.Context, roots Roots) context.Context {
	return context.WithValue(ctx, rootsKey{}, roots)
}

// RootsFrom returns the roots of the session a tool call runs in, and false when
// the client declared no roots capability.
func RootsFrom(ctx context.Context) (Roots, bool) {
	roots, ok := ctx.Value(rootsKey{}).(Roots)
	return roots, ok
}

// sessionRoots implements Roots over the session's streams.
type sessionRoots struct{ server *Server }

// List implements Roots: it sends roots/list and reads until the response arrives.
func (r sessionRoots) List(ctx context.Context) ([]string, error) {
	s := r.server
	s.nextID++
	id := json.RawMessage(strconv.Quote(rootsIDPrefix + strconv.Itoa(s.nextID)))
	if err := s.writer.WriteMessage(Message{ID: id, Method: MethodRootsList}); err != nil {
		return nil, err
	}
	response, err := s.awaitResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return rootDirectories(response)
}

func rootDirectories(response Message) ([]string, error) {
	if response.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRootsRefused, response.Error.Message)
	}
	var result struct {
		Roots []struct {
			URI string `json:"uri"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRootsRefused, err)
	}
	var directories []string
	for _, root := range result.Roots {
		if directory, ok := localPath(root.URI); ok {
			directories = append(directories, directory)
		}
	}
	return directories, nil
}

// localPath is the directory a `file://` URI names on this machine, and false for
// any other URI. The package links no networking, so the URI is read by hand: an
// empty or `localhost` authority, then a percent-encoded absolute path.
func localPath(uri string) (string, bool) {
	rest, isFile := strings.CutPrefix(uri, fileScheme)
	if !isFile {
		return "", false
	}
	authority, path, hasPath := strings.Cut(rest, "/")
	if !hasPath || (authority != "" && authority != localHostName) {
		return "", false
	}
	decoded, ok := percentDecode("/" + path)
	if !ok || decoded == "/" {
		return "", false
	}
	return decoded, true
}

func percentDecode(text string) (string, bool) {
	var out strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] != percentSign {
			out.WriteByte(text[index])
			continue
		}
		if index+hexDigits >= len(text) {
			return "", false
		}
		value, err := strconv.ParseUint(text[index+1:index+1+hexDigits], 16, 8)
		if err != nil {
			return "", false
		}
		out.WriteByte(byte(value))
		index += hexDigits
	}
	return out.String(), true
}

// Package egress is the logging proxy that is a live session's only way out: an
// HTTP CONNECT proxy that forwards to the allowlisted endpoints, refuses every
// other request, and writes one JSON line per request to its log. An allowlist
// that holds container.AllowAll forwards every CONNECT, and still logs each.
package egress

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
)

// Bounds of the proxy.
const (
	dialTimeout   = 30 * time.Second
	headerTimeout = 30 * time.Second
	logFileMode   = 0o644
)

// Decisions the log records.
const (
	DecisionAllowed = "allowed"
	DecisionDenied  = "denied"
	DecisionFailed  = "failed"
)

// Entry is one line of the log.
type Entry struct {
	Time     string `json:"time"`
	Method   string `json:"method"`
	Target   string `json:"target"`
	Decision string `json:"decision"`
	Detail   string `json:"detail,omitempty"`
}

// Proxy forwards CONNECT requests to the allowlisted endpoints.
type Proxy struct {
	Allow []string
	log   *json.Encoder
	mutex sync.Mutex
	now   func() time.Time
	dial  func(network, address string) (net.Conn, error)
}

// New opens the log for appending and returns the proxy.
func New(allow []string, logPath string) (*Proxy, io.Closer, error) {
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFileMode)
	if err != nil {
		return nil, nil, err
	}
	return NewWithLog(allow, file), file, nil
}

// NewWithLog returns a proxy that writes its log to the writer.
func NewWithLog(allow []string, log io.Writer) *Proxy {
	return &Proxy{
		Allow: allow, log: json.NewEncoder(log), now: time.Now,
		dial: func(network, address string) (net.Conn, error) { return net.DialTimeout(network, address, dialTimeout) },
	}
}

// Serve accepts connections until the listener closes.
func (p *Proxy) Serve(listener net.Listener) error {
	server := &http.Server{Handler: p, ReadHeaderTimeout: headerTimeout}
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (p *Proxy) record(method, target, decision, detail string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	_ = p.log.Encode(Entry{Time: p.now().UTC().Format(time.RFC3339Nano), Method: method, Target: target, Decision: decision, Detail: detail})
}

// ServeHTTP forwards a CONNECT to an allowlisted endpoint and refuses the rest.
func (p *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	target := request.Host
	if request.Method != http.MethodConnect {
		target = request.URL.String()
		p.record(request.Method, target, DecisionDenied, "only CONNECT is forwarded")
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	if !p.allows(target) {
		p.record(request.Method, target, DecisionDenied, "not allowlisted")
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	upstream, err := p.dial("tcp", target)
	if err != nil {
		p.record(request.Method, target, DecisionFailed, err.Error())
		http.Error(writer, "bad gateway", http.StatusBadGateway)
		return
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(writer, "no hijacking", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	p.record(request.Method, target, DecisionAllowed, "")
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	splice(client, buffered, upstream)
}

// allows reports whether the proxy forwards a CONNECT to the target.
func (p *Proxy) allows(target string) bool {
	return slices.Contains(p.Allow, container.AllowAll) || slices.Contains(p.Allow, target)
}

// splice copies both ways until either side closes.
func splice(client net.Conn, buffered *bufio.ReadWriter, upstream net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffered)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
	client.Close()
	upstream.Close()
	<-done
}

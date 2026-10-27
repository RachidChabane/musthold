package egress_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/egress"
)

// syncBuffer is a log the proxy and the test share.
type syncBuffer struct {
	mutex sync.Mutex
	bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.Buffer.Write(p)
}

func (b *syncBuffer) entries(t *testing.T) []egress.Entry {
	t.Helper()
	b.mutex.Lock()
	defer b.mutex.Unlock()
	var entries []egress.Entry
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		var entry egress.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

// connect asks the proxy to CONNECT to the target and returns the status code and the tunnel.
func connect(t *testing.T, proxy, target string) (int, net.Conn) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", proxy, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, connection
}

func TestProxy(t *testing.T) {
	upstream := listen(t)
	go func() {
		for {
			connection, err := upstream.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				line, _ := bufio.NewReader(connection).ReadString('\n')
				fmt.Fprint(connection, "echo "+line)
			}()
		}
	}()
	var log syncBuffer
	proxyListener := listen(t)
	proxy := egress.NewWithLog([]string{upstream.Addr().String()}, &log)
	go func() { _ = proxy.Serve(proxyListener) }()

	t.Run("a CONNECT to an allowlisted endpoint is tunnelled and logged as allowed [verifies SP-2415~1]", func(t *testing.T) {
		status, tunnel := connect(t, proxyListener.Addr().String(), upstream.Addr().String())
		defer tunnel.Close()
		if status != http.StatusOK {
			t.Fatalf("status %d", status)
		}
		fmt.Fprint(tunnel, "hello\n")
		reply, _ := bufio.NewReader(tunnel).ReadString('\n')
		if reply != "echo hello\n" {
			t.Errorf("reply %q", reply)
		}
	})

	t.Run("any other host and any request but CONNECT are refused and logged as denied [verifies SP-2415~1]", func(t *testing.T) {
		status, tunnel := connect(t, proxyListener.Addr().String(), "example.com:443")
		tunnel.Close()
		if status != http.StatusForbidden {
			t.Errorf("CONNECT status %d", status)
		}
		response, err := (&http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(mustURL(t, "http://"+proxyListener.Addr().String()))}}).Get("http://example.com/")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("GET status %d", response.StatusCode)
		}
		decisions := map[string]string{}
		for _, entry := range log.entries(t) {
			decisions[entry.Method+" "+entry.Target] = entry.Decision
		}
		want := map[string]string{
			"CONNECT " + upstream.Addr().String(): egress.DecisionAllowed,
			"CONNECT example.com:443":             egress.DecisionDenied,
			"GET http://example.com/":             egress.DecisionDenied,
		}
		for request, decision := range want {
			if decisions[request] != decision {
				t.Errorf("%s: %q, want %q (log %v)", request, decisions[request], decision, decisions)
			}
		}
	})
}

func TestAllowAll(t *testing.T) {
	upstream := listen(t)
	go func() {
		for {
			connection, err := upstream.Accept()
			if err != nil {
				return
			}
			connection.Close()
		}
	}()
	var log syncBuffer
	proxyListener := listen(t)
	proxy := egress.NewWithLog([]string{container.AllowAll}, &log)
	go func() { _ = proxy.Serve(proxyListener) }()

	t.Run("in the allow-all mode every CONNECT is forwarded and logged with its host, and any other request is still refused [verifies SP-2516~1]", func(t *testing.T) {
		status, tunnel := connect(t, proxyListener.Addr().String(), upstream.Addr().String())
		tunnel.Close()
		if status != http.StatusOK {
			t.Fatalf("status %d", status)
		}
		response, err := (&http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(mustURL(t, "http://"+proxyListener.Addr().String()))}}).Get("http://example.com/")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		entries := log.entries(t)
		if len(entries) != 2 || entries[0].Target != upstream.Addr().String() || entries[0].Decision != egress.DecisionAllowed || entries[1].Decision != egress.DecisionDenied {
			t.Errorf("log %+v", entries)
		}
	})
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

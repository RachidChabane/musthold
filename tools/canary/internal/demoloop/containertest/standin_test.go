package containertest

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
)

// The stand-in for the model API: a TLS server in a container of the session
// image, on a network of its own that only the egress proxy joins, so the session
// reaches it through the proxy alone, as it reaches the real API.
const (
	standInHost     = "model-api"
	standInPort     = "8443"
	standInEndpoint = standInHost + ":" + standInPort
	standInScript   = "standin.py"
	standInCert     = "cert.pem"
	standInKey      = "key.pem"
	standInLog      = "api.log"
	// standInCertInRun is where the session's container finds the stand-in's
	// certificate, inside the run directory it mounts.
	standInCertInRun = plan.TmpDir + "/model-api.pem"
)

// standInSource answers a Messages request with one streamed reply, any other
// request with a not-found error, and logs each request with whether its bearer
// is the token, never the bearer itself.
const standInSource = `import json, ssl, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CERT, KEY, LOG, TOKEN, PORT = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5])

def events(model):
    yield "message_start", {"type": "message_start", "message": {"id": "msg_standin", "type": "message", "role": "assistant", "model": model, "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}}
    yield "content_block_start", {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}
    yield "content_block_delta", {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "ready"}}
    yield "content_block_stop", {"type": "content_block_stop", "index": 0}
    yield "message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 1}}
    yield "message_stop", {"type": "message_stop"}

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def record(self):
        with open(LOG, "a") as log:
            log.write(json.dumps({"method": self.command, "path": self.path, "bearerIsToken": self.headers.get("Authorization", "") == "Bearer " + TOKEN}) + "\n")

    def reply(self, status, payload):
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        body = json.loads(self.rfile.read(length) or b"{}") if length else {}
        self.record()
        if not self.path.startswith("/v1/messages") or "count_tokens" in self.path:
            return self.reply(404, {"type": "error", "error": {"type": "not_found_error", "message": "not served"}})
        model = body.get("model", "model")
        if not body.get("stream"):
            return self.reply(200, {"id": "msg_standin", "type": "message", "role": "assistant", "model": model, "content": [{"type": "text", "text": "ready"}], "stop_reason": "end_turn", "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}})
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        for name, payload in events(model):
            self.wfile.write(("event: %s\ndata: %s\n\n" % (name, json.dumps(payload))).encode())
            self.wfile.flush()
        self.close_connection = True

    def do_GET(self):
        self.record()
        self.reply(404, {"type": "error", "error": {"type": "not_found_error", "message": "not served"}})

server = ThreadingHTTPServer(("", PORT), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(CERT, KEY)
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
`

// standIn is one running stand-in and the network only it and the proxy join.
type standIn struct {
	dir, network, name string
	cert               []byte
}

// startStandIn starts the stand-in under the name given and removes it, with
// its network, when the test ends.
func (w world) startStandIn(t *testing.T, name string) standIn {
	t.Helper()
	s := standIn{dir: filepath.Join(w.work, name), network: "standin-out-" + name, name: "standin-api-" + name}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cert, key := selfSigned(t, standInHost)
	s.cert = cert
	for file, content := range map[string][]byte{standInScript: []byte(standInSource), standInCert: cert, standInKey: key, standInLog: nil} {
		if err := os.WriteFile(filepath.Join(s.dir, file), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		w.run(t, "", "rm", "--force", s.name)
		w.run(t, "", "network", "rm", s.network)
	})
	if _, stderr, code := w.run(t, "", "network", "create", s.network); code != 0 {
		t.Fatalf("network: %s", stderr)
	}
	argv := []string{
		"run", "--name", s.name, "--init", "--user", userOf(), "--read-only", "--cap-drop", "ALL", "--log-driver", "none",
		"--mount", "type=bind,source=" + s.dir + ",target=" + s.dir, "--network", s.network, "--network-alias", standInHost, "--detach", w.image,
		"python3", filepath.Join(s.dir, standInScript), filepath.Join(s.dir, standInCert), filepath.Join(s.dir, standInKey), filepath.Join(s.dir, standInLog), fakeToken, standInPort,
	}
	if _, stderr, code := w.run(t, "", argv...); code != 0 {
		t.Fatalf("stand-in: %s", stderr)
	}
	return s
}

// request is one request the stand-in logged.
type request struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	BearerIsToken bool   `json:"bearerIsToken"`
}

func (s standIn) requests(t *testing.T) []request {
	t.Helper()
	file, err := os.Open(filepath.Join(s.dir, standInLog))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var requests []request
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r request
		if json.Unmarshal(scanner.Bytes(), &r) == nil {
			requests = append(requests, r)
		}
	}
	return requests
}

// standInProcesses runs the runner's docker commands with the proxy's allowlist
// naming the stand-in instead of the model endpoints and, when the stand-in is to
// be reachable, the proxy joined to the stand-in's network.
type standInProcesses struct {
	inner     canary.Processes
	standIn   standIn
	reachable bool
	w         world
	t         *testing.T
}

func (p standInProcesses) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	if slices.Contains(spec.Argv, container.ProxySubcommand) {
		argv := slices.Clone(spec.Argv)
		for index := range argv {
			if index > 0 && argv[index-1] == "--allow" {
				argv[index] = standInEndpoint
			}
		}
		spec.Argv = argv
	}
	output, err := p.inner.Run(ctx, spec)
	if err == nil && p.reachable && isProxyConnect(spec.Argv) {
		proxy := spec.Argv[len(spec.Argv)-1]
		if _, stderr, code := p.w.run(p.t, "", "network", "connect", p.standIn.network, proxy); code != 0 {
			p.t.Errorf("the proxy did not join the stand-in's network: %s", stderr)
		}
	}
	return output, err
}

func isProxyConnect(argv []string) bool {
	line := strings.Join(argv, " ")
	return strings.Contains(line, " network connect --alias "+container.ProxyHost+" ")
}

// standInSessions runs the session with the pinned Claude Code addressing the
// stand-in and trusting its certificate, and without the plugin trees named.
type standInSessions struct {
	inner   canary.Sessions
	standIn standIn
	without []string
}

func (s standInSessions) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	runDir := filepath.Dir(filepath.Dir(spec.StdoutPath))
	certPath := filepath.Join(runDir, standInCertInRun)
	if err := os.WriteFile(certPath, s.standIn.cert, 0o644); err != nil {
		return canary.SessionOutcome{}, err
	}
	var argv []string
	for index := 0; index < len(spec.Argv); index++ {
		word := spec.Argv[index]
		if word == "--plugin-dir" && index+1 < len(spec.Argv) && slices.Contains(s.without, spec.Argv[index+1]) {
			index++
			continue
		}
		argv = append(argv, word)
		if word == "-i" && len(argv) > 1 && strings.HasSuffix(argv[len(argv)-2], "env") {
			argv = append(argv, "ANTHROPIC_BASE_URL=https://"+standInEndpoint, "NODE_EXTRA_CA_CERTS="+certPath)
		}
	}
	spec.Argv = argv
	return s.inner.Run(spec)
}

// userOf is the user every container of the test runs as: the tester's own.
func userOf() string { return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()) }

// selfSigned makes a certificate for the host and its key, in PEM.
func selfSigned(t *testing.T, host string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

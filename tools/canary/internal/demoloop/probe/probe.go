// Package probe looks, from inside a session's container, at what the session
// could reach: what HOME holds, whether an SSH agent or a docker socket is there,
// which sockets the file system offers, and which hosts answer directly or through
// the egress proxy.
package probe

import (
	"bufio"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Paths and bounds of the probe.
const (
	DockerSocket   = "/var/run/docker.sock"
	sshAgentEnv    = "SSH_AUTH_SOCK"
	dialTimeout    = 5 * time.Second
	proxyVariable  = "HTTPS_PROXY"
	connectLine    = "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	statusOKPrefix = "HTTP/1.1 200"
)

// skippedRoots are the virtual file systems the socket search leaves out.
var skippedRoots = []string{"/proc", "/sys", "/dev"}

// Report is what the probe found.
type Report struct {
	Home         string   `json:"home"`
	HomeEntries  []string `json:"homeEntries"`
	SSHAuthSock  string   `json:"sshAuthSock"`
	DockerSocket bool     `json:"dockerSocket"`
	Sockets      []string `json:"sockets"`
	// Direct maps each target dialled without the proxy to whether it answered.
	Direct map[string]bool `json:"direct"`
	// ViaProxy maps each target asked of the proxy to whether it was forwarded.
	ViaProxy map[string]bool `json:"viaProxy"`
}

// Run probes the container: it dials each direct target itself, and asks the
// proxy of HTTPS_PROXY for each proxied one.
func Run(direct, proxied []string) Report {
	report := Report{Home: os.Getenv("HOME"), SSHAuthSock: os.Getenv(sshAgentEnv), Direct: map[string]bool{}, ViaProxy: map[string]bool{}}
	if entries, err := os.ReadDir(report.Home); err == nil {
		for _, entry := range entries {
			report.HomeEntries = append(report.HomeEntries, entry.Name())
		}
	}
	if _, err := os.Stat(DockerSocket); err == nil {
		report.DockerSocket = true
	}
	report.Sockets = sockets("/")
	for _, target := range direct {
		connection, err := net.DialTimeout("tcp", target, dialTimeout)
		report.Direct[target] = err == nil
		if err == nil {
			connection.Close()
		}
	}
	for _, target := range proxied {
		report.ViaProxy[target] = throughProxy(os.Getenv(proxyVariable), target)
	}
	return report
}

// sockets lists the socket files under the root, leaving out the virtual file systems.
func sockets(root string) []string {
	found := []string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		for _, skipped := range skippedRoots {
			if path == skipped {
				return fs.SkipDir
			}
		}
		if entry.Type()&fs.ModeSocket != 0 {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// throughProxy asks the proxy to CONNECT to the target and reports whether it did.
func throughProxy(proxy, target string) bool {
	parsed, err := url.Parse(proxy)
	if err != nil || parsed.Host == "" {
		return false
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, dialTimeout)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(dialTimeout))
	if _, err := fmt.Fprintf(connection, connectLine, target, target); err != nil {
		return false
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		return false
	}
	response.Body.Close()
	return strings.HasPrefix(response.Proto+" "+response.Status, statusOKPrefix) || response.StatusCode == http.StatusOK
}

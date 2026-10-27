// Package container builds the docker commands that isolate every step of a live
// run: the steps that touch the project, the session, and the egress proxy that is
// the session's only way out. It is pure: it builds command lines and runs nothing,
// so a dry run prints exactly what a run starts.
//
// Every container runs as the user's uid with no capability, no new privilege, a
// read-only root file system, a scratch HOME on tmpfs, the limits of the demos-1
// engine, the run directory mounted read-write at its own path and the product tree
// read-only, an environment that `env -i` empties and refills from a fixed list,
// and no docker socket. The session's network is internal; its only route out is
// the proxy, which forwards only to the allowlisted model endpoints.
package container

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ErrMountPath refuses a mounted path that `--mount` cannot carry.
var ErrMountPath = errors.New("a mounted path cannot hold ',' or '='")

// mountForbidden are the characters a `--mount` source or target cannot hold.
const mountForbidden = ",="

// Limits and fixed paths of every container.
const (
	CPUs   = "2"
	Memory = "2560m"
	// ProductTarget is where the product tree is mounted, read-only.
	ProductTarget = "/opt/musthold"
	// ScratchHome is the HOME of every container: an empty tmpfs.
	ScratchHome = "/home/demo"
	scratchSize = "size=256m,mode=1777"
	tmpTarget   = "/tmp"
	envProgram  = "/usr/bin/env"
	// SystemPath are the image's own directories, after the product's.
	SystemPath = "/usr/local/bin:/usr/bin:/bin"
	// TokenPlaceholder stands for the token in a printed command: the token reaches
	// the session's entry on its standard input, never on a command line.
	TokenPlaceholder = "<stdin>"
)

// Network modes of a container.
const (
	networkNone = "none"
	// ProxyPort is the port the egress proxy listens on.
	ProxyPort = "3128"
	// ProxyHost is the name the session reaches the egress proxy by on the run's
	// internal network, where it is the proxy's alias. A container's own name holds
	// the run's name and can outgrow the 63 bytes of a DNS label, which resolvers
	// refuse; each run has its own network, so one fixed alias serves every run.
	ProxyHost = "egress-proxy"
)

// Subcommands of the runner binary the containers run.
const (
	EntrySubcommand = "entry"
	ProxySubcommand = "egress-proxy"
	ProbeSubcommand = "probe-isolation"
)

// ModelEndpoints are the only destinations the proxy forwards to, unless a box
// names its own.
func ModelEndpoints() []string { return []string{"api.anthropic.com:443"} }

// TLSPort is the port the proxy forwards an allowlisted host's CONNECT to.
const TLSPort = "443"

// AllowAll is the allowlist entry that makes the proxy forward every CONNECT and
// log it: the mode that observes which hosts a session contacts.
const AllowAll = "*"

// HostEndpoints turns allowlisted hosts into the endpoints the proxy forwards to:
// each host on the TLS port.
func HostEndpoints(hosts []string) []string {
	endpoints := make([]string, 0, len(hosts))
	for _, host := range hosts {
		endpoints = append(endpoints, host+":"+TLSPort)
	}
	return endpoints
}

// TrustedRepository is the environment that lets git inside a container work in
// the repository at dir. The host creates the mounted run directory, so the engine
// shows it under an owner other than the container's user, and git refuses a
// repository it does not own unless the command's configuration names it.
func TrustedRepository(dir string) []string {
	return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=" + dir}
}

// Box is what every container of a run shares.
type Box struct {
	// Image is the image, named by digest.
	Image string
	// RunDir is the run directory, mounted read-write at its own path.
	RunDir string
	// Product is the host's product tree, mounted read-only at ProductTarget.
	Product string
	// UID and GID are the user every container runs as.
	UID, GID int
	// Run names the containers and the network of the run.
	Run string
	// Memory bounds the memory, and the memory and swap, of every container;
	// empty, it is Memory. PidsLimit bounds the processes; zero sets no bound.
	Memory    string
	PidsLimit int
	// Egress are the endpoints the proxy forwards to, as host:port, or AllowAll;
	// empty, they are ModelEndpoints. ProxyImage is the proxy's image; empty, it
	// is Image.
	Egress     []string
	ProxyImage string
}

// memory is the box's memory bound.
func (b Box) memory() string {
	if b.Memory == "" {
		return Memory
	}
	return b.Memory
}

// Endpoints are the endpoints the box's proxy forwards to.
func (b Box) Endpoints() []string {
	if len(b.Egress) == 0 {
		return ModelEndpoints()
	}
	return slices.Clone(b.Egress)
}

func (b Box) proxyImage() string {
	if b.ProxyImage == "" {
		return b.Image
	}
	return b.ProxyImage
}

// Validate refuses a box whose mounted paths `--mount` cannot carry.
func (b Box) Validate() error {
	for _, path := range []string{b.RunDir, b.Product} {
		if strings.ContainsAny(path, mountForbidden) {
			return fmt.Errorf("%w: %q", ErrMountPath, path)
		}
	}
	return nil
}

// Names of a run's containers and network.
func (b Box) SessionName() string { return "spdl-session-" + b.Run }
func (b Box) ProxyName() string   { return "spdl-proxy-" + b.Run }
func (b Box) NetworkName() string { return "spdl-net-" + b.Run }

// StepName names the container of one step.
func (b Box) StepName(step int) string {
	return fmt.Sprintf("spdl-step-%s-%d", b.Run, step)
}

// Runner is the runner binary as every container sees it.
func Runner() string { return ProductTarget + "/bin/musthold-demo-loop" }

// base are the options every container has.
func (b Box) base(name string) []string {
	options := []string{
		"run", "--name", name, "--init",
		"--user", fmt.Sprintf("%d:%d", b.UID, b.GID),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", ScratchHome + ":" + scratchSize,
		"--tmpfs", tmpTarget + ":" + scratchSize,
		"--cpus", CPUs, "--memory", b.memory(), "--memory-swap", b.memory(),
	}
	if b.PidsLimit > 0 {
		options = append(options, "--pids-limit", strconv.Itoa(b.PidsLimit))
	}
	return append(options,
		"--log-driver", "none",
		"--mount", "type=bind,source="+b.RunDir+",target="+b.RunDir,
		"--mount", "type=bind,source="+b.Product+",target="+ProductTarget+",readonly",
	)
}

// environment empties the environment and sets exactly the listed variables.
func environment(variables []string) []string {
	return append([]string{envProgram, "-i"}, variables...)
}

// StepArgv is a preparation or collection step: a command run in the project
// with no network at all, removed once it ends.
func (b Box) StepArgv(step int, dir string, variables, argv []string) []string {
	command := append(b.base(b.StepName(step)), "--rm", "--network", networkNone, "--workdir", dir, b.Image)
	command = append(command, environment(variables)...)
	return append(command, argv...)
}

// NetworkArgv creates the run's internal network: one with no route out.
func (b Box) NetworkArgv() []string {
	return []string{"network", "create", "--internal", b.NetworkName()}
}

// ProxyArgv starts the egress proxy, detached, on the engine's default network;
// ConnectArgv then joins it to the run's internal network.
func (b Box) ProxyArgv(logPath string) []string {
	command := append(b.base(b.ProxyName()), "--detach", b.proxyImage())
	command = append(command, environment([]string{"PATH=" + SystemPath})...)
	command = append(command, Runner(), ProxySubcommand, "--listen", ":"+ProxyPort, "--log", logPath)
	for _, endpoint := range b.Endpoints() {
		command = append(command, "--allow", endpoint)
	}
	return command
}

// ConnectArgv joins the proxy to the run's internal network under its alias.
func (b Box) ConnectArgv() []string {
	return []string{"network", "connect", "--alias", ProxyHost, b.NetworkName(), b.ProxyName()}
}

// ProxyURL is the proxy as the session addresses it.
func ProxyURL() string { return "http://" + ProxyHost + ":" + ProxyPort }

// NoProxy are the hosts a client of the session reaches without the proxy: the
// container's own loopback.
const NoProxy = "localhost,127.0.0.1"

// ProxyVariables send every HTTP client of the session through the proxy, but
// for the container's own loopback.
func ProxyVariables() []string {
	url := ProxyURL()
	return []string{"HTTPS_PROXY=" + url, "HTTP_PROXY=" + url, "https_proxy=" + url, "http_proxy=" + url, "NO_PROXY=" + NoProxy, "no_proxy=" + NoProxy}
}

// SessionArgv is the session: attached to standard input, which carries the
// token to the entry, on the internal network, with the entry starting the
// session's command once it has read the token.
func (b Box) SessionArgv(dir string, variables, argv []string) []string {
	command := append(b.base(b.SessionName()), "--interactive", "--network", b.NetworkName(), "--workdir", dir, b.Image)
	command = append(command, environment(variables)...)
	command = append(command, Runner(), EntrySubcommand, "--")
	return append(command, argv...)
}

// RemoveArgv removes the run's containers, whatever their state, and NetworkRemoveArgv its network.
func (b Box) RemoveArgv() []string {
	return []string{"rm", "--force", b.SessionName(), b.ProxyName()}
}
func (b Box) NetworkRemoveArgv() []string { return []string{"network", "rm", b.NetworkName()} }

// InspectArgv reads the configuration the engine keeps for a container.
func InspectArgv(name string) []string { return []string{"inspect", name} }

// stateFormat selects, of what the engine keeps for a container, its exit code
// and whether it was killed for memory, and nothing else: the rest of the
// container's configuration holds its environment.
const stateFormat = "{{.State.ExitCode}} {{.State.OOMKilled}}"

// StateArgv reads the exit code and the memory kill of a container.
func StateArgv(name string) []string { return []string{"inspect", "--format", stateFormat, name} }

// State is how a container ended, as StateArgv reads it.
type State struct {
	ExitCode  int
	OOMKilled bool
}

// ErrState is returned for an answer of StateArgv that is not an exit code and a
// boolean.
var ErrState = errors.New("the container's state cannot be read")

// stateFields is how many fields StateArgv prints.
const stateFields = 2

// ParseState reads the answer of StateArgv.
func ParseState(output string) (State, error) {
	fields := strings.Fields(output)
	if len(fields) != stateFields {
		return State{}, fmt.Errorf("%w: %q", ErrState, output)
	}
	code, err := strconv.Atoi(fields[0])
	if err != nil {
		return State{}, fmt.Errorf("%w: exit code %q", ErrState, fields[0])
	}
	killed, err := strconv.ParseBool(fields[1])
	if err != nil {
		return State{}, fmt.Errorf("%w: OOMKilled %q", ErrState, fields[1])
	}
	return State{ExitCode: code, OOMKilled: killed}, nil
}

// Line renders a docker command as one shell line, the stdin placeholder said.
func Line(docker string, argv []string, stdin bool) string {
	words := make([]string, 0, len(argv)+1)
	for _, word := range append([]string{docker}, argv...) {
		words = append(words, quote(word))
	}
	line := strings.Join(words, " ")
	if stdin {
		line += "   (standard input: CLAUDE_CODE_OAUTH_TOKEN=" + TokenPlaceholder + ")"
	}
	return line
}

func quote(word string) string {
	if word != "" && !strings.ContainsFunc(word, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:,@%+", r))
	}) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// Allowed reports whether an endpoint is one the proxy forwards to.
func Allowed(endpoint string) bool { return slices.Contains(ModelEndpoints(), endpoint) }

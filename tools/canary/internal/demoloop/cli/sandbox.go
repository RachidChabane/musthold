package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/egress"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/entry"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/probe"
)

// listFlag gathers the values of a repeated flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// entry is the first process of a session's container: it reads the token on
// standard input and starts the command after `--` with it.
func (e Env) entry(args []string) int {
	set := e.flags(container.EntrySubcommand)
	if err := set.Parse(args); err != nil {
		return ExitUsage
	}
	return entry.Run(set.Args(), e.Stdin, e.Stdout, e.Stderr)
}

// egressProxy serves the logging proxy of a session's network.
func (e Env) egressProxy(args []string) int {
	set := e.flags(container.ProxySubcommand)
	listen := set.String("listen", ":"+container.ProxyPort, "the address to listen on")
	logPath := set.String("log", "", "the log to append one JSON line per request to")
	var allow listFlag
	set.Var(&allow, "allow", "a host:port to forward to; repeat it")
	if err := set.Parse(args); err != nil || *logPath == "" {
		return ExitUsage
	}
	proxy, closer, err := egress.New(allow, *logPath)
	if err != nil {
		return e.fail(err)
	}
	defer closer.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return e.fail(err)
	}
	if err := proxy.Serve(listener); err != nil {
		return e.fail(err)
	}
	return ExitOK
}

// probeIsolation prints, as JSON, what a container can reach.
func (e Env) probeIsolation(args []string) int {
	set := e.flags(container.ProbeSubcommand)
	var direct, proxied listFlag
	set.Var(&direct, "direct", "a host:port to dial without the proxy; repeat it")
	set.Var(&proxied, "via-proxy", "a host:port to ask the proxy for; repeat it")
	if err := set.Parse(args); err != nil {
		return ExitUsage
	}
	encoded, err := json.Marshal(probe.Run(direct, proxied))
	if err != nil {
		return e.fail(err)
	}
	fmt.Fprintln(e.Stdout, string(encoded))
	return ExitOK
}

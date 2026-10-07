// Command fakehost plays a fake-host persona against a module's command line: it
// lists the pending tickets with `request list --json` and answers them with
// `request submit`, the way a host harness, a script or a relay would. It is test
// support and is never linked into a shipped binary.
package main

import (
	"os"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, execRunner, time.Sleep))
}

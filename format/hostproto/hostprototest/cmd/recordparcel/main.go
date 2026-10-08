// Command recordparcel records the answers an honest host gives to the parcel
// project: it builds the project at a base commit with one uncommitted edit, drives
// the command line of each proof module through its waves of tickets, and writes
// every answer to <request id>.json in the output directory. The recordings feed the
// fake host (`fakehost -recordings`) and the benchmark's clean-room gate tests. It is
// test support and is never linked into a shipped binary.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

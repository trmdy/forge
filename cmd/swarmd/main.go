// Package main is the entry point for the swarmd daemon.
// swarmd runs on each node to provide real-time agent orchestration,
// screen capture, and event streaming to the control plane.
package main

import (
	"flag"
	"fmt"
	"os"
)

// Version information (set by goreleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// TODO: Implement swarmd daemon
	hostname := flag.String("hostname", "127.0.0.1", "hostname to listen on")
	port := flag.Int("port", 0, "port to listen on")
	flag.Parse()

	fmt.Printf("swarmd %s (commit: %s, built: %s)\n", version, commit, date)
	fmt.Printf("Binding: %s:%d\n", *hostname, *port)
	fmt.Println("Daemon mode not yet implemented. This is a post-MVP feature.")
	os.Exit(0)
}

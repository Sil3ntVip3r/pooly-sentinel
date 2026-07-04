package main

import (
	"fmt"
	"os"
)

const version = "v0.0.0-task1-skeleton"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	switch args[0] {
	case "version":
		fmt.Println(version)
	case "run", "check", "status", "doctor", "report", "notify", "collectors", "rules":
		fmt.Fprintf(os.Stderr, "pooly-agent %s is a Task 1 placeholder; monitoring is not implemented yet.\n", args[0])
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "unknown pooly-agent command %q\n\n", args[0])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`pooly-agent

Pooly Sentinel agent skeleton.

Usage:
  pooly-agent help
  pooly-agent version

Planned commands:
  run
  check config
  status
  doctor
  report daily
  notify test discord
  collectors list
  collectors run resources
  rules test

Task 1 status:
  Repository structure and documentation only. Production monitoring is not implemented.
`)
}

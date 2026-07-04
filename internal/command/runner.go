package command

// Package command will contain the safe command runner.
//
// Future implementation rules:
//   - use exec.CommandContext with explicit args only
//   - use configured absolute paths for sensitive commands
//   - apply timeouts and output limits
//   - redact output before logging

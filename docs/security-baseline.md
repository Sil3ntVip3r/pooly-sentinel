# Security Baseline

Pooly Sentinel is security-first. This document captures baseline expectations for later collectors and rule work.

## Global Redaction Rules

Never log, store in metrics, send externally, or expose:

- secret values
- webhook URLs
- raw tokens
- SSH private key material
- full public authorized key material
- raw audit records in notifications
- raw journal dumps in notifications
- shell command strings in alert text

## SSH Alpha Rules

The SSH collector should verify configuration and evidence, but alpha builds must not automatically reload or restart SSH.

Critical future checks include:

- expected SSH port is listening
- forbidden SSH ports are not publicly listening
- root login is disabled
- password authentication is disabled
- empty passwords are disabled
- strict modes are enabled
- root `authorized_keys` is empty

## Audit Alpha Rules

Audit support is optional and observe-only during alpha. The agent should not manage or load audit rules in the initial implementation.

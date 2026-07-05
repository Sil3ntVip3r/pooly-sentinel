# SSH Listening-Port Collector

Task 5 collects listening TCP sockets with:

```text
ss -H -l -n -t
```

The collector parses IPv4, IPv6, and wildcard listeners and reports whether configured expected ports and forbidden ports are currently listening.

Metrics:

- `pooly_ssh_expected_port_listening`
- `pooly_ssh_forbidden_port_listening`

The `port` label is bounded to configured numeric ports. The collector does not infer policy severity from these facts, does not reload SSH, and does not edit configuration.

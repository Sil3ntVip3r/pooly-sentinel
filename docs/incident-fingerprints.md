# Incident Fingerprints

Incident fingerprints use:

```text
<node_id>:<type>:<target>:<condition>
```

Fingerprint components are normalized, bounded, and validated. Empty components are rejected. Components must not contain secrets, raw journal messages, usernames, source addresses, MAC addresses, boot UUIDs, private-key material, or arbitrary error text.

The stored fingerprint remains human-readable. The incident ID is derived deterministically from the fingerprint with a compact hash prefix.

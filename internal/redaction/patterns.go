package redaction

import "regexp"

const Replacement = "[REDACTED]"

var (
	discordWebhookPattern         = regexp.MustCompile(`https://(?:(?:canary|ptb)\.)?discord(?:app)?\.com/api/webhooks/[0-9]+/[A-Za-z0-9._~+/=-]+`)
	genericWebhookTokenURLPattern = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]*(?:^|/)webhooks?/[^\s"'<>/]{8,}/[A-Za-z0-9._~+/=-]{16,}[^\s"'<>]*`)
	privateKeyPattern             = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	authorizedKeyPattern          = regexp.MustCompile(`(?m)\b(?:ssh-rsa|ssh-ed25519|ecdsa-sha2-nistp[0-9]+|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com)\s+[A-Za-z0-9+/=]{20,}(?:\s+\S+)?`)

	authorizationPattern = regexp.MustCompile(`(?i)\b(authorization\s*[:=]\s*)(?:bearer|basic|token)?\s*[A-Za-z0-9._~+/=-]+`)
	querySecretPattern   = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|token|api[_-]?key|apikey|key|password|passwd|pwd|secret|signature|sig|auth|authorization|code)=)[^&\s"'<>]+`)
	keyValuePattern      = regexp.MustCompile(`(?i)\b([A-Z0-9_.-]*(?:TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|APIKEY|AUTHORIZATION|WEBHOOK|PRIVATE[_-]?KEY)[A-Z0-9_.-]*|password|passwd|pwd|token|api[_-]?key|apikey|secret|client_secret|authorization|webhook)\b(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
)

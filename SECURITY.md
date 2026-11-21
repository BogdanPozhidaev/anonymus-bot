# Security

Report suspected vulnerabilities through a private channel to the
deployment owner or repository maintainer. Describe the affected version,
the impact and the minimum evidence needed to reproduce it using synthetic
data. Omit accounts, invite URLs, bot tokens and captured conversations.

The operator API requires a session issued after password and TOTP
verification. Bot service calls use separate file-mounted tokens.
For deployment and credential handling see the
[deployment guide](docs/DEPLOYMENT.md) and
[preflight checks](docs/SECURITY_CHECKLIST.md). Known technical findings
are tracked in the [roadmap](DEVELOPMENT_ROADMAP.md).

# Project handover

Use the root [README](../README.md) as the entry point. Follow the
[deployment guide](DEPLOYMENT.md) for a fresh demo. Use a separate Compose
project, synthetic fixtures and generated credentials. Confirm migration
status, operator sign-in, invitation redemption with a fixture bot,
and representative media preview. Record the commit and the environment
used for every smoke test.

Transfer ownership of the source repository, package registry and image
access, deployment host, domain and TLS renewal, bot account, and backup
storage. Transfer secret locations and recovery responsibilities through
an approved private channel. Validate source, dependency and contributor
rights before making commercial claims about the project.

Use the [roadmap](../DEVELOPMENT_ROADMAP.md) and
[development log](DEVELOPMENT_LOG.md) for release status. Local test
results and a GitHub Actions run on a PR are separate pieces of release
evidence.

## Deferred independent acceptance

The project owner deferred this check because an independent reviewer
was unavailable. The isolated deployment and sign-in passed locally.
When a reviewer is available, give them the reviewed PR
commit and this guide. Have them follow the [isolated HTTP deployment
instructions](DEPLOYMENT.md#start-an-isolated-http-stand) from a clean
checkout using only fixture credentials. Record any instruction they
had to get from another person. Check that the PR's backend, bot,
frontend, image and migration jobs all passed for that same commit.

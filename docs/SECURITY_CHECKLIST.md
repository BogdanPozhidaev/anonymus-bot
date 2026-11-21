# Deployment security checklist

- The production Compose configuration publishes the Nginx edge only.
  The dev override binds backend, PostgreSQL and Redis to loopback.
- The edge blocks `/api/internal` and its subpaths. Check the public
  health endpoint and confirm that direct service calls require the
  correct token.
- Mount different service tokens from files outside the repository.
  Keep Telegram, database, Redis and encryption credentials in separately
  controlled deployment configuration.
- Install owner-managed TLS certificates and verify their hostname,
  validity and renewal procedure. Disable the Swagger endpoint unless
  an operator specifically needs it.
- Verify operator ownership, role isolation and the TOTP sign-in path
  on a fixture installation. Confirm offboarding revokes application
  access under the procedure for the current release.
- Validate backup and restore in an isolated project. Review retention
  and backup expiry with whoever controls the deployment.

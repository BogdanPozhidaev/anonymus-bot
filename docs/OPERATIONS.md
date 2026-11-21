# Operations

Use the correct Compose project name for every operation. The
[deployment guide](DEPLOYMENT.md) covers initialization, migration
status, authenticated health checks and external TLS.

- Monitor `/api/health` through Nginx. HTTP 503 indicates a failed
  PostgreSQL or Redis readiness check. Review the relevant service
  status and logs with access restricted to operators.
- Check the panel history for queued, retrying or failed deliveries.
  A failed Telegram response can be ambiguous. Replaying messages
  manually can generate duplicates.
- Rotate the two internal API tokens using separate files outside the
  repository. Coordinate bot and backend restarts. Check application
  health and authorization after both services reload the files.
- Follow [retention](RETENTION.md) for dry-run and execution. Keep the
  writers quiesced when following the
  [database and media snapshot procedure](../deploy/backup/README.md).
- For operational handover, pass configuration locations and access
  ownership securely. Use synthetic accounts for smoke tests.

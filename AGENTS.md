# Anonymus Bot — Agent Instructions

## Project goal
Maintain and incrementally improve the existing Anonymus Bot MVP, with the long-term goal of making it demonstrable, secure, documented, and commercially transferable. Do not rewrite it from scratch.

## Source of truth
1. Inspect the actual repository before making claims or changes.
2. Read `docs/PROJECT_CONTEXT.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY_AUDIT.md`, `docs/KNOWN_ISSUES.md`, and `DEVELOPMENT_ROADMAP.md` before substantial work.
3. Treat `docs/DEVELOPMENT_LOG.md` as a record of verified work only. Treat `docs/DECISIONS.md` as the record of explicit project decisions.
4. The audit documents describe a static review of the archive at Git HEAD `5e63f63`; re-check findings against the current working tree before changing code.
5. Clearly label facts, hypotheses, and unverified assumptions. Never claim a test passed unless it was run and its result observed.

## Working rules
- Preserve existing behavior unless the task explicitly changes it. Prefer small, reviewable patches.
- Before editing, identify relevant files, current behavior, risks, and acceptance criteria.
- Do not silently broaden scope, add dependencies, change public APIs, or alter database schema.
- For security/privacy work, consider authorization, data exposure, logging, retention, race conditions, and failure behavior.
- Never include real tokens, passwords, Telegram IDs, session files, private user data, or production secrets in code, logs, tests, commits, or documentation. Use placeholders and synthetic fixtures.
- Do not weaken authentication or validation to make a test pass.
- Preserve user-facing Russian/English strings unless localization changes are requested. Write new code comments and technical documentation in English unless asked otherwise.
- Do not assert absolute anonymity, GDPR compliance, or production readiness without a separate, documented basis.
- Do not run destructive commands, production migrations, external Telegram actions, or deploys without explicit approval.

## Required workflow
1. Inspect status/diff and relevant code before editing.
2. State a concise plan and acceptance criteria.
3. Make the smallest coherent change.
4. Add/update tests for changed behavior where practical.
5. Run the narrowest relevant checks, then broader checks if environment permits.
6. Review the final diff for unrelated changes, secrets, generated files, and line-ending noise.
7. Report files changed, behavior changed, commands actually run, results, and remaining risks.

## Test/environment constraints known from the audit
The reviewed archive required Go 1.26.0 for backend and Go 1.25.1 for bot. The audit environment had Go 1.23.2 and Docker unavailable; `npm ci` timed out. These are historical audit observations, not assumptions about the current machine. Check installed versions before running commands.

## Definition of done
A task is done only when its acceptance criteria are met, relevant tests/checks are reported accurately, documentation is updated where needed, and the final diff is reviewed. If blocked, state the blocker and leave the work in a safe, understandable state.

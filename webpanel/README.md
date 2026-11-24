# Operator web panel

The panel provides operator sign-in, session and incoming-request workflows,
and access to message history and audit records.

Use this directory for the commands below. The [local development guide](../docs/README.md#local-development)
covers the backend and fixture administrator. For the built panel served
through Nginx, see [deployment](../docs/DEPLOYMENT.md).

```sh
npm ci
npm run dev
```

Run `npm run lint`, `npm run typecheck` and `npm run build` for
local checks. For browser tests, install Chromium with
`npx playwright install chromium` and run `npm run test:e2e`. The
browser suite uses a synthetic API.

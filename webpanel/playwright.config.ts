import { defineConfig } from '@playwright/test';

const baseURL = 'http://127.0.0.1:4175';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  workers: process.env.CI ? 2 : undefined,
  reporter: 'line',
  use: {
    baseURL,
    browserName: 'chromium',
    serviceWorkers: 'block',
  },
  webServer: {
    command: 'npm run preview -- --host 127.0.0.1 --port 4175 --strictPort',
    url: baseURL,
    reuseExistingServer: false,
  },
});

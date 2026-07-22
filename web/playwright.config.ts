import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "../test/e2e",
  outputDir: "../.flowlens-dev/playwright-results",
  reporter: [["list"]],
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:4174",
    trace: "retain-on-failure",
    screenshot: "off",
    ...devices["Desktop Chrome"],
  },
  webServer: [
    {
      command: "pnpm build:demo && pnpm preview:demo",
      port: 4174,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "pnpm build:app && pnpm preview:app",
      port: 4175,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});

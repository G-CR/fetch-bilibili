import { defineConfig } from "@playwright/test";

const isLive = process.env.E2E_MODE === "live";
const chromePath =
  process.env.PLAYWRIGHT_CHROME_PATH || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const baseURL = process.env.E2E_BASE_URL || (isLive ? "http://127.0.0.1:5173" : "http://127.0.0.1:43173");
const apiBase = process.env.E2E_API_BASE || (isLive ? "http://127.0.0.1:8080" : "http://127.0.0.1:43180");

const webServer = isLive
  ? undefined
  : {
      command: "node ./scripts/e2e/start-stack.mjs",
      url: baseURL,
      reuseExistingServer: false,
      timeout: 30_000
    };

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  retries: 0,
  timeout: 30_000,
  use: {
    baseURL,
    headless: true,
    launchOptions: {
      executablePath: chromePath
    }
  },
  webServer: process.env.E2E_MANAGED ? undefined : webServer,
  metadata: {
    e2eMode: isLive ? "live" : "mock",
    apiBase
  }
});

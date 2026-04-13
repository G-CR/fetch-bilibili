import { spawn } from "node:child_process";

const isLive = process.env.E2E_MODE === "live";
const baseURL = process.env.E2E_BASE_URL || (isLive ? "http://127.0.0.1:5173" : "http://127.0.0.1:43173");
const apiBase = process.env.E2E_API_BASE || (isLive ? "http://127.0.0.1:8080" : "http://127.0.0.1:43180");
const playwrightCommand = process.platform === "win32" ? "npx.cmd" : "npx";

let stackProcess = null;

async function waitFor(url, label) {
  const deadline = Date.now() + 30_000;

  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        console.log(`${label} ready: ${url}`);
        return;
      }
    } catch (_error) {
      // ignore
    }

    await new Promise((resolve) => setTimeout(resolve, 300));
  }

  throw new Error(`${label} did not become ready: ${url}`);
}

function shutdown() {
  if (stackProcess && !stackProcess.killed) {
    stackProcess.kill("SIGTERM");
  }
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
process.on("exit", shutdown);

if (!isLive) {
  stackProcess = spawn("node", ["./scripts/e2e/start-stack.mjs"], {
    stdio: "inherit",
    env: {
      ...process.env
    }
  });

  stackProcess.on("exit", (code) => {
    if (code && code !== 0) {
      process.exit(code);
    }
  });

  await waitFor(`${apiBase}/healthz`, "mock-api");
  await waitFor(baseURL, "frontend");
}

const testProcess = spawn(
  playwrightCommand,
  ["playwright", "test", "--config", "playwright.config.js"],
  {
    stdio: "inherit",
    env: {
      ...process.env,
      E2E_MANAGED: "1",
      E2E_BASE_URL: baseURL,
      E2E_API_BASE: apiBase
    }
  }
);

const exitCode = await new Promise((resolve) => {
  testProcess.on("exit", (code) => resolve(code || 0));
});

shutdown();
process.exit(exitCode);

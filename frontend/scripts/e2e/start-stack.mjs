import { spawn } from "node:child_process";

const children = [];

function spawnChild(command, args, name, env = {}) {
  const child = spawn(command, args, {
    stdio: "inherit",
    env: {
      ...process.env,
      ...env
    }
  });

  child.on("exit", (code) => {
    if (code !== null && code !== 0) {
      console.error(`${name} exited with code ${code}`);
      process.exit(code || 1);
    }
  });

  children.push(child);
  return child;
}

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
  for (const child of children) {
    child.kill("SIGTERM");
  }
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
process.on("exit", shutdown);

spawnChild("node", ["./scripts/e2e/mock-api.mjs"], "mock-api", {
  E2E_API_PORT: "43180"
});
spawnChild("npm", ["run", "dev", "--", "--host", "127.0.0.1", "--port", "43173"], "frontend-dev");

await waitFor("http://127.0.0.1:43180/healthz", "mock-api");
await waitFor("http://127.0.0.1:43173", "frontend");

setInterval(() => {}, 1 << 30);

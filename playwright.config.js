import { defineConfig } from "@playwright/test";
import path from "node:path";

const port = Number(process.env.E2E_PORT || 18090);
const dataDir = path.resolve(process.env.E2E_DATA_DIR || ".e2e-data");

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    browserName: "chromium",
    viewport: { width: 1280, height: 900 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "./scripts/e2e-server.sh",
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      ...process.env,
      E2E_PORT: String(port),
      E2E_DATA_DIR: dataDir,
    },
  },
});

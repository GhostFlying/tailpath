import { defineConfig, devices } from "@playwright/test";

const isScaleGate = process.env.TAILPATH_SCALE_E2E === "1";

export default defineConfig({
  testDir: "./e2e",
  outputDir: process.env.TAILPATH_PLAYWRIGHT_OUTPUT_DIR ?? "test-results",
  fullyParallel: true,
  retries: isScaleGate ? 0 : process.env.CI ? 2 : 0,
  workers: isScaleGate ? 1 : process.env.CI ? 4 : undefined,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.TAILPATH_E2E_BASE_URL ?? "http://127.0.0.1:18082",
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : undefined,
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "desktop-chromium",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: "mobile-chromium",
      use: { ...devices["Pixel 7"] },
    },
  ],
});

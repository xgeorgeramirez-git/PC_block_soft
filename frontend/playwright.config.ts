import { defineConfig } from "@playwright/test";

// Снимки экранов интерфейса в браузере с подменой программы (window.go).
// Настоящее окно Windows снимается отдельно в CI на windows-latest.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: "http://127.0.0.1:4173",
    viewport: { width: 960, height: 640 },
    locale: "ru-RU",
    colorScheme: "light",
  },
  webServer: {
    command: "npm run preview -- --host 127.0.0.1",
    url: "http://127.0.0.1:4173",
    reuseExistingServer: false,
    timeout: 60_000,
  },
});

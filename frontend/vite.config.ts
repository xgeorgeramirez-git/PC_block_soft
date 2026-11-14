/// <reference types="vitest/config" />
import { fileURLToPath } from "node:url";
import { defineConfig, searchForWorkspaceRoot } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Стикеры промокода берутся из assets/stickers/ вне папки фронта (RM5b-8): окну разработки
  // (vite dev, wails dev) их можно отдавать; сборке это не нужно.
  server: {
    fs: {
      allow: [searchForWorkspaceRoot(process.cwd()), fileURLToPath(new URL("../assets/stickers", import.meta.url))],
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  preview: {
    port: 4173,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/test-setup.ts"],
    // Покрытие: `npx vitest run --coverage`. Порог 70 % на логике интерфейса src/lib (ТЗ 10.4);
    // экраны проверяются своими тестами, но в порог не входят.
    coverage: {
      provider: "v8",
      include: ["src/lib/**/*.ts"],
      exclude: ["src/lib/**/*.test.ts"],
      reporter: ["text-summary", "text"],
      reportsDirectory: "coverage",
      thresholds: { lines: 70, functions: 70, branches: 70, statements: 70 },
    },
  },
});

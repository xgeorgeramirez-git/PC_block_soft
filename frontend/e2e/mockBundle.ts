import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "vite";

// Подменная программа (src/__mocks__/backend.ts) одним скриптом для страницы в браузере:
// Playwright кладёт его в addInitScript, и окно получает window.go до запуска интерфейса.
// Тот же сборщик проверяется в vitest (src/__mocks__/bundle.test.ts), потому что браузеры
// в облачной песочнице не запускаются.

const ENTRY = resolve(dirname(fileURLToPath(import.meta.url)), "../src/__mocks__/backend.ts");

interface BuiltChunk {
  type: string;
  code?: string;
}

let cached: Promise<string> | undefined;

export function mockBundle(): Promise<string> {
  cached ??= build({
    configFile: false,
    logLevel: "silent",
    build: {
      write: false,
      minify: false,
      lib: { entry: ENTRY, name: "PCBlockMock", formats: ["iife"], fileName: "mock" },
    },
  }).then((result) => {
    const results = (Array.isArray(result) ? result : [result]) as { output?: BuiltChunk[] }[];
    const chunk = results.flatMap((r) => r.output ?? []).find((o) => o.type === "chunk" && o.code);
    if (!chunk?.code) {
      throw new Error("не удалось собрать подменную программу для браузера");
    }
    return chunk.code;
  });
  return cached;
}

/** Код для addInitScript: подменная программа и её установка с настройками. */
export async function mockScript(options: unknown): Promise<string> {
  const code = await mockBundle();
  return `${code}\n;PCBlockMock.installBackend(${JSON.stringify(options ?? {})});`;
}

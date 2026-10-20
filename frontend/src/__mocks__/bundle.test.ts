import { afterEach, describe, expect, it } from "vitest";
import { mockScript } from "../../e2e/mockBundle";

// Браузерные сценарии (Playwright) получают подменную программу как один скрипт.
// Здесь проверяется, что скрипт собирается и после установки отвечает, как в тестах.

afterEach(() => {
  delete window.go;
});

describe("подменная программа одним скриптом", () => {
  it("после установки window.go отвечает на команды", async () => {
    const script = await mockScript({ version: "7.7.7", mode: "parent", sessionActive: true });
    new Function(script)();
    const app = window.go!.ui!.App!;
    expect(await app.Version()).toBe("7.7.7");
    expect(await app.Mode()).toBe("parent");
    const hello = JSON.parse(await app.Call("Hello", "{}"));
    expect(hello).toMatchObject({ ok: true, payload: { version: "7.7.7", configured: true } });
    const today = JSON.parse(await app.Call("GetToday", "{}"));
    expect(today.payload.children[0].name).toBe("Миша");
  }, 60_000);

  it("настройки доходят до подмены", async () => {
    const script = await mockScript({ configured: false, children: [] });
    new Function(script)();
    const hello = JSON.parse(await window.go!.ui!.App!.Call("Hello", "{}"));
    expect(hello.payload.configured).toBe(false);
  }, 60_000);
});

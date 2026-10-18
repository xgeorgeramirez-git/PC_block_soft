#!/usr/bin/env python3
"""Скан текстов интерфейса (ТЗ NFR-11, урок L-117).

Ищет длинное и среднее тире в пользовательских строках:
  - Go: строковые литералы в .go (кроме тестов), без комментариев;
  - фронт: .ts/.tsx в frontend/src (кроме тестов), без комментариев.
Код выхода 1, если найдено. Запуск: python3 tools/scan_ui_texts.py
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
DASHES = ("—", "–")

GO_STRING = re.compile(r'"(?:[^"\\\n]|\\.)*"|`[^`]*`')


def strip_go_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", lambda m: "\n" * m.group(0).count("\n"), text, flags=re.S)
    return "\n".join(line.split("//", 1)[0] if '"' not in line.split("//", 1)[0] or line.count('"') % 2 == 0 else line
                     for line in text.splitlines())


def strip_ts_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", lambda m: "\n" * m.group(0).count("\n"), text, flags=re.S)
    out = []
    for line in text.splitlines():
        stripped = line.lstrip()
        if stripped.startswith("//"):
            out.append("")
        else:
            out.append(re.sub(r"\s//\s.*$", "", line))
    return "\n".join(out)


def scan() -> list[str]:
    problems: list[str] = []
    for path in ROOT.rglob("*.go"):
        rel = path.relative_to(ROOT)
        if path.name.endswith("_test.go") or "node_modules" in rel.parts:
            continue
        text = strip_go_comments(path.read_text(encoding="utf-8"))
        for no, line in enumerate(text.splitlines(), 1):
            for lit in GO_STRING.findall(line):
                if any(d in lit for d in DASHES):
                    problems.append(f"{rel}:{no}: тире в строке {lit[:60]}")
    src = ROOT / "frontend" / "src"
    for path in list(src.rglob("*.ts")) + list(src.rglob("*.tsx")):
        if ".test." in path.name:
            continue
        rel = path.relative_to(ROOT)
        text = strip_ts_comments(path.read_text(encoding="utf-8"))
        for no, line in enumerate(text.splitlines(), 1):
            if any(d in line for d in DASHES):
                problems.append(f"{rel}:{no}: тире в тексте: {line.strip()[:80]}")
    return problems


def main() -> int:
    # На Windows консоль по умолчанию не UTF-8: без этого печать кириллицы падает.
    sys.stdout.reconfigure(encoding="utf-8")
    problems = scan()
    for p in problems:
        print(p)
    if problems:
        print(f"Найдено тире в текстах интерфейса: {len(problems)}. Замените на дефис, запятую или двоеточие.")
        return 1
    print("Тексты интерфейса: длинных тире нет.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

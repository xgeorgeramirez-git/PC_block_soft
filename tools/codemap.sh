#!/usr/bin/env bash
# Генератор кодовой карты docs/CODEMAP.md (методика dev-methodology, шаблон §13).
# Карту руками не править: только этот скрипт. Работает без сети и без сборки.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=docs/CODEMAP.md

pkg_role() {
  # Роль пакета — первая строка комментария «// Package x ...» из любого файла пакета.
  local dir=$1 line
  if [ "$dir" = "." ]; then echo "main: точка входа, запуск окна"; return; fi
  line=$(grep -h -m1 -E '^// Package [a-z0-9_]+ ' "$dir"/*.go 2>/dev/null | head -1 || true)
  [ -n "$line" ] && echo "${line#// Package }" || echo "(нет комментария Package)"
}

{
  echo "# CODEMAP — карта кода PC_block_soft (сгенерировано tools/codemap.sh)"
  echo
  echo "> НЕ редактировать руками — перегенерируется скриптом. Карта сужает поиск;"
  echo "> сигнатуры перед кодом сверять точечным grep'ом (урок L-52)."
  echo
  echo "## Пакеты Go (роль одной строкой)"
  echo
  echo "| Пакет | Файлов (тесты) | Роль |"
  echo "|---|---|---|"
  find . -name '*.go' -not -path './frontend/*' -not -path './build/*' -printf '%h\n' | sort -u | while read -r d; do
    files=$(find "$d" -maxdepth 1 -name '*.go' -not -name '*_test.go' | wc -l)
    tests=$(find "$d" -maxdepth 1 -name '*_test.go' | wc -l)
    echo "| \`${d#./}\` | $files ($tests) | $(pkg_role "$d") |"
  done
  echo
  echo "## Миграции базы"
  echo
  if ls internal/storage/migrations/*.sql >/dev/null 2>&1; then
    for f in internal/storage/migrations/*.sql; do
      echo "- \`$(basename "$f")\` — $(head -1 "$f" | sed 's/^-- *//')"
    done
  else
    echo "Пока нет (появятся в M1: internal/storage/migrations/NNN_имя.sql, первая строка — комментарий о назначении)."
  fi
  echo
  echo "## Интерфейс (frontend/src)"
  echo
  find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \) -not -name '*.test.*' | sort | while read -r f; do
    echo "- \`${f#frontend/}\`"
  done
  echo
  echo "## Биндинги Wails (что интерфейс может вызвать у программы)"
  echo
  find frontend/wailsjs/go -name '*.d.ts' | sort | while read -r f; do
    echo "- \`${f#frontend/}\`: $(grep -oE 'export function [A-Za-z0-9_]+' "$f" | sed 's/export function //' | paste -sd, -)"
  done
  echo
  echo "## Конвенции стека"
  echo
  echo "- Ядро правил и учёта — только стандартная библиотека Go, без Wails и Windows (ТЗ 3.3); тесты ядра идут локально и на ubuntu."
  echo "- Windows-код — в файлах \`*_windows.go\`; проверка сборки: \`GOOS=windows go vet -tags desktop,production ./...\`."
  echo "- Длительности в базе — целые секунды; дата дня — \`YYYY-MM-DD\` по местному времени; моменты — UTC мс + смещение пояса."
  echo "- Тексты интерфейса — русские, без длинных тире (\`python3 tools/scan_ui_texts.py\`)."
  echo "- После смены Go-методов App — биндинги \`frontend/wailsjs/go\` регенерирует \`wails build\` в CI; расхождение с закоммиченными роняет CI."
  echo
  echo "## Точки входа"
  echo
  echo "- \`main.go\` — запуск окна (\`internal/ui.Run\`), встраивает \`frontend/dist\`."
  echo "- \`internal/ui/app.go\` — методы App для интерфейса."
  echo "- \`frontend/src/main.tsx\` → \`App.tsx\`; обращения к программе — только через \`frontend/src/api.ts\`."
  echo
  echo "## Команды"
  echo
  echo "- Локально (облачная песочница, только ядро на стандартной библиотеке): \`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/version/...\`"
  echo "- Фронт: \`cd frontend && npm ci && npm test && npm run build\`; снимки: \`npm run e2e\` (в CI)."
  echo "- CI: \`ci.yml\` (ubuntu, R0–R2 + снимки), \`windows.yml\` (R3: установщик, окно, выпуск), \`full-series.yml\` (R4). go.sum обновляет CI сам в рабочей ветке (сделать git pull)."
  echo "- Карта: \`bash tools/codemap.sh\`."
} > "$OUT"
echo "Записано: $OUT"

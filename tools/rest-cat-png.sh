#!/usr/bin/env bash
# Картинка экрана «Перерыв на отдых» (ТЗ 8.2.3 п.12): SVG проекта → PNG для окна стража.
# PNG вдвое крупнее (320×320), окно уменьшает его до 160 px с учётом масштаба экрана.
# Нужен rsvg-convert (librsvg2-bin). Запуск из корня репозитория: bash tools/rest-cat-png.sh
set -euo pipefail
src=internal/guard/assets/rest-cat.svg
dst=internal/guard/assets/rest-cat.png
rsvg-convert --width 320 --height 320 --keep-aspect-ratio --background-color none "$src" -o "$dst"
echo "Готово: $dst"

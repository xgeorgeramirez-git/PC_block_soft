#!/usr/bin/env bash
# Проверка собранного фронта (frontend/dist) после `vite build`, шаг ci.yml (RM5b-8).
# Стикеры .tgs попадают в сборку только те 17, что нужны промокоду (R26, D-83): остальные
# стикеры заказчика из assets/stickers/ в программу не идут. Имя в dist - <номер>-<хеш>.tgs.
#
# Запуск: bash tools/check-dist.sh [--tgs-exact N] [--player] [папка dist]
#   без ключей         - .tgs не больше 17, все из набора R26;
#   --tgs-exact 17     - ровно 17 (включается, когда окно ребёнка показывает стикеры, T8);
#   --player           - в сборке есть файл проигрывателя lottie_light.min-*.js.
# Код выхода 1 и строки ::error, если что-то не так.
set -uo pipefail

exact=""
player=0
dist=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tgs-exact) exact="$2"; shift 2 ;;
    --player) player=1; shift ;;
    *) dist="$1"; shift ;;
  esac
done
[ -n "$dist" ] || dist="$(dirname "$0")/../frontend/dist"

if [ ! -d "$dist" ]; then
  echo "::error title=Сборка фронта::Нет папки $dist (сначала vite build)"
  exit 1
fi

allowed=" 017 033 044 049 050 077 093 096 112 023 028 048 069 082 084 095 110 "
status=0
count=0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  count=$((count + 1))
  base=$(basename "$f")
  num=${base:0:3}
  if [[ ! "$base" =~ ^[0-9]{3}([-.].*)?\.tgs$ ]] || [[ "$allowed" != *" $num "* ]]; then
    echo "::error title=Стикеры в сборке::$base не из набора промокода R26 (assets/stickers/README.md)"
    status=1
  fi
done < <(find "$dist" -type f -name '*.tgs' | sort)

if [ -n "$exact" ]; then
  if [ "$count" -ne "$exact" ]; then
    echo "::error title=Стикеры в сборке::В $dist $count файлов .tgs, ждали ровно $exact"
    status=1
  fi
elif [ "$count" -gt 17 ]; then
  echo "::error title=Стикеры в сборке::В $dist $count файлов .tgs, больше 17: в сборку попали лишние стикеры"
  status=1
fi

if [ "$player" -eq 1 ] && [ -z "$(find "$dist" -type f -name 'lottie_light.min*.js' | head -1)" ]; then
  echo "::error title=Проигрыватель стикеров::В $dist нет lottie_light.min*.js"
  status=1
fi

[ "$status" -eq 0 ] && echo "Сборка фронта: стикеров .tgs $count${exact:+ (ждали $exact)}$([ "$player" -eq 1 ] && echo ', проигрыватель на месте')."
exit "$status"

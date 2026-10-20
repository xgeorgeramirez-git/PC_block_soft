#!/usr/bin/env bash
# Проверка своих шаблонов установщика NSIS (build/windows/installer/*.nsi, *.nsh), R0 в ci.yml.
# Ловит три беды, которые на Windows видны только по битой кириллице или пропавшей строке:
#   1. файл без BOM (EF BB BF): NSIS читает его как ANSI, и русские тексты ломаются;
#   2. файл не в UTF-8;
#   3. комментарий (# или ;) заканчивается обратной косой чертой: NSIS склеивает его со следующей
#      строкой, и она пропадает.
# Проверяются только файлы, которые лежат в git: wails_tools.nsh Wails пишет сам при каждой сборке
# (он в .gitignore), его BOM нас не касается.
# Запуск: bash tools/check_nsis_bom.sh. Код выхода 1, если что-то найдено.
set -uo pipefail
cd "$(dirname "$0")/.."

status=0
files=$(git ls-files 'build/windows/installer/*.nsi' 'build/windows/installer/*.nsh')
if ! grep -q 'project\.nsi$' <<<"$files"; then
  echo "::error title=NSIS::Нет build/windows/installer/project.nsi: без него Wails соберёт установщик по своему английскому шаблону"
  exit 1
fi

while IFS= read -r f; do
  [ -n "$f" ] || continue
  head3=$(head -c 3 "$f" | od -An -tx1 | tr -d ' \n')
  if [ "$head3" != "efbbbf" ]; then
    echo "::error file=$f,title=NSIS без BOM::$f должен быть в UTF-8 с BOM (первые байты EF BB BF), иначе кириллица в установщике побьётся"
    status=1
  fi
  if ! iconv -f UTF-8 -t UTF-8 "$f" >/dev/null 2>&1; then
    echo "::error file=$f,title=NSIS не UTF-8::$f не читается как UTF-8"
    status=1
  fi
  bad=$(sed '1s/^\xEF\xBB\xBF//' "$f" | grep -nE '^[[:space:]]*[#;].*\\[[:space:]]*$' || true)
  if [ -n "$bad" ]; then
    while IFS= read -r line; do
      echo "::error file=$f,title=NSIS комментарий с обратной чертой::${line%%:*}: комментарий заканчивается на обратную косую черту и съест следующую строку"
    done <<<"$bad"
    status=1
  fi
done <<<"$files"

if [ "$status" -eq 0 ]; then
  echo "Шаблоны NSIS: BOM на месте, комментарии без обратной черты в конце."
fi
exit $status

#!/usr/bin/env bash
# Публикует главные строки ошибок из журнала шага пометками сборки (::error).
# Зачем: журналы и артефакты GitHub Actions из облачной песочницы агента не скачиваются,
# а пометки видны через API: gh api repos/<owner>/<repo>/check-runs/<job-id>/annotations
# Использование: tools/annotate-errors.sh <журнал> [заголовок]
set -uo pipefail
LOG=${1:?журнал}
TITLE=${2:-Ошибка сборки}
[ -s "$LOG" ] || { echo "::error title=$TITLE::Журнал пуст: $LOG"; exit 0; }
TMP=$(mktemp -d)
sed -e 's/\x1b\[[0-9;]*[A-Za-z]//g' -e 's/\r$//' "$LOG" > "$TMP/clean"
PATTERN='(--- FAIL|^FAIL|^ok  |panic:|error TS[0-9]+|[Ee]rror:|ERROR|Traceback|Exception|×|✗|FAIL |МАЛО |[A-Za-z0-9_./\\-]+\.(go|ts|tsx|py):[0-9]+(:[0-9]+)?:|npm ERR!|undefined:|cannot |expected|Expected|Received|exit status)'
# Упавшие тесты Playwright (репортёр list): блок «  1) e2e/…spec.ts:N:M › название» с текстом ошибки
# идёт первым, до 30 строк на тест: иначе строки установки браузера и списка тестов съедают лимит
# пометок раньше, чем доходят до причины (ревью M4, F2).
awk '
  /^  [0-9]+\) .*›/ { inblk = 1; left = 30 }
  /^  [0-9]+ (failed|flaky|passed|skipped|did not run)/ { inblk = 0 }
  inblk && left > 0 { if ($0 !~ /^[[:space:]]*$/) { printf "%d:%s\n", NR, $0; left-- } }
' "$TMP/clean" > "$TMP/pw"
cp "$TMP/pw" "$TMP/lines"
grep -nE "$PATTERN" "$TMP/clean" | grep -vxF -f "$TMP/pw" >> "$TMP/lines" || true
head -80 "$TMP/lines" > "$TMP/top" && mv "$TMP/top" "$TMP/lines"
[ -s "$TMP/lines" ] || tail -n 40 "$TMP/clean" > "$TMP/lines"
split -l 12 "$TMP/lines" "$TMP/chunk-"
n=0
for c in "$TMP"/chunk-*; do
  n=$((n + 1))
  [ $n -le 9 ] || break
  msg=$(sed -e 's/%/%25/g' "$c" | awk 'BEGIN{ORS="%0A"} {print}')
  echo "::error title=$TITLE ($n)::$msg"
done
rm -rf "$TMP"

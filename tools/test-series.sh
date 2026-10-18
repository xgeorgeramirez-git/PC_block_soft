#!/usr/bin/env bash
# Полная тест-серия R4 (методика dev-methodology, лестница R0–R4). Запускается в CI
# (full-series.yml на windows-latest, Git Bash) ровно на закрытии вехи и после ревью-фиксов.
# Кэп параллелизма — чтобы не перегревать машину (GOMAXPROCS ~75% ядер, -p 2, фаззинг -parallel 2).
# Переопределение: SERIES_PROCS, FUZZ_TIME. Лог: test-results/series-<дата>.log
set -uo pipefail
cd "$(dirname "$0")/.."
mkdir -p test-results
STAMP=$(date +%Y-%m-%d_%H%M)
LOG="test-results/series-$STAMP.log"
CORES=$( (nproc 2>/dev/null || echo 4) | tr -d '\r')
export GOMAXPROCS=${SERIES_PROCS:-$(( CORES * 3 / 4 > 0 ? CORES * 3 / 4 : 1 ))}
FUZZ_TIME=${FUZZ_TIME:-60s}
TAGS="desktop,production"
FAILED=0
SUMMARY=""
START=$(date +%s)
export PYTHONIOENCODING=utf-8

say() { echo "$*" | tee -a "$LOG"; }
step() {
  local name=$1; shift
  local t0=$(date +%s) out="test-results/step.out"
  if "$@" >"$out" 2>&1; then
    cat "$out" >>"$LOG"
    say "PASS  $name ($(( $(date +%s) - t0 )) с)"; SUMMARY+="PASS $name; "
  else
    cat "$out" >>"$LOG"
    say "FAIL  $name ($(( $(date +%s) - t0 )) с), подробности в $LOG"; SUMMARY+="FAIL $name; "; FAILED=1
    # Журнал CI из песочницы агента недоступен: строки ошибок — пометками сборки.
    [ -n "${GITHUB_ACTIONS:-}" ] && bash tools/annotate-errors.sh "$out" "R4: $name"
  fi
  rm -f "$out"
}

say "Тест-серия R4, $(date '+%d.%m.%Y %H:%M'), GOMAXPROCS=$GOMAXPROCS, фаззинг $FUZZ_TIME на цель"

# R0 статика
step "gofmt" bash -c 'out=$(gofmt -l $(git ls-files "*.go")); [ -z "$out" ] || { echo "$out"; exit 1; }'
step "go vet" go vet -tags "$TAGS" ./...
step "тексты интерфейса" python tools/scan_ui_texts.py
step "tsc" bash -c 'cd frontend && npm run typecheck'

# Юниты с гонками и покрытием по пакетам
PKGS=$(go list ./... | grep -v -E '/internal/ui$' | grep -v -E '^pcblock$')
step "go test -race" env CGO_ENABLED=1 go test -race -count=1 -p 2 -tags "$TAGS" -coverprofile=test-results/cover.out $PKGS

check_coverage() {
  local fail=0
  while read -r pkg min; do
    [[ -z "$pkg" || "$pkg" == \#* ]] && continue
    [ -d "$pkg" ] || continue
    local pct
    pct=$(CGO_ENABLED=1 go test -count=1 -tags "$TAGS" -cover "./$pkg/..." 2>&1 | grep -oE 'coverage: [0-9.]+%' | head -1 | grep -oE '[0-9.]+' || echo 0)
    if awk -v p="$pct" -v m="$min" 'BEGIN{exit !(p+0 >= m+0)}'; then echo "OK   $pkg $pct% (порог $min%)"
    else echo "МАЛО $pkg $pct% (порог $min%)"; fail=1; fi
  done < tools/coverage-thresholds.txt
  return $fail
}
step "покрытие по порогам" check_coverage

# Фаззинг: каждая цель Fuzz* в каждом пакете
run_fuzz() {
  local fail=0 any=0
  for pkg in $PKGS; do
    for target in $(go test -tags "$TAGS" -list '^Fuzz' "$pkg" 2>/dev/null | grep -E '^Fuzz'); do
      any=1
      echo "фаззинг $pkg $target"
      go test -tags "$TAGS" -run '^$' -fuzz "^${target}\$" -fuzztime "$FUZZ_TIME" -parallel 2 "$pkg" || fail=1
    done
  done
  [ $any -eq 1 ] || echo "целей фаззинга пока нет"
  return $fail
}
step "фаззинг" run_fuzz

# Фронт
step "vitest" bash -c 'cd frontend && npx vitest run'

TOTAL=$(( $(date +%s) - START ))
RESULT="Итог: $([ $FAILED -eq 0 ] && echo ЗЕЛЁНАЯ || echo КРАСНАЯ), время серии $TOTAL с"
say "$RESULT"
# Итог серии виден пометкой сборки и без журнала (для песочницы агента).
[ -n "${GITHUB_ACTIONS:-}" ] && echo "::notice title=Серия R4::$RESULT. $SUMMARY"
exit $FAILED

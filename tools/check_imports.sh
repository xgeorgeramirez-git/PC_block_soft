#!/usr/bin/env bash
# Линтер импортов (NFR-1, Ruling 8 плана M1): пакеты программы pcblock/... не импортируют
# сетевые пакеты. Канал связи — именованный канал Windows через x/sys/windows, не пакет net.
# Проверяются файлы сборки для Linux и для Windows (*_windows.go) с тегами desktop,production;
# тесты не проверяются (это не программа). Код выхода 1 и строка ::error при находке.
# Запуск: bash tools/check_imports.sh
set -uo pipefail
cd "$(dirname "$0")/.."

FORBIDDEN=" net net/http net/rpc net/smtp crypto/tls "
TAGS="desktop,production"
ERRS=$(mktemp)
trap 'rm -f "$ERRS"' EXIT
found=0

for goos in linux windows; do
  if ! out=$(GOOS=$goos go list -e -tags "$TAGS" -f '{{.ImportPath}} {{join .Imports " "}}' ./... 2>"$ERRS"); then
    echo "::error title=Импорты (NFR-1)::go list не выполнен для GOOS=$goos: $(tr '\n' ' ' <"$ERRS")"
    exit 1
  fi
  while read -r pkg imports; do
    [[ "$pkg" == pcblock || "$pkg" == pcblock/* ]] || continue
    for imp in $imports; do
      case "$FORBIDDEN" in
        *" $imp "*)
          echo "::error title=Сеть запрещена (NFR-1)::$pkg импортирует $imp (GOOS=$goos)"
          found=1
          ;;
      esac
    done
  done <<<"$out"
done

if [ $found -ne 0 ]; then
  echo "Найдены сетевые импорты: программа не должна выходить в сеть (NFR-1)."
  exit 1
fi
echo "Сетевых импортов нет (NFR-1)."

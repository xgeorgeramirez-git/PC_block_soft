#!/bin/bash
# Локальный прогон пакета ядра (только стандартная библиотека) на go1.25 во временном
# модуле — обход требования go.mod 1.27, которое в облачной песочнице не ставится.
# НЕ замена CI (там go1.27 + -race на всех пакетах); быстрая проверка типов и логики ядра.
# Воссоздаёт структуру <mod>/internal/core + <mod>/testdata/golden — golden_test ищет
# эталоны по пути ../../testdata/golden.
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/internal/core" "$TMP/testdata"
cp "$ROOT"/internal/core/*.go "$TMP/internal/core/"
cp -r "$ROOT/testdata/golden" "$TMP/testdata/golden"
cat > "$TMP/go.mod" <<MOD
module coretest
go 1.25
MOD
cd "$TMP/internal/core"
GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off PATH=/usr/local/go1.25.1/bin:$PATH go test "$@" .

#!/bin/bash
# Локальный прогон указанных пакетов на go1.25 в полной копии репозитория с пониженной
# go-директивой (обход go.mod 1.27, недоступного в песочнице). Работает для пакетов на
# стандартной библиотеке и golang.org/x/sys (Windows-файлы с тегами не собираются на
# Linux). НЕ для пакетов с modernc.org/sqlite и Wails — те только в CI.
# Использование: tools/localtest.sh ./internal/ipc/... [доп. флаги go test]
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
tar -c --exclude=.git --exclude=node_modules --exclude=frontend/dist -C "$ROOT" -f - . | tar -x -C "$TMP"
sed -i 's/^go 1\.27\.0/go 1.25.0/; /^toolchain /d' "$TMP/go.mod"
cd "$TMP"
GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off PATH=/usr/local/go1.25.1/bin:$PATH go test "$@"

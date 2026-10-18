#!/usr/bin/env bash
# Загружает PNG-снимки в ветку screenshots, папка screens/<ветка>/ (как в Android-проекте заказчика).
# Использование (в CI): GH_TOKEN=... tools/push-screenshots.sh <папка-со-снимками> [префикс-имени]
# Работает через GitHub API, без checkout ветки screenshots. Не роняет сборку при сбое загрузки.
set -uo pipefail
DIR=${1:?папка со снимками}
PREFIX=${2:-}
R=${GITHUB_REPOSITORY:?}
BRANCH=${SCREENS_BRANCH_NAME:-${GITHUB_HEAD_REF:-$GITHUB_REF_NAME}}
NAME=${BRANCH//\//-}
SHA=${GITHUB_SHA:-unknown}

shopt -s nullglob
files=("$DIR"/*.png)
[ ${#files[@]} -gt 0 ] || { echo "Снимков нет в $DIR"; exit 0; }

gh api "repos/$R/git/ref/heads/screenshots" >/dev/null 2>&1 \
  || gh api -X POST "repos/$R/git/refs" -f ref=refs/heads/screenshots -f sha="$SHA" >/dev/null

n=0
for f in "${files[@]}"; do
  p="screens/$NAME/${PREFIX}$(basename "$f")"
  sha=$(gh api "repos/$R/contents/$p?ref=screenshots" --jq .sha 2>/dev/null || true)
  base64 -w0 "$f" > /tmp/screen.b64
  args=(-X PUT "repos/$R/contents/$p" -f message="Снимок $p (${SHA::7})" -f branch=screenshots -F content=@/tmp/screen.b64)
  [ -n "$sha" ] && args+=(-f sha="$sha")
  if gh api "${args[@]}" >/dev/null; then n=$((n+1)); else echo "::warning title=Снимок не загружен::$p"; fi
done
echo "::notice title=Снимки::загружено $n в ветку screenshots, папка screens/$NAME"

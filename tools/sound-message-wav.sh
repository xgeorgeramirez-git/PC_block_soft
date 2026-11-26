#!/usr/bin/env bash
# Пересобирает встроенные звуки сообщения от родителя (FR-41, D-101): каждый файл заказчика
# assets/sounds/message/N.mp3 один раз переводится в internal/sound/message/N.wav (PCM 16 бит,
# 44,1 кГц, стерео, без метаданных, bitexact, без обрезки), который встраивается в программу
# (internal/sound, //go:embed) и играется PlaySoundW из памяти. Рядом пишется sha256 каждого mp3
# (N.mp3.sha256) и общий durations.json — длина каждого mp3 по ffprobe в секундах: тест
# internal/sound сверяет с ним длину wav и падает, если mp3 заменили, а wav не пересобрали.
# Набор звуков — по файлам: новый N.mp3 + этот скрипт = новый звук, код не меняется.
# Запуск после добавления или замены mp3: bash tools/sound-message-wav.sh
set -euo pipefail
cd "$(dirname "$0")/.."
src=assets/sounds/message
dst=internal/sound/message
mkdir -p "$dst"
rm -f "$dst"/*.wav "$dst"/*.mp3.sha256 "$dst"/durations.json

nums=()
for f in "$src"/*.mp3; do
	n=$(basename "$f" .mp3)
	if ! [[ "$n" =~ ^[1-9][0-9]?$ ]]; then
		echo "Пропущен $f: имя должно быть номером 1…99" >&2
		continue
	fi
	nums+=("$n")
done
if [ ${#nums[@]} -eq 0 ]; then
	echo "Нет звуков в $src" >&2
	exit 1
fi
mapfile -t nums < <(printf '%s\n' "${nums[@]}" | sort -n)

json="{"
sep=""
for n in "${nums[@]}"; do
	in="$src/$n.mp3"
	out="$dst/$n.wav"
	ffmpeg -nostdin -loglevel error -y -i "$in" -ac 2 -ar 44100 -sample_fmt s16 -map_metadata -1 -fflags +bitexact "$out"
	sha256sum "$in" | cut -d' ' -f1 > "$dst/$n.mp3.sha256"
	d=$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$in")
	json+="$sep\"$n\": $d"
	sep=", "
	echo "Готово: $out ($(stat -c %s "$out") байт, $d с)"
done
json+="}"
printf '%s\n' "$json" > "$dst/durations.json"
echo "Длины: $dst/durations.json"

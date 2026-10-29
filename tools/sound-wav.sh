#!/usr/bin/env bash
# Пересобирает встроенный звук предупреждения (D-55, Ruling R-1 пакета 0.2.5): звук заказчика
# assets/sounds/warning.mp3 один раз переводится в wav (PCM 16 бит, 48 кГц, стерео), который
# встраивается в программу (internal/sound, //go:embed) и играется PlaySoundW из памяти без
# временных файлов. Рядом пишется sha256 исходного mp3: тест internal/sound падает, если mp3
# заменили, а wav не пересобрали. Запуск после замены mp3: bash tools/sound-wav.sh
set -euo pipefail
cd "$(dirname "$0")/.."
src=assets/sounds/warning.mp3
dst=internal/sound/warning.wav
ffmpeg -nostdin -loglevel error -y -i "$src" -ac 2 -ar 48000 -sample_fmt s16 -map_metadata -1 -fflags +bitexact "$dst"
sha256sum "$src" | cut -d' ' -f1 > internal/sound/warning.mp3.sha256
echo "Готово: $dst ($(stat -c %s "$dst") байт), отметка internal/sound/warning.mp3.sha256"

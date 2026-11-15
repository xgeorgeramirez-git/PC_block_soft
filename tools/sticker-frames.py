#!/usr/bin/env python3
"""Кадры стикеров для экрана блокировки (RM5b-9, уточнение 11 плана M5b).

Берёт стикер assets/stickers/<n>.tgs (gzip Lottie JSON, 60 кадров/с), проигрывает его
тем же проигрывателем, что и окно ребёнка (frontend/src/vendor/lottie_light.min.js,
renderer "svg"), в установленном Chrome через Playwright и снимает каждый
(60 / fps)-й кадр с прозрачным фоном (по умолчанию 30 кадров/с, D-84: каждый
второй). Кадры кладутся в горизонтальный спрайт
internal/guard/assets/stickers/<n>.png (палитра 256 цветов с прозрачностью),
число кадров - в internal/guard/assets/stickers/frames.json.

Запускает разработчик в своей песочнице, вывод коммитится (как rest-cat.png);
в CI генератор не идёт. Пример:

    python3 -I tools/sticker-frames.py            # 17 стикеров промокода (R26)
    python3 -I tools/sticker-frames.py 017 023 --size 240 --fps 30
"""

import argparse
import gzip
import io
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# R26, D-83: верный код, затем неверный.
PROMO = [
    "017", "033", "044", "049", "050", "077", "093", "096", "112",
    "023", "028", "048", "069", "082", "084", "095", "110",
]


def parse_args(argv):
    p = argparse.ArgumentParser(description="Кадры стикеров для стража (спрайт PNG).")
    p.add_argument("numbers", nargs="*", default=PROMO, help="номера стикеров (по умолчанию 17 из R26)")
    p.add_argument("--size", type=int, default=300, help="сторона кадра, px (300)")
    p.add_argument("--fps", type=int, default=30, help="кадров в секунду (30: плавный показ по кругу, D-84; бюджет 10 МБ)")
    p.add_argument("--src", type=pathlib.Path, default=ROOT / "assets" / "stickers")
    p.add_argument("--out", type=pathlib.Path, default=ROOT / "internal" / "guard" / "assets" / "stickers")
    p.add_argument("--player", type=pathlib.Path, default=ROOT / "frontend" / "src" / "vendor" / "lottie_light.min.js")
    p.add_argument("--chrome", default="/usr/bin/google-chrome")
    return p.parse_args(argv)


def frame_times(total, src_fps, fps):
    """Номера кадров Lottie, которые снимаем: 0, step, 2*step, ... < total."""
    step = src_fps / fps
    out = []
    f = 0.0
    while f < total - 1e-9:
        out.append(f)
        f += step
    return out


def render(page, player_js, anim_json, size, fps):
    from PIL import Image

    page.set_content(
        "<!DOCTYPE html><html><head><style>html,body{margin:0;background:transparent}</style></head>"
        f'<body><div id="c" style="width:{size}px;height:{size}px"></div></body></html>'
    )
    page.add_script_tag(content=player_js)
    info = page.evaluate(
        """(a) => {
            const data = JSON.parse(a);
            window.an = lottie.loadAnimation({
                container: document.getElementById('c'), renderer: 'svg',
                loop: false, autoplay: false, animationData: data,
            });
            return { total: window.an.totalFrames, fr: data.fr };
        }""",
        anim_json,
    )
    el = page.query_selector("#c")
    frames = []
    for f in frame_times(info["total"], info["fr"], fps):
        page.evaluate("(f) => window.an.goToAndStop(f, true)", f)
        png = el.screenshot(omit_background=True)
        frames.append(Image.open(io.BytesIO(png)).convert("RGBA"))
    return frames


def sprite(frames, size):
    from PIL import Image

    sheet = Image.new("RGBA", (size * len(frames), size), (0, 0, 0, 0))
    for i, fr in enumerate(frames):
        if fr.size != (size, size):
            raise SystemExit(f"кадр {i}: размер {fr.size}, ждали {size}x{size}")
        if fr.getchannel("A").getextrema()[1] == 0:
            raise SystemExit(f"кадр {i} целиком прозрачный")
        sheet.paste(fr, (i * size, 0))
    # Одна палитра на весь спрайт: 256 цветов вместе с прозрачностью (tRNS).
    pal = sheet.quantize(colors=256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.NONE)
    buf = io.BytesIO()
    pal.save(buf, format="PNG", optimize=True)
    return buf.getvalue()


def main(argv):
    args = parse_args(argv)
    from playwright.sync_api import sync_playwright

    player_js = args.player.read_text(encoding="utf-8")
    args.out.mkdir(parents=True, exist_ok=True)
    index_path = args.out / "frames.json"
    index = json.loads(index_path.read_text(encoding="utf-8")) if index_path.exists() else {}
    total = 0
    with sync_playwright() as p:
        browser = p.chromium.launch(executable_path=args.chrome, args=["--no-sandbox"])
        page = browser.new_page(viewport={"width": args.size + 20, "height": args.size + 20}, device_scale_factor=1)
        for n in args.numbers:
            anim = gzip.open(args.src / f"{n}.tgs").read().decode("utf-8")
            frames = render(page, player_js, anim, args.size, args.fps)
            data = sprite(frames, args.size)
            (args.out / f"{n}.png").write_bytes(data)
            index[n] = len(frames)
            total += len(data)
            print(f"{n}: {len(frames)} кадров, {len(data)} байт")
        browser.close()
    index_path.write_text(json.dumps(dict(sorted(index.items())), indent=2) + "\n", encoding="utf-8")
    print(f"итого {len(args.numbers)} файлов, {total} байт ({total / 1048576:.2f} МБ)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

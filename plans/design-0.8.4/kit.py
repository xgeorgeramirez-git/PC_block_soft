# Общие токены стиля «Игровое лобби» для артбордов
INK='#0B1D5C'
BG="background: radial-gradient(ellipse 90% 70% at 50% 25%, #4A86FF 0%, #2A5CE0 45%, #15318C 100%)"
FONT='<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Rubik:wght@500;600;700;800;900&amp;display=swap">'
def outl(size, color='#FFFFFF', w=None):
    w = w or max(3, round(size/11))
    return f"font-weight: 900; font-size: {size}px; line-height: 1.05; color: {color}; -webkit-text-stroke: {w}px {INK}; paint-order: stroke fill; text-shadow: 0 {max(2,round(size/16))}px 0 {INK}"
def panel(bg='#13287A', hi='#2D4DC0', r=22):
    return f"background: {bg}; border: 3px solid {INK}; border-radius: {r}px; box-shadow: inset 0 3px 0 {hi}, 0 5px 0 {INK}"
BTN={
 'gold': ('#FFE066','#FFB800','#D98200',INK),
 'green':('#7CF29B','#2FCB5C','#1C9440',INK),
 'blue': ('#7FB6FF','#3D86FF','#225FD1','#FFFFFF'),
 'coral':('#FFA48A','#FF6A4A','#D9442A','#FFFFFF'),
 'navy': ('#3552B8','#22388F','#152A70','#FFFFFF'),
}
def btn(kind, h=56, r=18, fs=18, extra=''):
    a,b,c,t = BTN[kind]
    stroke = '' if t==INK else f"-webkit-text-stroke: 3px {INK}; paint-order: stroke fill; text-shadow: 0 2px 0 {INK};"
    return f"height: {h}px; padding: 0 {round(h*0.42)}px; border-radius: {r}px; border: 3px solid {INK}; background: linear-gradient(180deg, {a} 0%, {b} 55%); box-shadow: inset 0 -6px 0 {c}, inset 0 3px 0 rgba(255,255,255,0.55), 0 4px 0 {INK}; color: {t}; {stroke} font-family: Rubik, sans-serif; font-weight: 900; font-size: {fs}px; cursor: pointer; {extra}"
def bar(pct, h=26, a='#8DFFC4', b='#22C47E', ticks=0, track='#0A1A55'):
    tick = f"background-image: repeating-linear-gradient(90deg, transparent 0, transparent calc(100%/{ticks} - 3px), rgba(11,29,92,0.55) calc(100%/{ticks} - 3px), rgba(11,29,92,0.55) calc(100%/{ticks}));" if ticks else ''
    return (f'<div style="position: relative; height: {h}px; border-radius: {h}px; background: {track}; border: 3px solid {INK}; padding: 4px; box-sizing: border-box">'
            f'<div style="width: {pct}%; height: 100%; border-radius: {h}px; background: linear-gradient(180deg, {a} 0%, {b} 60%); box-shadow: inset 0 -4px 0 rgba(0,0,0,0.18), inset 0 3px 0 rgba(255,255,255,0.5)"></div>'
            + (f'<div style="position: absolute; inset: 4px; border-radius: {h}px; {tick}"></div>' if ticks else '') + '</div>')
# заливные значки (белые) в цветных плашках
G={
 'game':'<path d="M7 7h10a5 5 0 0 1 0 10c-1.6 0-2.4-1-3.2-2h-3.6c-.8 1-1.6 2-3.2 2A5 5 0 0 1 7 7zm-.5 3.2v1.3H5.2v1.6h1.3v1.3h1.6v-1.3h1.3v-1.6H8.1v-1.3zM16 10.5a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2zm2.2 2.3a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2z"></path>',
 'video':'<path d="M5 5h14a3 3 0 0 1 3 3v8a3 3 0 0 1-3 3H5a3 3 0 0 1-3-3V8a3 3 0 0 1 3-3zm5 4v6l5-3z"></path>',
 'book':'<path d="M5 3h11a3 3 0 0 1 3 3v15H8a3 3 0 0 1-3-3zm3 13a1 1 0 0 0 0 2h9v-2z"></path>',
 'moon':'<path d="M20.5 14.6A8.5 8.5 0 1 1 9.4 3.5a7 7 0 0 0 11.1 11.1z"></path>',
 'check':'<path d="M9.5 16.2 5.3 12l-1.8 1.8 6 6L21 8.3l-1.8-1.8z"></path>',
 'key':'<path d="M14 3a7 7 0 1 1-3.3 13.2L9 18H7v2H5v2H2v-3.4l7.8-7.8A7 7 0 0 1 14 3zm2 3.5a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6z"></path>',
 'msg':'<path d="M4 4h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-5 4v-4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"></path>',
 'chart':'<path d="M4 20V10h4v10zm6 0V4h4v16zm6 0v-7h4v7z"></path>',
 'kids':'<path d="M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zm8 0a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM1 20c0-3.5 3.6-6 8-6s8 2.5 8 6zm17 0c0-2-.8-3.8-2.2-5 3.7.2 6.2 2 6.2 5z"></path>',
 'gear':'<path d="m12 2 2 2.6 3.2-.6.6 3.2L20.4 9 19.6 12l.8 3-2.6 1.8-.6 3.2-3.2-.6L12 22l-2-2.6-3.2.6-.6-3.2L3.6 15l.8-3-.8-3 2.6-1.8.6-3.2 3.2.6zm0 6.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z"></path>',
 'home':'<path d="M12 3 2 11.5h3V21h5.5v-6h3v6H19v-9.5h3z"></path>',
 'shield':'<path d="M12 2 4 5v6c0 5 3.4 9.4 8 11 4.6-1.6 8-6 8-11V5z"></path>',
 'globe':'<path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm6.9 9h-3a15 15 0 0 0-1.3-6 8 8 0 0 1 4.3 6zM12 4.1c.9 1.3 1.8 3.6 2 6.9h-4c.2-3.3 1.1-5.6 2-6.9zM9.4 5a15 15 0 0 0-1.3 6h-3a8 8 0 0 1 4.3-6zM5.1 13h3a15 15 0 0 0 1.3 6 8 8 0 0 1-4.3-6zm6.9 6.9c-.9-1.3-1.8-3.6-2-6.9h4c-.2 3.3-1.1 5.6-2 6.9zm2.6-.9a15 15 0 0 0 1.3-6h3a8 8 0 0 1-4.3 6z"></path>',
 'list':'<path d="M4 5h3v3H4zm5 .5h11v2H9zM4 10.5h3v3H4zm5 .5h11v2H9zM4 16h3v3H4zm5 .5h11v2H9z"></path>',
 'grid':'<path d="M3 3h8v8H3zm10 0h8v8h-8zM3 13h8v8H3zm10 0h8v8h-8z"></path>',
 'pause':'<path d="M6 4h4v16H6zm8 0h4v16h-4z"></path>',
}
def glyph(name, size=24, color='#FFFFFF'):
    return f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="{color}" aria-hidden="true">{G[name]}</svg>'
def badge(name, bg, size=46, g=26):
    return f'<div style="width: {size}px; height: {size}px; flex-shrink: 0; border-radius: {round(size*0.3)}px; background: {bg}; border: 3px solid {INK}; box-shadow: inset 0 -4px 0 rgba(0,0,0,0.2), inset 0 3px 0 rgba(255,255,255,0.4); display: flex; align-items: center; justify-content: center">{glyph(name,g)}</div>'
COIN='<svg width="{s}" height="{s}" viewBox="0 0 40 40" aria-hidden="true"><circle cx="20" cy="21.5" r="16" fill="#C97B00" stroke="#0B1D5C" stroke-width="3"></circle><circle cx="20" cy="19" r="16" fill="#FFC53A" stroke="#0B1D5C" stroke-width="3"></circle><circle cx="20" cy="19" r="10.5" fill="none" stroke="#E79A00" stroke-width="2.5"></circle><path d="M20 12.5v13M16.5 15.5h5a2.5 2.5 0 0 1 0 5h-3a2.5 2.5 0 0 0 0 5h5" fill="none" stroke="#B56A00" stroke-width="0"></path><path d="M20 13v6.2l3.6 2.2" fill="none" stroke="#A85E00" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"></path></svg>'
def coin(s=34): return COIN.replace('{s}',str(s))
def page(title, w, h, body, lang='ru'):
    return f'''<!doctype html>
<html lang="{lang}">
<head>
<meta charset="utf-8">
<title>{title}</title>
<script src="./support.js"></script>
</head>
<body>
<x-dc>
<helmet>
{FONT}
<style>
body{{margin:0}}
a{{color:#FFE066}}a:hover{{color:#FFFFFF}}
</style>
</helmet>
{body}
</x-dc>
<script type="text/x-dc" data-dc-script data-props='{{"$preview":{{"width":{w},"height":{h}}}}}'>
class Component extends DCLogic {{
renderVals() {{ return {{}}; }}
}}
</script>
</body>
</html>
'''

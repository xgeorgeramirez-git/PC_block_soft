# Взрослый стиль родителя: спокойный, общий синий с детским окном
INK='#1A2238'; MUTED='#5B6680'; LINE='#DDE3EE'; GROUND='#F3F5F9'; BLUE='#2552D9'; BLUE_BG='#E6EDFF'
GOLD='#E89A00'; GREEN='#138A4F'; RED='#C2410C'
FONT='<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&amp;display=swap">'
NUM="font-variant-numeric: tabular-nums"
def chip(t, kind='plain', h=40, extra=''):
    c={'plus':(GREEN,'#E7F5EC'),'minus':(RED,'#FCEDE6'),'plain':(INK,'#FFFFFF'),'blue':('#FFFFFF',BLUE),'gold':('#5A3B00','#FFE7AD')}[kind]
    bd = LINE if kind=='plain' else c[1]
    return f'<button style="height: {h}px; padding: 0 14px; border-radius: 10px; border: 1px solid {bd}; background: {c[1]}; color: {c[0]}; font-family: \'Inter\', sans-serif; font-weight: 600; font-size: 15px; {NUM}; cursor: pointer; {extra}">{t}</button>'
def bar(pct, h=10, col=BLUE, ticks=8):
    t = f"background-image: repeating-linear-gradient(90deg, transparent 0, transparent calc(100%/{ticks} - 2px), #FFFFFF calc(100%/{ticks} - 2px), #FFFFFF calc(100%/{ticks}));"
    return f'<div style="position: relative; height: {h}px; border-radius: {h}px; background: #E3E8F2; overflow: hidden"><div style="width: {pct}%; height: 100%; background: {col}"></div><div style="position: absolute; inset: 0; {t}"></div></div>'
G={
 'msg':'<path d="M4 5h16v11H9l-5 4z"></path>','chart':'<path d="M5 20V11M12 20V5M19 20v-7"></path>',
 'moon':'<path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z"></path>','key':'<circle cx="8" cy="15" r="4"></circle><path d="M11 12l8-8M16 7l2 2M14 9l2 2"></path>',
 'kids':'<circle cx="9" cy="8" r="3.5"></circle><path d="M2.5 20c.6-3.6 3.2-5.5 6.5-5.5s5.9 1.9 6.5 5.5M16 4.8a3.2 3.2 0 0 1 0 6.4M18 14.6c2 .6 3.2 2.4 3.5 5.4"></path>',
 'gear':'<circle cx="12" cy="12" r="3"></circle><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1-2.1M16.6 7.4l2.1-2.1"></path>',
 'home':'<path d="M3 11 12 4l9 7M6 9.5V20h12V9.5"></path>','grid':'<rect x="4" y="4" width="7" height="7" rx="1.5"></rect><rect x="13" y="4" width="7" height="7" rx="1.5"></rect><rect x="4" y="13" width="7" height="7" rx="1.5"></rect><rect x="13" y="13" width="7" height="7" rx="1.5"></rect>',
 'globe':'<circle cx="12" cy="12" r="9"></circle><path d="M3 12h18M12 3c2.5 2.6 3.7 5.6 3.7 9s-1.2 6.4-3.7 9c-2.5-2.6-3.7-5.6-3.7-9S9.5 5.6 12 3z"></path>',
 'list':'<path d="M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01"></path>','pause':'<path d="M9 5v14M15 5v14"></path>',
 'chev':'<path d="M9 6l6 6-6 6"></path>','shield':'<path d="M12 3 5 6v5c0 4.5 3 8.3 7 10 4-1.7 7-5.5 7-10V6z"></path>',
 'unlock':'<rect x="5" y="11" width="14" height="9" rx="2"></rect><path d="M8 11V8a4 4 0 0 1 7.5-2"></path>','plus':'<path d="M12 5v14M5 12h14"></path>',
}
def ic(name, size=22, color=INK, w=1.8):
    return f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="{w}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">{G[name]}</svg>'
def coin(s=20):
    return f'<svg width="{s}" height="{s}" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10" fill="#FFC53A" stroke="#B87800" stroke-width="1.5"></circle><path d="M12 7.5V12l3 2" fill="none" stroke="#8A5A00" stroke-width="1.8" stroke-linecap="round"></path></svg>'
def avatar(letter, bg, s=40):
    return f'<div style="width: {s}px; height: {s}px; border-radius: 50%; background: {bg}; color: {INK}; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: {round(s*0.42)}px; flex-shrink: 0">{letter}</div>'
def page(title, w, h, body):
    return f'''<!doctype html>
<html lang="ru">
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
a{{color:{BLUE}}}a:hover{{color:#163A9E}}
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

T_GREEN=('#138A4F','#1FA463'); T_BLUE=('#2552D9','#2552D9'); T_RED=('#C81E1E','#E5484D')
def tcol(minutes):
    return T_GREEN if minutes>60 else (T_BLUE if minutes>15 else T_RED)

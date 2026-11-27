from kit2 import *
from phones_more import frame, head, card, nav, bigbtn, FF
def dec(t,kind):
    c={'game':(GREEN,'#E7F5EC'),'study':(BLUE,BLUE_BG),'other':(INK,'#FFFFFF'),'ban':(T_RED[0],'#FCE8E8')}[kind]
    return f'<button style="height: 46px; border-radius: 12px; border: 1px solid {LINE if kind=="other" else c[1]}; background: {c[1]}; color: {c[0]}; {FF}; font-weight: 600; font-size: 15px">{t}</button>'
def prog(name, pub, who, when, path):
    return card(f'''
  <div style="display: flex; flex-direction: column; gap: 2px"><span style="font-weight: 700; font-size: 18px">{name}</span><span style="font-weight: 500; font-size: 13px; color: {MUTED}">{pub}</span></div>
  <div style="font-weight: 500; font-size: 14px; color: {INK}">{who}</div>
  <div style="font-weight: 500; font-size: 12px; color: {MUTED}; word-break: break-all">{path}</div>
  <div style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px">{dec('Игра','game')}{dec('Учёба','study')}{dec('Прочее','other')}{dec('Запретить','ban')}</div>''')
inner = head('Ждут решения') + f'<div style="font-weight: 500; font-size: 14px; color: {MUTED}">Решение действует для всех детей. Ребёнок увидит, что вы решили, и сможет запустить программу снова.</div>' + \
  prog('SuperGame','Без подписи издателя','Миша, 3 попытки, последняя в 18:41',"когда — сегодня",r'C:\Users\Misha\Downloads\SuperGame\SuperGame.exe').replace("<div style=\"font-weight: 500; font-size: 14px; color: #1A2238\">Миша","<div style=\"font-weight: 500; font-size: 14px; color: #1A2238\">Миша") + \
  prog('Steam','Valve Corp.','Аня, 1 попытка в 16:02','',r'C:\Program Files (x86)\Steam\steam.exe') + \
  f'<div style="font-weight: 500; font-size: 13px; color: {MUTED}">Данные на 18:42</div>' + nav('Дети')
open('../project/PhonePending.dc.html','w',encoding='utf-8').write(page('Телефон: ждут решения',390,900,frame('x',inner,h=900)))

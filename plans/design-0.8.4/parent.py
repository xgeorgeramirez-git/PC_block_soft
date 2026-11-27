from kit import *
menu=[('home','Сегодня',True),('kids','Дети',False),('grid','Программы',False),('globe','Сайты',False),('chart','Статистика',False),('key','Промокоды',False),('list','Журнал',False),('gear','Настройки',False)]
def item(ic,t,on):
    if on:
        return f'<a href="#" style="{btn("gold",50,14,17,"display: flex; align-items: center; gap: 12px; text-decoration: none; padding: 0 16px")}">{glyph(ic,22,INK)}{t}</a>'
    return f'<a href="#" style="height: 48px; display: flex; align-items: center; gap: 12px; padding: 0 16px; border-radius: 14px; color: #DCE4FF; text-decoration: none; font-weight: 700; font-size: 17px">{glyph(ic,22,"#9FB3F0")}{t}</a>'
def small(kind,t): return f'<button style="{btn(kind,44,12,15,"padding: 0 14px")}">{t}</button>'
def child(av,avbg,name,status,left,total,pct,a,b,games,video,coins,forbid):
    return f'''<div style="{panel('#132C8A','#3A5DD6',24)}; padding: 20px 22px; display: flex; flex-direction: column; gap: 14px">
      <div style="display: flex; align-items: center; justify-content: space-between">
        <div style="display: flex; align-items: center; gap: 12px">
          <div style="width: 50px; height: 50px; border-radius: 50%; background: {avbg}; border: 3px solid {INK}; display: flex; align-items: center; justify-content: center; {outl(22)}">{av}</div>
          <div style="display: flex; flex-direction: column; gap: 2px"><div style="{outl(24)}">{name}</div><div style="font-weight: 600; font-size: 14px; color: #C7D4FF">{status}</div></div>
        </div>
        <div style="display: flex; align-items: center; gap: 8px; height: 40px; padding: 0 14px 0 4px; border-radius: 20px; background: #0E1F66; border: 3px solid {INK}">{coin(30)}<span style="{outl(17,'#FFD54A',3)}">{coins}</span></div>
      </div>
      <div style="display: flex; align-items: baseline; justify-content: space-between"><span style="{outl(38)}">{left}</span><span style="font-weight: 700; font-size: 14px; color: #C7D4FF">осталось из {total}</span></div>
      {bar(pct,30,a,b,8)}
      <div style="display: flex; gap: 18px; font-weight: 700; font-size: 15px; color: #DCE4FF">
        <span style="display: flex; align-items: center; gap: 6px">{glyph('game',18,'#FFB199')}Игры: {games}</span>
        <span style="display: flex; align-items: center; gap: 6px">{glyph('video',18,'#A9CCFF')}Видео: {video}</span>
        <span style="display: flex; align-items: center; gap: 6px">{glyph('moon',18,'#C9C2FF')}{forbid}</span>
      </div>
      <div style="display: flex; gap: 8px; flex-wrap: wrap">{small('green','+15')}{small('green','+30')}{small('green','+60')}{small('coral','−15')}{small('navy','Своё число')}{small('navy','Пауза')}</div>
    </div>'''
body=f'''<div style="width: 1280px; height: 800px; box-sizing: border-box; {BG}; font-family: Rubik, system-ui, sans-serif; color: #FFFFFF; display: flex">
  <div style="width: 250px; flex-shrink: 0; background: #0E1F66; border-right: 3px solid {INK}; padding: 22px 16px; box-sizing: border-box; display: flex; flex-direction: column; gap: 6px">
    <div style="{outl(26)}; padding: 0 8px 18px">PC Block</div>
    {''.join(item(*m) for m in menu)}
    <div style="margin-top: auto; padding: 0 8px; font-weight: 600; font-size: 13px; color: #9FB3F0">Версия 0.8.4</div>
  </div>
  <div style="flex-grow: 1; padding: 26px 30px; box-sizing: border-box; display: flex; flex-direction: column; gap: 20px; min-width: 0">
    <div style="display: flex; align-items: center; justify-content: space-between">
      <div style="display: flex; flex-direction: column; gap: 4px"><div style="{outl(36)}">Сегодня</div><div style="font-weight: 700; font-size: 15px; color: #C7D4FF">Пятница, 27 ноября</div></div>
      <button style="{btn('navy',48,14,16)}">Выйти</button>
    </div>
    <div style="display: flex; gap: 20px; align-items: flex-start">
      <div style="flex: 1; display: flex; flex-direction: column; gap: 18px; min-width: 0">
        {child('М','linear-gradient(180deg,#FFE066,#FFB800)','Миша','за компьютером с 18:20','1 ч 25 мин','2 ч',71,'#8DFFC4','#22C47E','20 мин','20 мин','45 мин','Запрет с 21:00')}
        {child('А','linear-gradient(180deg,#9FD0FF,#3D86FF)','Аня','не за компьютером с 17:05','40 мин','2 ч',33,'#FFC2A8','#FF6A4A','10 мин','0 мин','120 мин','Запрет с 21:30')}
      </div>
      <div style="width: 300px; display: flex; flex-direction: column; gap: 18px">
        <div style="{panel('#3B2A9E','#5A47D6')}; padding: 18px; display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; align-items: center; gap: 12px">{badge('shield','#FF6A4A',44,24)}<div style="font-weight: 800; font-size: 18px">Ждут решения</div></div>
          <div style="font-weight: 600; font-size: 15px; color: #DCD3FF">2 новые программы: Roblox Studio и Steam</div>
          <button style="{btn('gold',48,14,17)}">Разобрать</button>
        </div>
        <div style="{panel()}; padding: 18px; display: flex; flex-direction: column; gap: 10px">
          <div style="font-weight: 800; font-size: 18px">Последние события</div>
          <div style="font-weight: 600; font-size: 14px; color: #DCE4FF">18:42 · Миша: +15 мин с телефона</div>
          <div style="font-weight: 600; font-size: 14px; color: #DCE4FF">18:20 · Миша сел за компьютер</div>
          <div style="font-weight: 600; font-size: 14px; color: #DCE4FF">17:05 · Аня вышла</div>
          <a href="#" style="font-weight: 800; font-size: 14px">Весь журнал</a>
        </div>
      </div>
    </div>
  </div>
</div>'''
open('../project/Parent.dc.html','w',encoding='utf-8').write(page('Окно родителя: игровое лобби',1280,800,body))

from kit2 import *
menu=[('home','Сегодня',True),('kids','Дети',False),('grid','Программы',False),('globe','Сайты',False),('chart','Статистика',False),('key','Промокоды',False),('list','Журнал',False),('gear','Настройки',False)]
def item(i,t,on):
    st = f"background: {BLUE_BG}; color: {BLUE}; font-weight: 700" if on else f"color: {INK}; font-weight: 500"
    return f'<a href="#" style="height: 44px; display: flex; align-items: center; gap: 12px; padding: 0 14px; border-radius: 10px; text-decoration: none; font-size: 16px; {st}">{ic(i,20,BLUE if on else MUTED)}{t}</a>'
def child(av,avbg,name,status,left,total,pct,games,video,coins,forbid,live,mins=85):
    tc=tcol(mins)
    dot = GREEN if live else '#A4AEC2'
    return f'''<div style="background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: 20px 22px; display: flex; gap: 28px; align-items: center">
      <div style="width: 200px; display: flex; flex-direction: column; gap: 10px">
        <div style="display: flex; align-items: center; gap: 12px">{avatar(av,avbg,48)}<div style="display: flex; flex-direction: column"><span style="font-weight: 700; font-size: 20px">{name}</span><span style="display: flex; align-items: center; gap: 6px; font-weight: 500; font-size: 13px; color: {MUTED}"><span style="width: 8px; height: 8px; border-radius: 50%; background: {dot}"></span>{status}</span></div></div>
        <div style="display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: 14px; {NUM}">{coin(18)}В копилке {coins}</div>
        <div style="display: flex; align-items: center; gap: 6px; font-weight: 500; font-size: 14px; color: {MUTED}">{ic('moon',16,MUTED)}{forbid}</div>
      </div>
      <div style="flex-grow: 1; display: flex; flex-direction: column; gap: 10px; min-width: 0">
        <div style="display: flex; align-items: baseline; gap: 10px"><span style="font-weight: 800; font-size: 44px; letter-spacing: -0.5px; color: {tc[0]}; {NUM}">{left}</span><span style="font-weight: 500; font-size: 15px; color: {MUTED}">осталось из {total}</span></div>
        {bar(pct,12,tc[1])}
        <div style="display: flex; gap: 20px; font-weight: 500; font-size: 14px; color: {MUTED}; {NUM}"><span>Игры {games}</span><span>Видео и соцсети {video}</span></div>
      </div>
      <div style="width: 300px; display: flex; flex-direction: column; gap: 8px">
        <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{chip('+15','plus')}{chip('+30','plus')}{chip('+60','plus')}{chip('Своё')}</div>
        <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{chip('−15','minus')}{chip('−30','minus')}{chip('−60','minus')}{chip(ic('pause',16,INK)+'Пауза','plain',40,'padding: 0 4px; display: flex; align-items: center; justify-content: center; gap: 4px')}</div>
        {chip(coin(18)+' Начислить в копилку','gold',40,'display: flex; align-items: center; justify-content: center; gap: 6px')}
      </div>
    </div>'''
ev=[('18:42','Миша','+15 минут с телефона'),('18:20','Миша','сел за компьютер'),('17:05','Аня','вышла из компьютера'),('16:30','Аня','промокод на 30 минут')]
evs=''.join(f'<div style="display: flex; gap: 12px; padding: 10px 0; border-top: 1px solid {LINE}; font-size: 14px"><span style="width: 44px; color: {MUTED}; font-weight: 500; {NUM}">{t}</span><span style="flex: 1"><b style="font-weight: 600">{n}</b> {x}</span></div>' for t,n,x in ev)
body=f'''<div style="width: 1280px; height: 800px; box-sizing: border-box; background: {GROUND}; font-family: 'Inter', system-ui, sans-serif; color: {INK}; display: flex">
  <div style="width: 240px; flex-shrink: 0; background: #FFFFFF; border-right: 1px solid {LINE}; padding: 22px 14px; box-sizing: border-box; display: flex; flex-direction: column; gap: 4px">
    <div style="display: flex; align-items: center; gap: 10px; padding: 0 10px 22px"><div style="width: 32px; height: 32px; border-radius: 9px; background: {BLUE}; display: flex; align-items: center; justify-content: center">{ic('shield',18,'#FFFFFF',2)}</div><span style="font-weight: 800; font-size: 19px">PC Block</span></div>
    {''.join(item(*m) for m in menu)}
    <div style="margin-top: auto; padding: 0 10px; font-weight: 500; font-size: 13px; color: {MUTED}">Версия 0.8.4</div>
  </div>
  <div style="flex-grow: 1; padding: 28px 32px; box-sizing: border-box; display: flex; flex-direction: column; gap: 20px; min-width: 0">
    <div style="display: flex; align-items: flex-end; justify-content: space-between">
      <div style="display: flex; flex-direction: column; gap: 2px"><div style="font-weight: 700; font-size: 30px">Сегодня</div><div style="font-weight: 500; font-size: 15px; color: {MUTED}">Пятница, 27 ноября</div></div>
      {chip('Выйти','plain',40)}
    </div>
    <div style="display: flex; gap: 20px; align-items: flex-start">
      <div style="flex: 1; display: flex; flex-direction: column; gap: 16px; min-width: 0">
        {child('М','#FFE7AD','Миша','За компьютером с 18:20','1:25','2:00',71,'40 из 60 мин','10 из 30 мин','45 мин','Запрет с 21:00 до 07:00',True)}
        {child('А','#D6E4FF','Аня','Не за компьютером с 17:05','0:10','2:00',8,'50 из 60 мин','30 из 30 мин','120 мин','Запрет с 21:30 до 07:00',False,10)}
      </div>
    </div>
    <div style="display: flex; gap: 16px; align-items: stretch">
      <div style="width: 380px; background: #FFF6E0; border: 1px solid #F3D99A; border-radius: 18px; padding: 18px 20px; display: flex; flex-direction: column; gap: 10px">
        <div style="font-weight: 700; font-size: 17px">2 программы ждут решения</div>
        <div style="font-weight: 500; font-size: 14px; color: #6B5320">Roblox Studio и Steam. Пока вы не решите, ребёнок не сможет их запустить.</div>
        <div>{chip('Разобрать','blue',40)}</div>
      </div>
      <div style="flex-grow: 1; background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: 16px 20px 6px; display: flex; flex-direction: column">
        <div style="display: flex; justify-content: space-between; padding-bottom: 8px"><span style="font-weight: 700; font-size: 17px">Последние события</span><a href="#" style="font-weight: 600; font-size: 14px; text-decoration: none">Весь журнал</a></div>
        {evs}
      </div>
    </div>
  </div>
</div>'''
open('../project/Parent.dc.html','w',encoding='utf-8').write(page('Окно родителя',1280,800,body))

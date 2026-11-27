from kit2 import *
def row(icon, t, sub=None, col=INK):
    s = f'<span style="font-weight: 500; font-size: 13px; color: {MUTED}">{sub}</span>' if sub else ''
    return f'<a href="#" style="min-height: 52px; display: flex; align-items: center; gap: 12px; padding: 0 4px; border-top: 1px solid {LINE}; color: {col}; text-decoration: none">{ic(icon,22,col)}<span style="flex-grow: 1; display: flex; flex-direction: column"><span style="font-weight: 600; font-size: 16px">{t}</span>{s}</span>{ic("chev",18,MUTED)}</a>'
body=f'''<div style="width: 390px; height: 844px; box-sizing: border-box; padding: 18px 16px 0; background: {GROUND}; font-family: 'Inter', system-ui, sans-serif; color: {INK}; display: flex; flex-direction: column; gap: 14px">
  <div style="display: flex; align-items: center; justify-content: space-between">
    <div style="font-weight: 700; font-size: 26px">Дети</div>
    <div style="display: flex; align-items: center; gap: 6px; font-weight: 500; font-size: 13px; color: {MUTED}"><span style="width: 8px; height: 8px; border-radius: 50%; background: {GREEN}"></span>Компьютер на связи</div>
  </div>

  <div style="background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: 16px; display: flex; flex-direction: column; gap: 12px">
    <div style="display: flex; align-items: center; gap: 10px">
      {avatar('М','#FFE7AD')}
      <div style="flex-grow: 1; display: flex; flex-direction: column"><span style="font-weight: 700; font-size: 18px">Миша</span><span style="font-weight: 500; font-size: 13px; color: {MUTED}">За компьютером с 18:20</span></div>
      <div style="display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: 14px; {NUM}">{coin()}45 мин</div>
    </div>
    <div style="display: flex; align-items: baseline; gap: 8px"><span style="font-weight: 800; font-size: 40px; letter-spacing: -0.5px; color: {tcol(85)[0]}; {NUM}">1:25</span><span style="font-weight: 500; font-size: 14px; color: {MUTED}">осталось из 2:00</span></div>
    {bar(71,10,tcol(85)[1])}
    <div style="display: flex; gap: 14px; font-weight: 500; font-size: 13px; color: {MUTED}; {NUM}"><span>Игры 40 из 60 мин</span><span>Видео 10 из 30 мин</span></div>
    <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{chip('+15','plus')}{chip('+30','plus')}{chip('+60','plus')}{chip('Своё','plain')}</div>
    <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{chip('−15','minus')}{chip('−30','minus')}{chip('−60','minus')}{chip(coin(18)+' В копилку','gold',40,'padding: 0 6px; display: flex; align-items: center; justify-content: center; gap: 4px; font-size: 13px')}</div>
    <a href="#" style="min-height: 52px; display: flex; align-items: center; gap: 10px; padding: 0 12px; border-radius: 12px; background: #FFF6E0; border: 1px solid #F3D99A; color: #5A3B00; text-decoration: none">{ic('grid',22,'#8A5A00')}<span style="flex-grow: 1; font-weight: 700; font-size: 15px">1 игра ждёт решения</span>{ic('chev',18,'#8A5A00')}</a>
    <div style="display: flex; flex-direction: column">
      {row('msg','Написать сообщение')}
      {row('chart','Статистика за сегодня')}
      {row('unlock','Снять запрет на сегодня','запрет с 21:00 до 07:00')}
    </div>
  </div>

  <div style="background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: 14px 16px; display: flex; align-items: center; gap: 10px">
    {avatar('А','#D6E4FF')}
    <div style="flex-grow: 1; display: flex; flex-direction: column; gap: 6px">
      <div style="display: flex; justify-content: space-between"><span style="font-weight: 700; font-size: 16px">Аня</span><span style="font-weight: 700; font-size: 16px; color: {tcol(10)[0]}; {NUM}">0:10</span></div>
      {bar(8,8,tcol(10)[1])}
      <span style="font-weight: 500; font-size: 13px; color: {MUTED}">Не за компьютером с 17:05</span>
    </div>
    {ic('chev',18,MUTED)}
  </div>

  <div style="margin-top: auto; margin-left: -16px; margin-right: -16px; height: 72px; background: #FFFFFF; border-top: 1px solid {LINE}; display: flex; align-items: center; justify-content: space-around">
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 3px; color: {BLUE}; text-decoration: none; font-weight: 600; font-size: 12px">{ic('kids',24,BLUE)}Дети</a>
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 3px; color: {MUTED}; text-decoration: none; font-weight: 500; font-size: 12px">{ic('key',24,MUTED)}Промокоды</a>
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 3px; color: {MUTED}; text-decoration: none; font-weight: 500; font-size: 12px">{ic('gear',24,MUTED)}Настройки</a>
  </div>
</div>'''
open('../project/Phone.dc.html','w',encoding='utf-8').write(page('Телефон родителя',390,844,body))

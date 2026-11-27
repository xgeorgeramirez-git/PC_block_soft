from kit import *
M033='/_blob/c7f2d5cd4e68fdd0ecc2ce878ceacfa6'
def limit(icon, bg, title, big, col, pct, a, b, sub):
    return f'''<div style="flex: 1; {panel()}; padding: 16px 18px; display: flex; flex-direction: column; gap: 10px">
      <div style="display: flex; align-items: center; gap: 12px">{badge(icon,bg)}<div style="font-weight: 800; font-size: 19px; color: #FFFFFF">{title}</div></div>
      <div style="{outl(34,col)}">{big}</div>
      {bar(pct,22,a,b) if pct is not None else '<div style="height: 22px; display: flex; align-items: center; font-weight: 800; font-size: 16px; color: #9FF0C4">Занимайся сколько хочешь</div>'}
      <div style="font-weight: 600; font-size: 15px; color: #C7D4FF">{sub}</div>
    </div>'''
body=f'''<div style="width: 1040px; height: 680px; box-sizing: border-box; padding: 24px 30px 20px; {BG}; font-family: Rubik, system-ui, sans-serif; color: #FFFFFF; display: flex; flex-direction: column; gap: 18px">
  <div style="display: flex; align-items: center; justify-content: space-between">
    <div style="display: flex; align-items: center; gap: 14px">
      <div style="width: 56px; height: 56px; border-radius: 50%; background: linear-gradient(180deg,#FFE066,#FFB800); border: 3px solid {INK}; box-shadow: 0 4px 0 {INK}; display: flex; align-items: center; justify-content: center; {outl(26)}">М</div>
      <div style="display: flex; flex-direction: column; gap: 4px">
        <div style="{outl(30)}">Привет, Миша!</div>
        <div style="font-weight: 700; font-size: 15px; color: #C7D4FF">Пятница, 27 ноября</div>
      </div>
    </div>
    <div style="display: flex; align-items: center; gap: 14px">
      <div style="height: 52px; display: flex; align-items: center; gap: 10px; padding: 0 18px 0 8px; border-radius: 26px; background: #0E1F66; border: 3px solid {INK}; box-shadow: inset 0 3px 0 #2D4DC0">
        {coin(38)}
        <div style="display: flex; flex-direction: column; line-height: 1.1"><span style="font-weight: 700; font-size: 13px; color: #C7D4FF">В копилке</span><span style="{outl(20,'#FFD54A',3)}">45 мин</span></div>
      </div>
      <button style="{btn('blue',52,16,17,'display: flex; align-items: center; gap: 8px')}">{glyph('key',22)}Ввести код</button>
    </div>
  </div>

  <div style="position: relative; {panel('#132C8A','#3A5DD6',28)}; padding: 22px 30px 22px 220px; display: flex; flex-direction: column; gap: 12px">
    <img src="{M033}" alt="" style="position: absolute; left: 6px; top: -26px; width: 210px; height: 210px; object-fit: contain">
    <div style="display: flex; align-items: flex-end; justify-content: space-between">
      <div style="display: flex; flex-direction: column; gap: 6px">
        <div style="font-weight: 800; font-size: 19px; color: #BFD0FF">Осталось сегодня</div>
        <div style="{outl(70)}">1 ч 25 мин</div>
      </div>
      <div style="font-weight: 700; font-size: 16px; color: #C7D4FF; text-align: right; padding-bottom: 8px">потратил 35 мин<br>из 2 часов</div>
    </div>
    {bar(71,40,'#8DFFC4','#22C47E',8)}
    <div style="font-weight: 600; font-size: 14px; color: #A9BCF5">Одно деление — 15 минут</div>
  </div>

  <div style="display: flex; gap: 16px">
    {limit('game','#FF6A4A','Игры','20 мин','#FFB199',33,'#FFC2A8','#FF6A4A','осталось из 1 часа')}
    {limit('video','#3D86FF','Видео и соцсети','20 мин','#A9CCFF',67,'#B8D6FF','#3D86FF','осталось из 30 минут')}
    {limit('book','#22B573','Учёба','Без лимита','#9FF0C4',None,'','','учёба не тратит твоё время')}
  </div>

  <div style="display: flex; gap: 16px; align-items: stretch; flex-grow: 1">
    <div style="flex: 1; {panel('#2E2A8F','#4A44C2')}; padding: 14px 20px; display: flex; align-items: center; gap: 16px">
      {badge('moon','#7B6CFF',52,28)}
      <div style="display: flex; flex-direction: column; gap: 4px">
        <div style="font-weight: 800; font-size: 18px">Ночью компьютер спит</div>
        <div style="{outl(26,'#D9D3FF',3)}">с 21:00 до 07:00</div>
      </div>
    </div>
    <div style="width: 330px; display: flex; flex-direction: column; gap: 8px; justify-content: center">
      <button style="{btn('gold',72,22,24,'width: 100%')}">Попросить ещё</button>
      <div style="font-weight: 600; font-size: 14px; color: #C7D4FF; text-align: center">родитель введёт пароль, и время добавится</div>
    </div>
  </div>

  <div style="display: flex; justify-content: space-between; align-items: center">
    <div style="display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 15px">{badge('check','#22B573',28,16)}Нарушений сегодня нет</div>
    <a href="#" style="font-weight: 800; font-size: 15px">Вся моя статистика</a>
  </div>
</div>'''
open('../project/Lobby.dc.html','w',encoding='utf-8').write(page('Окно ребёнка: игровое лобби',1040,680,body))

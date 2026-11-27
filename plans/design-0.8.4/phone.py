from kit import *
def tb(kind, label, w='auto'):
    return f'<button style="{btn(kind,48,14,17,f"flex: 1; padding: 0")}">{label}</button>'
body=f'''<div style="width: 390px; height: 844px; box-sizing: border-box; padding: 20px 16px 0; {BG}; font-family: Rubik, system-ui, sans-serif; color: #FFFFFF; display: flex; flex-direction: column; gap: 14px">
  <div style="display: flex; align-items: center; justify-content: space-between">
    <div style="{outl(24)}">PC Block</div>
    <div style="display: flex; align-items: center; gap: 8px; height: 34px; padding: 0 12px; border-radius: 17px; background: #0E1F66; border: 3px solid {INK}; font-weight: 700; font-size: 14px"><span style="width: 10px; height: 10px; border-radius: 50%; background: #3BE37A; border: 2px solid {INK}"></span>Компьютер на связи</div>
  </div>

  <div style="{panel('#132C8A','#3A5DD6',24)}; padding: 16px; display: flex; flex-direction: column; gap: 12px">
    <div style="display: flex; align-items: center; justify-content: space-between">
      <div style="display: flex; align-items: center; gap: 10px">
        <div style="width: 44px; height: 44px; border-radius: 50%; background: linear-gradient(180deg,#FFE066,#FFB800); border: 3px solid {INK}; display: flex; align-items: center; justify-content: center; {outl(20)}">М</div>
        <div style="display: flex; flex-direction: column; gap: 2px"><div style="{outl(22)}">Миша</div><div style="font-weight: 600; font-size: 13px; color: #C7D4FF">за компьютером с 18:20</div></div>
      </div>
      <div style="display: flex; align-items: center; gap: 6px; height: 38px; padding: 0 12px 0 4px; border-radius: 19px; background: #0E1F66; border: 3px solid {INK}">{coin(28)}<span style="{outl(16,'#FFD54A',3)}">45 мин</span></div>
    </div>
    <div style="display: flex; align-items: baseline; justify-content: space-between"><span style="{outl(34)}">1 ч 25 мин</span><span style="font-weight: 700; font-size: 13px; color: #C7D4FF">осталось из 2 ч</span></div>
    {bar(71,28,'#8DFFC4','#22C47E',8)}
    <div style="display: flex; gap: 8px">{tb('green','+15')}{tb('green','+30')}{tb('green','+60')}</div>
    <div style="display: flex; gap: 8px">{tb('coral','−15')}{tb('coral','−30')}{tb('coral','−60')}</div>
    <div style="display: flex; gap: 8px">
      <button style="{btn('gold',52,16,17,'flex: 1; padding: 0; display: flex; align-items: center; justify-content: center; gap: 6px')}">{coin(24)}В копилку</button>
      <button style="{btn('navy',52,16,16,'flex: 1; padding: 0')}">Своё число</button>
    </div>
    <div style="display: flex; gap: 8px">
      <button style="{btn('blue',48,14,15,'flex: 1; padding: 0; display: flex; align-items: center; justify-content: center; gap: 6px')}">{glyph('msg',20)}Сообщение</button>
      <button style="{btn('blue',48,14,15,'flex: 1; padding: 0; display: flex; align-items: center; justify-content: center; gap: 6px')}">{glyph('chart',20)}Статистика</button>
    </div>
  </div>

  <div style="{panel()}; padding: 12px 16px; display: flex; align-items: center; gap: 12px">
    <div style="width: 40px; height: 40px; border-radius: 50%; background: linear-gradient(180deg,#9FD0FF,#3D86FF); border: 3px solid {INK}; display: flex; align-items: center; justify-content: center; {outl(18)}">А</div>
    <div style="flex-grow: 1; display: flex; flex-direction: column; gap: 6px">
      <div style="display: flex; justify-content: space-between"><span style="font-weight: 800; font-size: 17px">Аня</span><span style="font-weight: 800; font-size: 15px; color: #FFB199">40 мин</span></div>
      {bar(33,18,'#FFC2A8','#FF6A4A')}
    </div>
  </div>

  <div style="margin-top: auto; margin-left: -16px; margin-right: -16px; height: 76px; background: #0E1F66; border-top: 3px solid {INK}; display: flex; align-items: center; justify-content: space-around">
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 4px; color: #FFD54A; text-decoration: none; font-weight: 800; font-size: 13px">{glyph('kids',26,'#FFD54A')}Дети</a>
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 4px; color: #C7D4FF; text-decoration: none; font-weight: 700; font-size: 13px">{glyph('key',26,'#C7D4FF')}Промокоды</a>
    <a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 4px; color: #C7D4FF; text-decoration: none; font-weight: 700; font-size: 13px">{glyph('gear',26,'#C7D4FF')}Настройки</a>
  </div>
</div>'''
open('../project/Phone.dc.html','w',encoding='utf-8').write(page('Телефон родителя: игровое лобби',390,844,body))

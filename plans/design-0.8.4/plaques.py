from kit import *
M={'048':'/_blob/6f0d8e28d05abc92c2afa2721e7ea7f2','031':'/_blob/1008f104f1664cfe12bcbac428bb9b0b','028':'/_blob/b1f25fc71802e23800d3ab35241debd6','033':'/_blob/c7f2d5cd4e68fdd0ecc2ce878ceacfa6'}
def plaque(st, title, text, buttons, accent):
    btns=''.join(f'<button style="{btn(k,46,14,16,"padding: 0 16px")}">{t}</button>' for k,t in buttons)
    close=f'<button aria-label="Закрыть" style="position: absolute; right: 12px; top: 12px; width: 34px; height: 34px; border-radius: 10px; border: 3px solid {INK}; background: #22388F; color: #FFFFFF; font-family: Rubik, sans-serif; font-weight: 900; font-size: 16px; line-height: 1">✕</button>'
    return f'''<div style="position: relative; width: 470px; box-sizing: border-box; {panel('#132C8A','#3A5DD6',24)}; border-color: {INK}; padding: 18px 54px 18px 16px; display: flex; gap: 14px; align-items: center">
      {close}
      <img src="{M[st]}" alt="" style="width: 108px; height: 108px; object-fit: contain; flex-shrink: 0">
      <div style="display: flex; flex-direction: column; gap: 8px">
        <div style="{outl(21,accent,3)}">{title}</div>
        <div style="font-weight: 600; font-size: 15px; line-height: 1.35; color: #DCE4FF">{text}</div>
        {f'<div style="display: flex; gap: 10px; margin-top: 4px">{btns}</div>' if buttons else ''}
      </div>
    </div>'''
grid=''.join([
 plaque('031','Ждёт разрешения','Игра «SuperGame» новая. Родитель должен решить, можно ли в неё играть.',[('gold','Попросить сейчас')],'#FFE066'),
 plaque('028','Эта программа запрещена','Родитель запретил «SuperGame». Выбери другую игру.',[],'#FFB199'),
 plaque('033','Можно играть!','Родитель разрешил «SuperGame»: это игра. Запусти её снова.',[],'#9FF0C4'),
 plaque('028','Родитель запретил','«SuperGame» запускать нельзя.',[],'#FFB199'),
 plaque('028','Сайт закрыт','example.com в списке запрещённых. Если он нужен, попроси родителей.',[],'#FFB199'),
 plaque('028','Сайт закрыт','Этот сайт для взрослых.',[],'#FFB199'),
 plaque('048','Видео на сегодня всё','Время на видео и соцсети закончилось. Общего времени осталось 40 мин.',[],'#FFB199'),
 plaque('028','Этот браузер запрещён','Пользуйся Brave.',[],'#FFB199'),
])
body=f'''<div style="width: 1040px; height: 900px; box-sizing: border-box; padding: 26px 30px; background: #2B3138; font-family: Rubik, system-ui, sans-serif; color: #FFFFFF; display: flex; flex-direction: column; gap: 18px">
  <div style="font-weight: 800; font-size: 20px; color: #DCE4FF">Плашки ребёнку в правом нижнем углу, стикер движется. Звук Windows: «Уведомление», когда разрешили; «Восклицание», когда ждёт или запрещено</div>
  <div style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px 40px">{grid}</div>
</div>'''
open('../project/Plaques.dc.html','w',encoding='utf-8').write(page('Плашки про программы и сайты',1040,900,body))

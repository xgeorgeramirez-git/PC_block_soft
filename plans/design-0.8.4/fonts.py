from kit2 import *
V=[('1','Onest','Onest:wght@400;500;600;700;800','','',''),
   ('2','Manrope','Manrope:wght@400;500;600;700;800','','',''),
   ('3','Montserrat','Montserrat:wght@400;500;600;700;800','','',''),
   ('4','Inter','Inter:wght@400;500;600;700;800','','','')]
links=''.join(f'<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={q}&amp;display=swap">' for _,_,q,_,_,_ in V)
def card(n,f,q,tc,bc,cname):
    ff=f"font-family: '{f}', system-ui, sans-serif"
    return f'''<div style="flex: 1; display: flex; flex-direction: column; gap: 10px; {ff}">
      <div style="font-weight: 700; font-size: 18px; color: {INK}">Шрифт {n}: {f}</div>
      <div style="background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: 16px; display: flex; flex-direction: column; gap: 12px">
        <div style="display: flex; align-items: center; gap: 10px">{avatar('М','#FFE7AD')}<div style="flex-grow: 1; display: flex; flex-direction: column"><span style="font-weight: 700; font-size: 18px; color: {INK}">Миша</span><span style="font-weight: 500; font-size: 13px; color: {MUTED}">За компьютером с 18:20</span></div><div style="display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: 14px; color: {INK}; {NUM}">{coin()}45 мин</div></div>
        <div style="display: flex; align-items: baseline; gap: 8px"><span style="font-weight: 800; font-size: 40px; color: {tcol(85)[0]}; {NUM}">1:25</span><span style="font-weight: 500; font-size: 14px; color: {MUTED}">осталось из 2:00</span></div>
        {bar(71,10,tcol(85)[1])}
        <div style="display: flex; gap: 14px; font-weight: 500; font-size: 13px; color: {MUTED}; {NUM}"><span>Игры 40 из 60 мин</span><span>Видео 10 из 30 мин</span></div>
        <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{''.join(chip(x,k,40,ff) for x,k in [('+15','plus'),('+30','plus'),('+60','plus'),('Своё','plain')])}</div>
        <div style="display: flex; flex-direction: column; gap: 8px; border-top: 1px solid {LINE}; padding-top: 12px">
          <div style="display: flex; align-items: center; gap: 10px">{avatar('А','#D6E4FF',32)}<span style="flex-grow: 1; font-weight: 600; font-size: 16px; color: {INK}">Аня</span><span style="font-weight: 800; font-size: 22px; color: {tcol(40)[0]}; {NUM}">0:40</span></div>
          <div style="display: flex; align-items: center; gap: 10px">{avatar('П','#DDF3E4',32)}<span style="flex-grow: 1; font-weight: 600; font-size: 16px; color: {INK}">Петя</span><span style="font-weight: 800; font-size: 22px; color: {tcol(10)[0]}; {NUM}">0:10</span></div>
        </div>
      </div>
    </div>'''
body=f'''<div style="width: 1640px; height: 600px; box-sizing: border-box; padding: 28px 32px; background: {GROUND}; display: flex; flex-direction: column; gap: 20px; font-family: 'Golos Text', sans-serif">
  <div style="font-weight: 700; font-size: 26px; color: {INK}">Цвет остатка: больше часа зелёный, до часа синий, 15 минут и меньше красный. Выберите шрифт</div>
  <div style="display: flex; gap: 24px">{''.join(card(*v) for v in V)}</div>
</div>'''
html=page('Варианты шрифта и цвета для родителя',1640,600,body).replace('</helmet>', links+'\n</helmet>',1)
open('../project/Fonts.dc.html','w',encoding='utf-8').write(html)

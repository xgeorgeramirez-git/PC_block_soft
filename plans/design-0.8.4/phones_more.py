from kit2 import *
FF="font-family: 'Inter', system-ui, sans-serif"
def frame(title, inner, w=390, h=844):
    return f'<div style="width: {w}px; height: {h}px; box-sizing: border-box; padding: 18px 16px 0; background: {GROUND}; {FF}; color: {INK}; display: flex; flex-direction: column; gap: 14px">{inner}</div>'
def head(t, back=True):
    b = f'<a href="#" style="display: flex; align-items: center; gap: 4px; font-weight: 600; font-size: 15px; text-decoration: none"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="{BLUE}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 6l-6 6 6 6"></path></svg>Назад</a>' if back else ''
    return f'<div style="display: flex; flex-direction: column; gap: 6px">{b}<div style="font-weight: 700; font-size: 24px">{t}</div></div>'
def nav(active):
    items=[('kids','Дети'),('key','Промокоды'),('gear','Настройки')]
    s=''.join(f'<a href="#" style="display: flex; flex-direction: column; align-items: center; gap: 3px; color: {BLUE if t==active else MUTED}; text-decoration: none; font-weight: {600 if t==active else 500}; font-size: 12px">{ic(i,24,BLUE if t==active else MUTED)}{t}</a>' for i,t in items)
    return f'<div style="margin-top: auto; margin-left: -16px; margin-right: -16px; height: 72px; background: #FFFFFF; border-top: 1px solid {LINE}; display: flex; align-items: center; justify-content: space-around">{s}</div>'
def card(inner, pad=16): return f'<div style="background: #FFFFFF; border: 1px solid {LINE}; border-radius: 18px; padding: {pad}px; display: flex; flex-direction: column; gap: 12px">{inner}</div>'
def bigbtn(t, kind='blue', extra=''):
    c={'blue':('#FFFFFF',BLUE,BLUE),'plain':(INK,'#FFFFFF',LINE),'danger':(T_RED[0],'#FFFFFF','#F3C4C4')}[kind]
    return f'<button style="height: 50px; border-radius: 12px; border: 1px solid {c[2]}; background: {c[1]}; color: {c[0]}; {FF}; font-weight: 600; font-size: 16px; cursor: pointer; {extra}">{t}</button>'

# 1. В копилку
piggy = frame('В копилку', head('Копилка Миши') + card(f'''
  <div style="display: flex; align-items: center; gap: 12px">{coin(44)}<div style="display: flex; flex-direction: column"><span style="font-weight: 500; font-size: 14px; color: {MUTED}">Сейчас в копилке</span><span style="font-weight: 800; font-size: 32px; {NUM}">45 мин</span></div></div>
  <label for="pm" style="font-weight: 600; font-size: 15px">Сколько минут положить</label>
  <input id="pm" value="30" inputmode="numeric" style="height: 52px; border-radius: 12px; border: 2px solid {BLUE}; padding: 0 14px; {FF}; font-weight: 700; font-size: 22px; color: {INK}; {NUM}; box-sizing: border-box; width: 100%">
  <div style="font-weight: 500; font-size: 13px; color: {MUTED}">От 1 до 555 минут: в копилке может быть не больше 600</div>
  {bigbtn('Положить 30 минут')}
''') + card(f'''
  <div style="font-weight: 600; font-size: 16px">Обнулить копилку</div>
  <div style="font-weight: 500; font-size: 14px; color: {MUTED}">Все 45 минут пропадут. Телефон спросит ещё раз, прежде чем обнулить.</div>
  {bigbtn('Обнулить копилку','danger')}
''') + nav('Дети'))

# 2. Промокоды
codes=[('SUMMER','30 мин','создан 25 ноября'),('K7RMZP','60 мин','создан 26 ноября'),('HX4TQA','15 мин','создан 26 ноября')]
rows=''.join(f'<div style="display: flex; align-items: center; gap: 12px; padding: 12px 0; border-top: 1px solid {LINE}"><div style="flex-grow: 1; display: flex; flex-direction: column; gap: 2px"><span style="font-weight: 700; font-size: 18px; letter-spacing: 1px">{c}</span><span style="font-weight: 500; font-size: 13px; color: {MUTED}">{d}</span></div><span style="font-weight: 700; font-size: 16px; {NUM}">{m}</span><button aria-label="Удалить код {c}" style="width: 44px; height: 44px; border-radius: 10px; border: 1px solid {LINE}; background: #FFFFFF; display: flex; align-items: center; justify-content: center"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="{MUTED}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"></path></svg></button></div>' for c,m,d in codes)
promos = frame('Промокоды', head('Промокоды', False) + f'''
  <div style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); background: #E3E8F2; border-radius: 12px; padding: 4px">
    <button style="height: 40px; border-radius: 9px; border: none; background: #FFFFFF; {FF}; font-weight: 600; font-size: 15px; color: {INK}">Действующие</button>
    <button style="height: 40px; border-radius: 9px; border: none; background: transparent; {FF}; font-weight: 500; font-size: 15px; color: {MUTED}">Использованные</button>
  </div>''' + card(f'<div style="display: flex; flex-direction: column; margin: -12px 0">{rows}</div>', 16) + f'''
  <div style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px">{bigbtn('Создать код')}{bigbtn('Придумать коды','plain')}</div>
  <div style="font-weight: 500; font-size: 13px; color: {MUTED}">Ребёнок вводит код в своём окне на компьютере. Использованный код пропадает отсюда и появляется во вкладке «Использованные»: кто и когда.</div>''' + nav('Промокоды'))

# 3. Сообщение: 7 последних стикеров и статусы
st=['/_blob/c7f2d5cd4e68fdd0ecc2ce878ceacfa6','/_blob/6f0d8e28d05abc92c2afa2721e7ea7f2','/_blob/afa6aa0f60f3b695abf9da6c3b7dbe5a','/_blob/c61f39d75962f101e7939df7b55a074a','/_blob/4886f0db420d97b1162830ac3b24c03a','/_blob/1008f104f1664cfe12bcbac428bb9b0b','/_blob/d6f71c528d61fb7d9b787c68cb4b623b']
cells=f'<button style="aspect-ratio: 1; border-radius: 12px; border: 2px solid {BLUE}; background: {BLUE_BG}; {FF}; font-weight: 600; font-size: 12px; color: {BLUE}">Без стикера</button>' + ''.join(f'<button aria-label="Стикер" style="aspect-ratio: 1; border-radius: 12px; border: 1px solid {LINE}; background: #FFFFFF; padding: 4px"><img src="{u}" alt="" style="width: 100%; height: 100%; object-fit: contain"></button>' for u in st)
msgs=[('Иди ужинать!','18:40',"Прочитано",GREEN),('Сделал уроки?','17:10',"Ждёт: компьютер не в сети",'#B45309'),('Молодец!','вчера',"Доставлено",MUTED)]
mrows=''.join(f'<div style="display: flex; justify-content: space-between; align-items: center; padding: 10px 0; border-top: 1px solid {LINE}"><div style="display: flex; flex-direction: column; gap: 2px"><span style="font-weight: 600; font-size: 15px">{t}</span><span style="font-weight: 500; font-size: 13px; color: {MUTED}">{w}</span></div><span style="font-weight: 600; font-size: 13px; color: {c}; text-align: right; max-width: 150px">{s}</span></div>' for t,w,s,c in msgs)
note = frame('Сообщение', head('Сообщение для Миши') + card(f'''
  <label for="nt" style="font-weight: 600; font-size: 15px">Текст</label>
  <textarea id="nt" style="height: 76px; border-radius: 12px; border: 1px solid {LINE}; padding: 12px; {FF}; font-size: 16px; color: {INK}; resize: none; box-sizing: border-box; width: 100%">Через 10 минут ужин</textarea>
  <div style="display: flex; justify-content: space-between; align-items: center"><span style="font-weight: 600; font-size: 15px">Стикер</span><a href="#" style="font-weight: 600; font-size: 14px; text-decoration: none">Все 74</a></div>
  <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px">{cells}</div>
  <div style="font-weight: 500; font-size: 13px; color: {MUTED}">Здесь 7 последних, что вы выбирали</div>
  {bigbtn('Отправить')}
''') + card(f'<div style="font-weight: 600; font-size: 15px">Последние сообщения</div><div style="display: flex; flex-direction: column; margin-bottom: -12px">{mrows}</div>') + nav('Дети'), h=900)

if __name__=="__main__":
  for name,title,html in [('PhonePiggy','Телефон: копилка',piggy),('PhonePromos','Телефон: промокоды',promos),('PhoneNote','Телефон: сообщение',note)]:
    open(f'../project/{name}.dc.html','w',encoding='utf-8').write(page(title,390,900 if name=='PhoneNote' else 844,html))

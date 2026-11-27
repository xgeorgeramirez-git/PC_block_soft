from kit import *
S=[('/_blob/c7f2d5cd4e68fdd0ecc2ce878ceacfa6','Времени много','больше 1 часа','#8DFFC4','#22C47E',80),
   ('/_blob/afa6aa0f60f3b695abf9da6c3b7dbe5a','Половина','от 15 минут до 1 часа','#8DFFC4','#22C47E',45),
   ('/_blob/1008f104f1664cfe12bcbac428bb9b0b','Осталось мало','15 минут и меньше','#FFE066','#FFB800',12),
   ('/_blob/6f0d8e28d05abc92c2afa2721e7ea7f2','Время кончилось','до завтра','#FFC2A8','#FF6A4A',0),
   ('/_blob/c61f39d75962f101e7939df7b55a074a','Компьютер спит','запретные часы','#C9C2FF','#7B6CFF',0)]
cards=''.join(f'''<div style="flex: 1; {panel('#132C8A','#3A5DD6',24)}; padding: 14px 14px 18px; display: flex; flex-direction: column; align-items: center; gap: 8px">
      <img src="{u}" alt="" style="width: 150px; height: 150px; object-fit: contain">
      <div style="{outl(19)}; text-align: center">{t}</div>
      <div style="font-weight: 700; font-size: 14px; color: #C7D4FF; text-align: center">{s}</div>
      <div style="width: 100%">{bar(p,20,a,b)}</div>
    </div>''' for u,t,s,a,b,p in S)
body=f'''<div style="width: 1040px; height: 380px; box-sizing: border-box; padding: 24px 28px; {BG}; font-family: Rubik, system-ui, sans-serif; color: #FFFFFF; display: flex; flex-direction: column; gap: 16px">
  <div style="{outl(24)}">Талисман меняется по остатку времени</div>
  <div style="display: flex; gap: 14px">{cards}</div>
</div>'''
open('../project/Moods.dc.html','w',encoding='utf-8').write(page('Талисман по остатку времени',1040,380,body))

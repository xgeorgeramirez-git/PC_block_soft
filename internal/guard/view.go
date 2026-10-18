package guard

// LockView — тексты экрана блокировки (ТЗ 8.2.3). Заполняет служба из ядра,
// страж только показывает. Без обращений к Windows.
type LockView struct {
	Reason string // заголовок причины, например «Время на сегодня закончилось»
	Detail string // пояснение под заголовком (для блокировки за обход)
	Until  string // когда снова можно, например «Можно снова: завтра в 07:00»
	Used   string // сколько использовано, например «Сегодня: 2 ч 00 мин из 2 ч»
}

// SelfTestResult — результат самопроверки окна блокировки: осталось ли окно
// видимым после попыток закрыть его штатными сообщениями.
type SelfTestResult struct {
	VisibleAfterWMClose bool // окно видно после WM_CLOSE
	VisibleAfterSCClose bool // окно видно после WM_SYSCOMMAND/SC_CLOSE (Alt+F4)
	VisibleAfterEsc     bool // окно видно после WM_KEYDOWN VK_ESCAPE
	Passed              bool // все три проверки пройдены
}

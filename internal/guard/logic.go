package guard

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"pcblock/internal/core"
)

// Сроки стража. Все моменты, которые получает Model, это реальные часы (Ruling 4):
// ускорение учёта в тестовом режиме их не касается.
const (
	offlineAfter = 10 * time.Second // нет ответа службы дольше этого срока: экран снят (Ruling 15, NFR-10)
	warningLife  = 15 * time.Second // плашка предупреждения закрывается сама (FR-9)
	addedLife    = 5 * time.Second  // плашка «Добавлено N минут» (ТЗ 8.2.2 п.4)

	// autoSwitchAfter — через сколько после появления экрана блокировки без родительского
	// пароля сама выполняется «Смена пользователя» (ТЗ 8.2.3 п.10, D-40).
	autoSwitchAfter = 15 * time.Minute
)

// EvGuardOffline — событие журнала «страж был без связи со службой»; пишется после
// восстановления связи, Details["seconds"] — длительность в секундах (Ruling 15).
const EvGuardOffline = "guard_offline"

// Event — событие журнала, формат ядра (ТЗ 4, R22): момент добавляет служба.
type Event = core.Event

// Button — кнопка, которую рисует окно стража и которую можно нажать (Press).
// Нулевое значение не кнопка: нажатие ButtonNone ничего не делает.
type Button int

// Кнопки экрана блокировки, его панелей и плашки.
const (
	ButtonNone            Button = iota
	ButtonParentPassword         // «Пароль родителя» на экране
	ButtonLogoff                 // «Выйти из учётной записи» на экране
	ButtonSubmitPassword         // «Продолжить» на панели пароля (Enter)
	ButtonCancel                 // «Отмена» на любой панели
	ButtonAdd15                  // «+15»
	ButtonAdd30                  // «+30»
	ButtonAdd60                  // «+60»
	ButtonOpenSettings           // «Открыть настройки»
	ButtonClearBlock             // «Снять блокировку» (блокировка за обход)
	ButtonConfirmLogoff          // «Выйти» в подтверждении выхода
	ButtonShutdown               // «Выключить компьютер» на экране
	ButtonConfirmShutdown        // «Выключить» в подтверждении выключения
	ButtonSwitchUser             // «Смена пользователя» на экране
	ButtonWarningOK              // «Понятно» на плашке предупреждения
	ButtonAskMore                // «Попросить ещё» на экране (FR-10): открывает панель пароля
	ButtonSpendBonus             // «Взять из копилки» на экране (FR-11): без пароля
	ButtonPromo                  // «Ввести промокод» на экране (FR-35, R26): открывает поле кода
	ButtonSubmitPromo            // «Применить» на панели промокода (Enter)
)

// Panel — панель поверх экрана блокировки; одновременно открыта одна.
type Panel int

// Панели.
const (
	PanelNone            Panel = iota // видны только кнопки экрана
	PanelPassword                     // поле пароля родителя
	PanelParentChoices                // выбор действия после верного пароля
	PanelConfirmLogoff                // «Несохранённые документы закроются. Выйти?»
	PanelConfirmShutdown              // «Несохранённые документы закроются. Выключить компьютер?»
	PanelPromo                        // поле промокода (FR-35, R26): текст виден, без пароля
)

// ActionKind — что окну стража делать после нажатия: исполняет окно (T10b) через службу.
type ActionKind int

// Действия.
const (
	ActNone          ActionKind = iota // ничего, модель сама обновила вид
	ActCheckPassword                   // отправить Password службе на проверку, ответ вернуть в OnPasswordResult
	ActAddExtra                        // попросить службу добавить Minutes минут, подтверждение вернуть в OnExtraAdded
	ActClearBlock                      // попросить службу снять блокировку за обход
	ActOpenSettings                    // попросить службу открыть окно родителя с родительской сессией
	ActLogoff                          // попросить службу выйти из учётной записи (WTSLogoffSession)
	ActShutdown                        // попросить службу выключить компьютер
	ActSwitchUser                      // попросить службу отключить сеанс без завершения: экран выбора пользователя Windows
	ActSpendBonus                      // попросить службу взять Minutes минут из копилки (FR-11, без пароля)
	ActRedeemPromo                     // отправить службе промокод Code, ответ вернуть в OnPromoResult (R26)
)

// Action — результат нажатия. Пароль есть только у ActCheckPassword и только в этом
// значении: модель его не хранит.
type Action struct {
	Kind     ActionKind
	Password string
	Minutes  int
	Code     string // промокод как набран (ActRedeemPromo); нормализует служба (R26)
}

// PromoResult — ответ службы на промокод (R26): принят с Minutes минутами или отказ с текстом
// для ребёнка («Код не подходит»).
type PromoResult struct {
	OK      bool
	Minutes int
	Error   string
}

// ButtonView — кнопка для рисования.
type ButtonView struct {
	Button  Button
	Label   string
	Main    bool   // главная кнопка: графитовая; Enter нажимает её (ТЗ 8.0 п.8)
	Enabled bool   // недоступная кнопка рисуется серой и всегда с Hint
	Hint    string // почему кнопка недоступна и когда станет доступна (ТЗ 8.0 п.5, L-17)
}

// WarningView — плашка справа внизу без перехвата фокуса.
type WarningView struct {
	// ID растёт с каждой новой плашкой и не меняется, пока плашка та же (обновляется
	// только отсчёт): окно играет звук, когда ID сменился и Sound = true.
	ID int
	// Text — полный текст плашки, уже с отсчётом («Осталась 1 минута: 0:59»).
	Text string
	// Countdown — тот же отсчёт отдельно, м:сс; пусто у плашек без отсчёта.
	Countdown string
	Sound     bool         // сопровождается звуком (за 1 минуту)
	Buttons   []ButtonView // «Понятно»; у «Добавлено N минут» кнопок нет; не nil
}

// View — что показать сейчас. Окно стража только рисует его и сообщает нажатия.
// Срезы никогда не nil (в JSON не уходит null).
type View struct {
	LockShown bool         // экран блокировки показан
	Lock      LockView     // тексты экрана
	Buttons   []ButtonView // кнопки экрана; пусто, пока открыта панель

	Panel     Panel
	PanelText string // вопрос подтверждения выхода или выключения; в остальных панелях пусто
	// Choices — кнопки открытой панели сверху вниз (для PanelNone пусто): на панели пароля
	// «Продолжить» и «Отмена», после пароля выбор действия, в подтверждении «Выйти» или «Выключить» и «Отмена».
	Choices []ButtonView

	PasswordEnabled bool // поле ввода (пароль или промокод) доступно
	// FieldPlain: поле показывает набранное как есть (промокод); иначе точками (пароль).
	FieldPlain bool
	// FieldLabel — подпись над полем: «Пароль родителя» или «Промокод» (без подписи неясно,
	// что вводить, B-12).
	FieldLabel        string
	PasswordError     string // ошибка у поля («Введите пароль», «Неверный пароль…», пауза)
	PasswordCountdown string // м:сс до конца паузы, пока поле недоступно
	// PasswordClearSeq растёт каждый раз, когда поле пароля надо очистить (панель
	// открыта, ответ службы, «Отмена»): окно очищает поле при смене значения.
	PasswordClearSeq int

	Warning *WarningView // плашка предупреждения; nil, если плашки нет

	// SuppressKeys: глушить Win, Alt+Tab, Alt+Esc, Esc, Alt+F4, Ctrl+Esc, Ctrl+Shift+Esc;
	// истина только пока показан экран блокировки. Нажатия не записываются (R23).
	SuppressKeys bool
}

// PasswordResult — ответ службы на проверку пароля родителя (R19).
type PasswordResult struct {
	OK           bool
	AttemptsLeft int       // сколько неверных попыток осталось до паузы
	BlockedUntil time.Time // конец паузы после серии неверных попыток; нулевое значение: паузы нет
}

// Тексты кнопок и сообщений (ТЗ 8.2.2, 8.2.3; ребёнку на «ты», родителю на «вы», без тире).
const (
	textParentPassword   = "Пароль родителя"
	textLogoff           = "Выйти из учётной записи"
	textShutdown         = "Выключить компьютер"
	textSwitchUser       = "Смена пользователя"
	textSubmit           = "Продолжить"
	textCancel           = "Отмена"
	textOpenSettings     = "Открыть настройки"
	textClearBlock       = "Снять блокировку"
	textAskMore          = "Попросить ещё"
	textConfirmLogoff    = "Выйти"
	textConfirmShutdown  = "Выключить"
	textWarningOK        = "Понятно"
	textLogoffQuestion   = "Несохранённые документы закроются. Выйти?"
	textShutdownQuestion = "Несохранённые документы закроются. Выключить компьютер?"
	textEnterPassword    = "Введите пароль"
	textWarn30           = "Через 30 минут время на сегодня закончится"
	textWarn15           = "Через 15 минут время на сегодня закончится"
	textWarn5            = "Через 5 минут время закончится. Сохрани игру"
	textWarn1            = "Осталась 1 минута: "
	textChildAskParent   = "Попроси родителя, если нужно больше времени"
	textPromo            = "Ввести промокод"
	textSubmitPromo      = "Применить"
	textPromoLabel       = "Промокод"
	textEnterCode        = "Введи код"
)

// isExitButton — кнопка выхода экрана блокировки (не даёт доступа ко времени): выход из
// учётной записи, выключение, смена пользователя. Отделяется отступом от кнопок доступа (B-10).
func isExitButton(b Button) bool {
	return b == ButtonLogoff || b == ButtonShutdown || b == ButtonSwitchUser
}

// spendBonusLabel — «Взять из копилки (45 мин)» (ТЗ 8.2.3 п.3).
func spendBonusLabel(balance int) string {
	return "Взять из копилки (" + core.FormatLimit(balance) + ")"
}

// bonusForbiddenHint — пояснение, почему «Взять из копилки» недоступна в запретный интервал
// (L-17): «Из копилки можно взять с 07:00». until — строка «Можно снова: завтра в 07:00».
func bonusForbiddenHint(until string) string {
	if i := indexClock(until); i >= 0 {
		return "Из копилки можно взять с " + until[i:]
	}
	return "Из копилки можно взять, когда закончатся запретные часы"
}

// indexClock возвращает позицию первого «ЧЧ:ММ» в строке или -1.
func indexClock(s string) int {
	for i := 0; i+5 <= len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] >= '0' && s[i+1] <= '9' && s[i+2] == ':' &&
			s[i+3] >= '0' && s[i+3] <= '9' && s[i+4] >= '0' && s[i+4] <= '9' {
			return i
		}
	}
	return -1
}

// warnThresholds — пороги R9, по которым бывают плашки, в минутах: те же, что у ядра (D-54).
var warnThresholds = core.WarnThresholds

type plateKind int

const (
	plateThreshold plateKind = iota // предупреждение за 15/5/1 минут
	plateAdded                      // «Добавлено N минут»
)

// plate — плашка в состоянии модели.
type plate struct {
	id        int
	kind      plateKind
	threshold int // минуты порога (plateThreshold)
	minutes   int // добавленные минуты (plateAdded)
	shownAt   time.Time
}

func (p *plate) life() time.Duration {
	if p.kind == plateAdded {
		return addedLife
	}
	return warningLife
}

// Model — состояние стража без Windows: что показать по решениям службы, нажатиям и
// реальному времени. Безопасна для вызова из нескольких горутин (опрос службы и окно).
type Model struct {
	mu sync.Mutex

	hasDec bool          // хотя бы одно решение получено
	lastOK time.Time     // момент последнего успешного ответа службы
	dec    core.Decision // последнее решение
	hidden bool          // идёт родительская сессия (Ruling 19)

	wasShown  bool          // экран был показан после прошлого решения
	shownKind core.LockKind // вид блокировки, которая показана

	// autoDue — когда выполнить автоматическую смену пользователя. Имеет смысл, пока экран
	// показан (wasShown): с новым появлением экрана срок ставится заново. Двигают его только
	// появление экрана, верный пароль родителя и сама смена (на switchAfter вперёд); действия
	// ребёнка не двигают.
	autoDue   time.Time
	autoAfter time.Duration // срок автосмены, заданный тестовым режимом; 0: autoSwitchAfter

	panel        Panel
	pwdErr       string    // ошибка у поля, кроме паузы
	promoErr     string    // ошибка у поля промокода («Код не подходит», «Введи код»)
	blockedUntil time.Time // конец паузы пароля; нулевое значение: паузы нет
	clearSeq     int

	warn     *plate
	plateSeq int

	events []Event
}

// NewModel создаёт модель без решений: пока служба не ответила, ничего не показывается.
func NewModel() *Model {
	return &Model{events: []Event{}}
}

// offlineAt: успешного ответа службы нет дольше offlineAfter. Ровно offlineAfter ещё
// не обрыв. Чистая функция времени, без состояния: обрыв виден и без Tick.
func (m *Model) offlineAt(now time.Time) bool {
	return m.hasDec && now.Sub(m.lastOK) > offlineAfter
}

// lockActive: по последнему решению экран блокировки положен (связь не учитывается).
func (m *Model) lockActive() bool {
	return m.hasDec && !m.hidden && m.dec.Lock != core.LockNone
}

// screenShown: экран блокировки виден сейчас.
func (m *Model) screenShown(now time.Time) bool {
	return m.lockActive() && !m.offlineAt(now)
}

// paused: пароль в паузе (R19).
func (m *Model) paused(now time.Time) bool {
	return now.Before(m.blockedUntil)
}

// resetPanels закрывает панели и убирает ошибку поля. Паузу пароля не трогает:
// её назначила служба, она действует, пока не истечёт.
func (m *Model) resetPanels() {
	m.panel = PanelNone
	m.pwdErr = ""
	m.promoErr = ""
}

// resetUI сбрасывает всё, что относится к показанному: панели, паузу, плашку. Вызывается,
// когда экран и плашки скрыты (обрыв связи, родительская сессия): вернуться они должны
// по свежему решению, а не по старому состоянию.
func (m *Model) resetUI() {
	m.resetPanels()
	m.blockedUntil = time.Time{}
	m.warn = nil
	m.wasShown = false
}

// switchAfter возвращает срок автосмены: заданный тестовым режимом или 15 минут.
func (m *Model) switchAfter() time.Duration {
	if m.autoAfter > 0 {
		return m.autoAfter
	}
	return autoSwitchAfter
}

// SetAutoSwitchAfter сокращает срок автосмены пользователя (только тестовый режим: приёмка не
// ждёт 15 минут настоящих часов). Ноль, отрицательное значение и срок длиннее 15 минут не
// принимаются: срок остаётся обычным.
func (m *Model) SetAutoSwitchAfter(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d <= 0 || d > autoSwitchAfter {
		d = 0
	}
	m.autoAfter = d
}

// advance приводит состояние к моменту now: обрыв связи, срок плашки, конец паузы.
// Вызывается каждым изменяющим методом; View его не вызывает и состояния не меняет.
func (m *Model) advance(now time.Time) {
	if m.offlineAt(now) {
		m.resetUI()
		return
	}
	if m.warn != nil && !now.Before(m.warn.shownAt.Add(m.warn.life())) {
		m.warn = nil
	}
	if !m.blockedUntil.IsZero() && !now.Before(m.blockedUntil) {
		m.blockedUntil = time.Time{}
		m.pwdErr = ""
	}
}

// OnDecision принимает успешный ответ службы: решение, признак родительской сессии
// (hideForParent, Ruling 19) и пороги, о которых служба сообщила впервые.
func (m *Model) OnDecision(now time.Time, d core.Decision, hideForParent bool, newWarnings []int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if m.hasDec && now.Sub(m.lastOK) > offlineAfter {
		secs := int((now.Sub(m.lastOK) + time.Second/2) / time.Second)
		m.events = append(m.events, Event{Kind: EvGuardOffline, Details: map[string]any{
			"seconds": secs,
			"text":    fmt.Sprintf("Страж был без связи со службой %d с", secs),
		}})
	}
	m.hasDec, m.lastOK, m.dec, m.hidden = true, now, d, hideForParent

	shown := d.Lock != core.LockNone && !hideForParent
	switch {
	case hideForParent:
		m.resetUI()
	case !shown:
		m.resetPanels()
	case !m.wasShown || m.shownKind != d.Lock:
		// Экран появился или сменился его вид: панели начинаются заново, плашек поверх нет.
		m.resetPanels()
		m.warn = nil
	}
	if shown && !m.wasShown {
		m.autoDue = now.Add(m.switchAfter()) // экран появился: отсчёт идёт, пока он показан
	}
	m.wasShown, m.shownKind = shown, d.Lock
	if shown || hideForParent {
		return
	}

	if m.warn != nil && m.stale() {
		m.warn = nil
	}
	if thr := mostUrgent(newWarnings); thr != 0 {
		m.newPlate(now, plateThreshold, thr, 0)
	}
}

// mostUrgent возвращает меньший из известных порогов (одна причина: одно уведомление,
// L-118); 0, если известных нет.
func mostUrgent(newWarnings []int) int {
	best := 0
	for _, w := range newWarnings {
		for _, known := range warnThresholds {
			if w == known && (best == 0 || w < best) {
				best = w
			}
		}
	}
	return best
}

// stale: событие, о котором предупреждала плашка, отодвинулось (доп. время, новые
// правила) или лимита больше нет: плашка устарела (R9: пороги взводятся заново).
func (m *Model) stale() bool {
	if m.warn.kind != plateThreshold {
		return false
	}
	return m.dec.Unlimited || m.dec.RemainingS > m.warn.threshold*60
}

func (m *Model) newPlate(now time.Time, kind plateKind, threshold, minutes int) {
	m.plateSeq++
	m.warn = &plate{id: m.plateSeq, kind: kind, threshold: threshold, minutes: minutes, shownAt: now}
}

// OnServiceUnreachable сообщает, что службы нет: сам по себе вид не меняет, но подводит
// состояние к моменту now. Отсчёт 10 с идёт от последнего успешного ответа.
func (m *Model) OnServiceUnreachable(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
}

// Tick подводит состояние к реальному моменту now: автозакрытие плашек, конец паузы
// пароля, обрыв связи.
func (m *Model) Tick(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
}

// OnPasswordResult принимает ответ службы на проверку пароля. Верный ответ открывает выбор
// действий, неверный показывает счётчик попыток, пауза закрывает поле с отсчётом. Если
// панель пароля уже закрыта («Отмена»), верный и неверный ответы игнорируются; пауза
// запоминается всегда.
func (m *Model) OnPasswordResult(now time.Time, r PasswordResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if !m.screenShown(now) {
		return
	}
	if !r.OK && r.BlockedUntil.After(now) {
		m.blockedUntil = r.BlockedUntil
		m.pwdErr = ""
		m.clearSeq++
		return
	}
	if m.panel != PanelPassword {
		return
	}
	m.clearSeq++
	if r.OK {
		m.panel = PanelParentChoices
		m.pwdErr = ""
		m.blockedUntil = time.Time{}
		m.autoDue = now.Add(m.switchAfter()) // родитель у экрана: ему даётся ещё срок на выбор
		return
	}
	left := r.AttemptsLeft
	if left < 0 {
		left = 0
	}
	m.pwdErr = fmt.Sprintf("Неверный пароль. Осталось попыток: %d", left)
}

// OnExtraAdded: служба подтвердила доп. время. Показывается плашка «Добавлено N минут»
// на 5 с; она заменяет видимую плашку предупреждения, а после доп. времени с экрана
// блокировки появляется, когда экран снят. В родительской сессии и без связи плашки нет.
func (m *Model) OnExtraAdded(now time.Time, minutes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if !m.hasDec || m.hidden || m.offlineAt(now) || minutes <= 0 {
		return
	}
	m.resetPanels()
	m.newPlate(now, plateAdded, 0, minutes)
}

// OnPromoResult принимает ответ службы на промокод (R26). Принят: панель закрывается и, как
// после доп. времени, готовится плашка «Добавлено N минут» (видна, когда экран снят). Отказ:
// текст службы у поля, панель остаётся, набранный код не стирается (можно поправить опечатку).
// Если панель промокода уже закрыта («Отмена»), отказ игнорируется.
func (m *Model) OnPromoResult(now time.Time, r PromoResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if r.OK {
		if !m.hasDec || m.hidden || m.offlineAt(now) || r.Minutes <= 0 {
			return
		}
		m.resetPanels()
		m.clearSeq++
		m.newPlate(now, plateAdded, 0, r.Minutes)
		return
	}
	if m.panel != PanelPromo || !m.screenShown(now) {
		return
	}
	m.promoErr = r.Error
	if m.promoErr == "" {
		m.promoErr = "Код не подходит"
	}
}

// TakeEvents возвращает накопленные события журнала и очищает очередь; пустой результат
// это [], не nil.
func (m *Model) TakeEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.events
	m.events = []Event{}
	return out
}

// AutoSwitch проверяет срок автоматической смены пользователя в момент now (ТЗ 8.2.3 п.10).
// Экран показан, пароль родителя за срок не введён: возвращает ActSwitchUser, закрывает панели
// (в оставленном сеансе не должно висеть набранного пароля) и переносит срок на autoAfter
// вперёд: ровно одно действие за срок, а если ребёнок вернулся к тому же экрану, через
// 15 минут будет следующая смена. Нет экрана (нет связи, родительская сессия, блокировка снята):
// ничего. Состояние меняется только здесь и в OnDecision, нажатия ребёнка срок не двигают.
func (m *Model) AutoSwitch(now time.Time) Action {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if !m.screenShown(now) || m.autoDue.IsZero() || now.Before(m.autoDue) {
		return Action{}
	}
	m.autoDue = now.Add(m.switchAfter())
	if m.panel != PanelNone {
		m.resetPanels()
		m.clearSeq++
	}
	return Action{Kind: ActSwitchUser}
}

// Press принимает нажатие кнопки b в момент now; password нужен только для
// ButtonSubmitPassword. Кнопка, которой сейчас нет на экране, ничего не делает.
func (m *Model) Press(now time.Time, b Button, password string) Action {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	if b == ButtonWarningOK {
		if m.plateVisible(now) {
			m.warn = nil
		}
		return Action{}
	}
	if !m.screenShown(now) {
		return Action{}
	}
	switch m.panel {
	case PanelPassword:
		return m.pressPassword(now, b, password)
	case PanelParentChoices:
		return m.pressChoices(b)
	case PanelConfirmLogoff:
		return m.pressConfirmLogoff(b)
	case PanelConfirmShutdown:
		return m.pressConfirmShutdown(b)
	case PanelPromo:
		return m.pressPromo(b, password)
	default:
		return m.pressScreen(b)
	}
}

// pressPromo: панель промокода. «Применить» отправляет код службе (без пароля родителя,
// R26); пустой код — подсказка у поля. code — то, что набрано в поле.
func (m *Model) pressPromo(b Button, code string) Action {
	switch b {
	case ButtonCancel:
		m.panel = PanelNone
		m.promoErr = ""
		m.clearSeq++
	case ButtonSubmitPromo:
		if strings.TrimSpace(code) == "" {
			m.promoErr = textEnterCode
			return Action{}
		}
		m.promoErr = ""
		return Action{Kind: ActRedeemPromo, Code: code}
	}
	return Action{}
}

func (m *Model) pressScreen(b Button) Action {
	switch b {
	case ButtonParentPassword, ButtonAskMore:
		// «Попросить ещё» и «Пароль родителя» открывают ввод пароля: после верного пароля
		// родитель выбирает доп. время (FR-10). «Попросить ещё» на экране блокировки по
		// ТЗ 8.2.3 п.4 ведёт к той же панели пароля с выбором времени.
		m.panel = PanelPassword
		m.pwdErr = ""
		m.clearSeq++
	case ButtonSpendBonus:
		// В запретный интервал кнопка недоступна (view её гасит); трата без пароля (FR-11).
		if m.dec.Lock == core.LockForbidden || m.dec.BonusBalance <= 0 {
			return Action{}
		}
		return Action{Kind: ActSpendBonus, Minutes: m.dec.BonusBalance}
	case ButtonPromo:
		// Промокод доступен и в запретный интервал: время добавится, но экран останется до
		// конца интервала (ТЗ 8.2.3 п.3, R26). При обходе кнопки нет.
		if m.dec.Lock == core.LockBypass {
			return Action{}
		}
		m.panel = PanelPromo
		m.promoErr = ""
		m.clearSeq++
	case ButtonLogoff:
		m.panel = PanelConfirmLogoff
	case ButtonShutdown:
		m.panel = PanelConfirmShutdown
	case ButtonSwitchUser:
		// Без подтверждения: сеанс не завершается, данные сохраняются (ТЗ 8.2.3 п.9).
		return Action{Kind: ActSwitchUser}
	}
	return Action{}
}

func (m *Model) pressPassword(now time.Time, b Button, password string) Action {
	switch b {
	case ButtonCancel:
		m.panel = PanelNone
		m.pwdErr = ""
		m.clearSeq++
	case ButtonSubmitPassword:
		if m.paused(now) {
			return Action{}
		}
		if password == "" {
			m.pwdErr = textEnterPassword
			return Action{}
		}
		m.pwdErr = ""
		return Action{Kind: ActCheckPassword, Password: password}
	}
	return Action{}
}

// pressChoices: после верного пароля. Выбор доп. времени и снятие блокировки закрывают
// панель: повторное нажатие не добавит время дважды, а при ошибке службы родитель
// снова на экране с кнопками.
func (m *Model) pressChoices(b Button) Action {
	bypass := m.dec.Lock == core.LockBypass
	switch b {
	case ButtonCancel:
		m.panel = PanelNone
	case ButtonAdd15, ButtonAdd30, ButtonAdd60:
		if bypass {
			return Action{}
		}
		m.panel = PanelNone
		return Action{Kind: ActAddExtra, Minutes: map[Button]int{ButtonAdd15: 15, ButtonAdd30: 30, ButtonAdd60: 60}[b]}
	case ButtonClearBlock:
		if !bypass {
			return Action{}
		}
		m.panel = PanelNone
		return Action{Kind: ActClearBlock}
	case ButtonOpenSettings:
		return Action{Kind: ActOpenSettings}
	}
	return Action{}
}

func (m *Model) pressConfirmLogoff(b Button) Action {
	switch b {
	case ButtonCancel:
		m.panel = PanelNone
	case ButtonConfirmLogoff:
		return Action{Kind: ActLogoff}
	}
	return Action{}
}

func (m *Model) pressConfirmShutdown(b Button) Action {
	switch b {
	case ButtonCancel:
		m.panel = PanelNone
	case ButtonConfirmShutdown:
		return Action{Kind: ActShutdown}
	}
	return Action{}
}

// plateVisible: плашка есть, не вышел её срок, не устарела и сейчас нет ни экрана
// блокировки, ни родительской сессии, ни обрыва связи.
func (m *Model) plateVisible(now time.Time) bool {
	if m.warn == nil || !m.hasDec || m.hidden || m.dec.Lock != core.LockNone || m.offlineAt(now) {
		return false
	}
	if !now.Before(m.warn.shownAt.Add(m.warn.life())) {
		return false
	}
	return !m.stale()
}

// View возвращает, что показать в момент now. Состояния не меняет.
func (m *Model) View(now time.Time) View {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := View{Buttons: []ButtonView{}, Choices: []ButtonView{}}
	if !m.hasDec || m.hidden || m.offlineAt(now) {
		return v
	}
	if m.dec.Lock == core.LockNone {
		if m.plateVisible(now) {
			v.Warning = m.plateView()
		}
		return v
	}
	d := m.dec
	reason := d.Reason
	if reason == "" {
		reason = core.ReasonTimeUp
		if d.Lock == core.LockBypass {
			reason = core.ReasonBypass
		}
	}
	v.LockShown, v.SuppressKeys = true, true
	v.Lock = LockView{Reason: reason, Detail: d.Detail, Until: d.Until, Used: d.Used}
	if d.Lock != core.LockBypass {
		v.Lock.Hint = textChildAskParent
	}
	v.Panel = m.panel
	v.PasswordClearSeq = m.clearSeq
	switch m.panel {
	case PanelNone:
		v.Buttons = m.lockButtons(d)
	case PanelPassword:
		m.passwordView(now, &v)
	case PanelPromo:
		v.PasswordEnabled, v.FieldPlain, v.FieldLabel = true, true, textPromoLabel
		v.PasswordError = m.promoErr
		v.Choices = []ButtonView{
			{Button: ButtonSubmitPromo, Label: textSubmitPromo, Main: true, Enabled: true},
			{Button: ButtonCancel, Label: textCancel, Enabled: true},
		}
	case PanelParentChoices:
		v.Choices = m.choicesView()
	case PanelConfirmLogoff:
		// Главная (графитовая, по Enter) — безопасная «Отмена»: необратимый выход не должен
		// срабатывать случайным Enter (B-15). «Выйти» остаётся обычной кнопкой.
		v.PanelText = textLogoffQuestion
		v.Choices = []ButtonView{
			{Button: ButtonCancel, Label: textCancel, Main: true, Enabled: true},
			{Button: ButtonConfirmLogoff, Label: textConfirmLogoff, Enabled: true},
		}
	case PanelConfirmShutdown:
		// То же, что и для выхода: Enter отменяет, а не выключает компьютер (B-15).
		v.PanelText = textShutdownQuestion
		v.Choices = []ButtonView{
			{Button: ButtonCancel, Label: textCancel, Main: true, Enabled: true},
			{Button: ButtonConfirmShutdown, Label: textConfirmShutdown, Enabled: true},
		}
	}
	return v
}

// lockButtons — кнопки главного экрана блокировки (ТЗ 8.2.3 п.3). При обходе доступны
// только «Пароль родителя» (главная) и выходы. Иначе главная «Попросить ещё», ниже
// «Взять из копилки (N мин)» (недоступна в запретный интервал с пояснением, FR-11),
// «Пароль родителя» и выходы. Выходы доступны всегда (доступа ко времени не дают).
func (m *Model) lockButtons(d core.Decision) []ButtonView {
	exits := []ButtonView{
		{Button: ButtonShutdown, Label: textShutdown, Enabled: true},
		{Button: ButtonLogoff, Label: textLogoff, Enabled: true},
		{Button: ButtonSwitchUser, Label: textSwitchUser, Enabled: true},
	}
	if d.Lock == core.LockBypass {
		return append([]ButtonView{
			{Button: ButtonParentPassword, Label: textParentPassword, Main: true, Enabled: true},
		}, exits...)
	}
	out := []ButtonView{
		{Button: ButtonAskMore, Label: textAskMore, Main: true, Enabled: true},
	}
	if d.BonusBalance > 0 {
		bonus := ButtonView{Button: ButtonSpendBonus, Label: spendBonusLabel(d.BonusBalance), Enabled: true}
		if d.Lock == core.LockForbidden {
			bonus.Enabled = false
			bonus.Hint = bonusForbiddenHint(d.Until)
		}
		out = append(out, bonus)
	}
	out = append(out, ButtonView{Button: ButtonPromo, Label: textPromo, Enabled: true})
	out = append(out, ButtonView{Button: ButtonParentPassword, Label: textParentPassword, Enabled: true})
	return append(out, exits...)
}

func (m *Model) passwordView(now time.Time, v *View) {
	v.FieldLabel = textParentPassword
	submit := ButtonView{Button: ButtonSubmitPassword, Label: textSubmit, Main: true, Enabled: true}
	if m.paused(now) {
		secs := secondsCeil(m.blockedUntil.Sub(now))
		// Структурный отсчёт м:сс оставляем как есть; в словах для ребёнка пишем единицы,
		// чтобы «0:59» не читалось как минуты (B-12, H9).
		v.PasswordCountdown = clock(secs)
		msg := "Повторить можно через " + clockWords(secs)
		v.PasswordError = "Слишком много попыток. " + msg
		submit.Enabled = false
		submit.Hint = msg
	} else {
		v.PasswordEnabled = true
		v.PasswordError = m.pwdErr
	}
	v.Choices = []ButtonView{submit, {Button: ButtonCancel, Label: textCancel, Enabled: true}}
}

func (m *Model) choicesView() []ButtonView {
	settings := ButtonView{Button: ButtonOpenSettings, Label: textOpenSettings, Enabled: true}
	cancel := ButtonView{Button: ButtonCancel, Label: textCancel, Enabled: true}
	if m.dec.Lock == core.LockBypass {
		return []ButtonView{{Button: ButtonClearBlock, Label: textClearBlock, Main: true, Enabled: true}, settings, cancel}
	}
	return []ButtonView{
		{Button: ButtonAdd15, Label: "+15", Enabled: true},
		{Button: ButtonAdd30, Label: "+30", Enabled: true},
		{Button: ButtonAdd60, Label: "+60", Enabled: true},
		settings, cancel,
	}
}

func (m *Model) plateView() *WarningView {
	p := m.warn
	w := &WarningView{ID: p.id, Buttons: []ButtonView{}}
	if p.kind == plateAdded {
		w.Text = addedText(p.minutes)
		return w
	}
	w.Buttons = []ButtonView{{Button: ButtonWarningOK, Label: textWarningOK, Enabled: true}}
	switch p.threshold {
	case 30:
		w.Text = textWarn30
	case 15:
		w.Text = textWarn15
	case 5:
		w.Text = textWarn5
	default:
		w.Countdown = clock(m.dec.RemainingS)
		w.Text = textWarn1 + w.Countdown
		w.Sound = true
	}
	return w
}

// clock форматирует секунды как м:сс; отрицательные считаются нулём.
func clock(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// clockWords — тот же отсчёт словами с единицами: «59 сек», «1 мин», «2 мин 05 сек». Нужен там,
// где м:сс без единиц читается неоднозначно (пауза пароля, B-12). Отрицательное — ноль.
func clockWords(secs int) string {
	if secs < 0 {
		secs = 0
	}
	m, s := secs/60, secs%60
	switch {
	case m == 0:
		return fmt.Sprintf("%d сек", s)
	case s == 0:
		return fmt.Sprintf("%d мин", m)
	default:
		return fmt.Sprintf("%d мин %02d сек", m, s)
	}
}

// secondsCeil округляет длительность вверх до целых секунд: пауза в 59,9 с показывает
// «1:00», а не «0:59».
func secondsCeil(d time.Duration) int {
	return int((d + time.Second - 1) / time.Second)
}

// minutesWord склоняет «минута» по числу n: 1 минута, 2 минуты, 5 минут, 11 минут,
// 21 минута (L-36).
func minutesWord(n int) string {
	if n%100 >= 11 && n%100 <= 14 {
		return "минут"
	}
	switch n % 10 {
	case 1:
		return "минута"
	case 2, 3, 4:
		return "минуты"
	}
	return "минут"
}

// addedText — «Добавлено 30 минут»; со словом «минута» глагол согласуется: «Добавлена
// 1 минута», «Добавлена 21 минута».
func addedText(n int) string {
	verb := "Добавлено"
	if n%10 == 1 && n%100 != 11 {
		verb = "Добавлена"
	}
	return fmt.Sprintf("%s %d %s", verb, n, minutesWord(n))
}

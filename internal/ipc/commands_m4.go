package ipc

import "encoding/json"

// Контракт вехи M4 «Сайты и Brave» (R6, R14, R15, FR-16, FR-21, RM4-1…RM4-12). Зеркалится в
// frontend/src/types.ts и проверяется contract_test.go + contract.test.ts. Обработчики команд
// сайтов реализует служба (internal/service).

// Команды M4.
const (
	CmdListSites        = "ListSites"        // «Сайты»: списки и переключатели 18+ (родитель, FR-21)
	CmdAddSite          = "AddSite"          // добавить или перенести домен (родитель)
	CmdRemoveSite       = "RemoveSite"       // удалить домен; «Отменить» — AddSite (родитель)
	CmdSetAdultFilter   = "SetAdultFilter"   // «Блокировать сайты 18+» ребёнку (родитель)
	CmdTestSeedPrograms = "TestSeedPrograms" // тестовый режим: программы «до обновления» (RM4-11, S46)
)

// FieldDomain — поле ввода домена на экране «Сайты»: к нему привязаны отказы AddSite.
const FieldDomain = "domain"

// Списки сайтов (как site_rules.kind и sites.Kind.String()).
const (
	SiteKindBlocked = "blocked"
	SiteKindStudy   = "study"
	SiteKindVideo   = "video_social"
)

// MaxBlockedSites — наибольшее число доменов в «Запрещённых»: «N из 1000» на экране и предел
// URLBlocklist правил Brave (R14, RM4-10).
const MaxBlockedSites = 1000

// MaxDomainLen — наибольшая длина домена (как sites.MaxDomainLen): SensorSample.ForegroundDomain
// длиннее служба отбрасывает (RM4-4).
const MaxDomainLen = 253

// DefaultVideoSites — стартовый набор «Видео и соцсетей» (R14), тот же, что кладёт миграция 009:
// для шага мастера «Сайты» и примера контракта.
var DefaultVideoSites = []string{
	"youtube.com", "vk.com", "tiktok.com", "rutube.ru", "ok.ru", "telegram.org", "web.telegram.org", "discord.com", "twitch.tv",
}

// GuardCmdCloseTab — команда стражу закрыть активную вкладку Brave с доменом GuardCommand.Domain
// (RM4-2): страж перед действием перечитывает передний план и домен.
const GuardCmdCloseTab = "closeTab"

// GuardCmdCloseBraveWindow — команда стражу закрыть окно Brave, у которого адрес не читается
// (SensorSample.DomainUnread, RM4-1): страж перечитывает передний план и, только если это то же
// окно Brave и адрес всё ещё не читается, шлёт ему WM_CLOSE; иначе команду отбрасывает. Это не
// closeTab с пустым доменом: пустой домен — ещё и служебная страница, которую закрывать нельзя.
const GuardCmdCloseBraveWindow = "closeBraveWindow"

// Виды плашек о закрытых сайтах и браузерах (BlockedPlate.Kind, RM4-9).
const (
	BlockedSite      = "site"      // «Сайт X закрыт: он в списке запрещённых. Если он нужен, попроси родителей»
	BlockedAdult     = "adult"     // сайт из списка «18+» (R14)
	BlockedVideoOver = "videoOver" // время на видео и соцсети закончилось (R6)
	BlockedBrowser   = "browser"   // «Этот браузер запрещён. Пользуйся Brave» (R15)
)

// WarnEventVideo — вид события предупреждения GuardWarning.Event: исчерпание лимита «Видео и
// соцсети» (R6). Страж считает плитки по остатку видео службы, как у игр.
const WarnEventVideo = "video"

// TestDomain — действие TestGuard: страж подменяет домен вкладки Brave текстом Text во всех
// следующих снимках; пустой Text возвращает настоящее чтение (RM4-12).
const TestDomain = "domain"

// Как страж закрыл вкладку (GuardStateReply.ClosedTabWay, лестница RM4-2).
const (
	CloseTabCtrlW   = "ctrlW"   // Ctrl+W
	CloseTabInvoke  = "invoke"  // UIA-кнопка закрытия вкладки
	CloseTabWMClose = "wmClose" // WM_CLOSE окну Brave
)

// --- окно родителя: «Сайты» (FR-21) -------------------------------------------------------

// SiteDTO — домен в одном из списков (SiteKind*).
type SiteDTO struct {
	Domain string `json:"domain"`
	Kind   string `json:"kind"`
}

// AdultFilterDTO — переключатель «Блокировать сайты 18+» одного ребёнка (R14).
type AdultFilterDTO struct {
	ChildID int64  `json:"childId"`
	Name    string `json:"name"`
	On      bool   `json:"on"`
}

// ListSitesReply — экран «Сайты»: три списка, переключатели детей, предел «Запрещённых» и подпись
// источника списка «18+» (8.3.5 п.3).
type ListSitesReply struct {
	Sites       []SiteDTO        `json:"sites"`       // по списку и домену
	Children    []AdultFilterDTO `json:"children"`    // переключатели «18+»
	BlockedMax  int              `json:"blockedMax"`  // MaxBlockedSites
	AdultSource string           `json:"adultSource"` // 8.3.5 п.3 дословно
}

// AddSiteRequest — добавить домен в список Kind. Domain — ввод родителя как есть (домен или
// вставленный адрес): служба берёт из него только домен.
type AddSiteRequest struct {
	Domain string `json:"domain"`
	Kind   string `json:"kind"`
}

// AddSiteReply — ответ AddSite: домен после нормализации; Moved — домен был в другом списке и
// перенесён (один домен — один список, RM4-5).
type AddSiteReply struct {
	Message string         `json:"message"`
	Domain  string         `json:"domain"`
	Moved   bool           `json:"moved"`
	Sites   ListSitesReply `json:"sites"`
}

// RemoveSiteRequest — удалить домен из своего списка.
type RemoveSiteRequest struct {
	Domain string `json:"domain"`
}

// RemoveSiteReply — ответ RemoveSite: какой домен и из какого списка удалён; «Отменить» до
// UndoUntilMs — это AddSite(Domain, Kind).
type RemoveSiteReply struct {
	Message     string         `json:"message"`
	Domain      string         `json:"domain"`
	Kind        string         `json:"kind"`
	UndoUntilMs int64          `json:"undoUntilMs"`
	Sites       ListSitesReply `json:"sites"`
}

// SetAdultFilterRequest — включить или выключить «Блокировать сайты 18+» ребёнку.
type SetAdultFilterRequest struct {
	ChildID int64 `json:"childId"`
	On      bool  `json:"on"`
}

// SetAdultFilterReply — ответ SetAdultFilter.
type SetAdultFilterReply struct {
	Message string         `json:"message"`
	Sites   ListSitesReply `json:"sites"`
}

// SetupSites — шаг мастера «Сайты» (SetupRequest.Sites). AdultOff — SID детей с выключенным
// «18+» (отрицание: пусто — включено всем); Video — полный список «Видео и соцсетей» после правок
// родителя (служба заменяет им список).
type SetupSites struct {
	AdultOff []string `json:"adultOff"`
	Video    []string `json:"video"`
}

// --- тестовый режим: приложения магазина «до обновления» (RM4-11, S46) ----------------------

// TestSeedProgramsRequest — записать программы, учёт и «Ждут решения» в обход наблюдателя (только
// тестовый режим): повторы приложения магазина с разными версиями пакета, как в базе 0.3.3.
type TestSeedProgramsRequest struct {
	Programs []TestSeedProgram      `json:"programs"`
	Usage    []TestSeedProgramUsage `json:"usage"`
	Pending  []TestSeedPending      `json:"pending"`
}

// TestSeedProgram — строка programs.
type TestSeedProgram struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Publisher  string `json:"publisher"`
	Category   string `json:"category"` // ProgramCat*
	Source     string `json:"source"`   // ProgramSource*
	Hidden     bool   `json:"hidden"`
	CustomName string `json:"customName"`
	DecidedAt  int64  `json:"decidedAt"` // UTC мс решения родителя; 0 — не решал
}

// TestSeedProgramUsage — строка учёта программы за день (usage_daily).
type TestSeedProgramUsage struct {
	ChildID  int64  `json:"childId"`
	Day      string `json:"day"` // YYYY-MM-DD
	Path     string `json:"path"`
	Category string `json:"category"` // ProgramCat*
	Seconds  int    `json:"seconds"`
}

// TestSeedPending — строка «Ждут решения» (попытки ребёнка запустить программу).
type TestSeedPending struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	Publisher     string `json:"publisher"`
	ChildID       int64  `json:"childId"`
	Attempts      int    `json:"attempts"`
	LastAttemptAt int64  `json:"lastAttemptAt"` // UTC мс
}

// UnmarshalJSON читает день лимитов: без ключа videoUnlimited — видео без отдельного лимита (S-3).
// Иначе окно до экрана видео, мастер и PowerShell-приёмка, которые полей видео не присылают,
// закрыли бы видео на каждый сохранённый день.
func (w *WeekdayLimit) UnmarshalJSON(b []byte) error {
	type plain WeekdayLimit // без методов: без бесконечной рекурсии
	p := plain{VideoUnlimited: true}
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*w = WeekdayLimit(p)
	return nil
}

// Типы запросов и ответов программы. Зеркало internal/ipc/commands.go: имена полей, единицы
// (секунды с суффиксом S, лимиты в минутах, моменты UTC в мс с суффиксом Ms) и значения
// перечислений те же. Расхождение ловит contract.test.ts по internal/ipc/testdata/contract.json.

/** Ответ программы на любую команду (ipc.Reply). */
export interface Reply {
  ok: boolean;
  error?: string;
  field?: string;
  payload?: unknown;
}

export type WindowMode =
  | "window"
  | "wizard"
  | "parent"
  | "reset-password"
  | "uninstall-gate"
  | "mystats"
  | "firstrun";

export interface HelloReply {
  protocol: number;
  version: string;
  testMode: boolean;
  configured: boolean;
}

export type StatusKind =
  | "ok"
  | "low"
  | "timeUp"
  | "forbidden"
  | "bypass"
  | "unlimited"
  | "paused"
  | "safe"
  | "admin"
  | "notChild";

/** Экран блокировки ребёнка (ipc ChildStatus.Lock); "rest" - перерыв на отдых (R27): время не кончилось. */
export type LockKind = "none" | "timeUp" | "forbidden" | "bypass" | "rest";

export interface ChildStatus {
  name: string;
  status: StatusKind;
  statusText: string;
  label: string;
  tooltip: string;
  summary: string;
  day: string;
  remainingS: number;
  limitS: number;
  usedS: number;
  unlimited: boolean;
  lock: LockKind;
  reason: string;
  detail: string;
  until: string;
  pauseUntil: string;
  used: string;
  bonusBalance: number;
  forbiddenNote: string;
  /** Остаток игр, с (R5); 0 без лимита игр. */
  gamesRemainingS: number;
  /** Лимит игр дня с доп. временем, с; 0 без лимита игр. */
  gamesLimitS: number;
  gamesUnlimited: boolean;
  /** Правило ребёнка «Когда время вышло» (R8). */
  endMode: EndMode;
  /** Игры закрыты: лимит игр исчерпан или экран блокировки. */
  closeGames: boolean;
  /** Действует «Оставить только учёбу»: экрана блокировки нет, открыта только учёба. */
  studyOnly: boolean;
  /** Остаток «Видео и соцсетей», с (M4, R6); 0 без лимита видео. Необязательны: экраны видео - T9. */
  videoRemainingS?: number;
  /** Лимит видео дня с доп. временем, с; 0 без лимита видео. */
  videoLimitS?: number;
  videoUnlimited?: boolean;
  /** Видео и соцсети закрыты: лимит видео исчерпан. */
  closeVideo?: boolean;
}

export interface Account {
  sid: string;
  name: string;
  admin: boolean;
  child: boolean;
  current: boolean;
  canBeChild: boolean;
}

export interface ListAccountsReply {
  accounts: Account[];
}

/**
 * Лимит дня недели: 1 = пн … 7 = вс; 0 минут - нельзя; unlimited - без лимита. gamesMin - лимит
 * игр (R5): 0 - игры нельзя; gamesUnlimited - игры без отдельного лимита. Поля игр необязательны,
 * пока экраны лимитов их не показывают: обёртки saveLimits и setup подставляют «без отдельного лимита».
 * videoMin, videoUnlimited - лимит «Видео и соцсети» (R6) по тому же правилу; без videoUnlimited служба
 * читает день «видео без отдельного лимита» (S-3).
 */
export interface WeekdayLimit {
  weekday: number;
  totalMin: number;
  unlimited: boolean;
  gamesMin?: number;
  gamesUnlimited?: boolean;
  videoMin?: number;
  videoUnlimited?: boolean;
}

/** «Когда время вышло» (R8, FR-18): экран блокировки или «Оставить только учёбу». */
export type EndMode = "lock" | "study_only";

export interface SetupChild {
  sid: string;
  name: string;
  limits: WeekdayLimit[];
}

export interface SetupRequest {
  password: string;
  children: SetupChild[];
  /** Категории с шага «Программы» (FR-2); нет - категории по умолчанию. */
  programs?: SetupProgram[];
  /** Шаг «Сайты» (M4); нет - «18+» включён всем, «Видео и соцсети» - стартовый набор. */
  sites?: SetupSites;
}

export interface ResetPasswordRequest {
  newPassword: string;
}

export interface UninstallCheckRequest {
  password: string;
  forgot: boolean;
}

export interface UninstallCheckReply {
  allowed: boolean;
}

export interface LoginRequest {
  password: string;
}

export interface LoginReply {
  token: string;
  expiresMs: number;
  idleTimeoutS: number;
}

/** Нагрузка отказа Login и UninstallCheck. */
export interface LoginDenied {
  attemptsLeft: number;
  pauseS: number;
}

export interface SessionStateReply {
  active: boolean;
  expiresMs: number;
  remainingS: number;
}

export interface TodayCard {
  childId: number;
  sid: string;
  name: string;
  status: ChildStatus;
  extraMin: number;
  bonusBalance: number;
  canClearBlock: boolean;
  noForbiddenToday: boolean;
  /** «Игры 40 мин из 1 ч» (8.3.2 п.3); "" - отдельного лимита игр нет. */
  gamesText: string;
  /** Сколько программ ждут решения: ссылка «Ждут решения: N» (8.3.2 п.5). */
  pendingPrograms: number;
  /** «Видео 10 мин из 20 мин» (M4, R6); "" - отдельного лимита видео нет. */
  videoText?: string;
}

// Фидбек-раунд 0.2.2.

export interface SetNoForbiddenReply {
  card: TodayCard;
  message: string;
}

export interface BonusChangeReply {
  balanceAfter: number;
  message: string;
}

/** Ответ ClearStats и ClearJournal (FR-38): «Статистика Миши очищена», «Журнал Миши очищен». */
export interface ClearReply {
  message: string;
}

export interface PromoView {
  /** Код заглавными (RM5-11): новые так и хранятся, старые служба переводит при показе. */
  codeDisplay: string;
  minutes: number;
  createdAtMs: number;
  usedAtMs?: number;
  usedByChild?: number;
  usedByName?: string;
}

export interface CreatePromoReply {
  promo: PromoView;
  message: string;
}

export interface ListPromosReply {
  active: PromoView[];
  used: PromoView[];
}

/** Состав знаков «Придумать коды» (RM5-12): цифры, буквы A-Z или буквы и цифры. */
export type PromoCharset = "digits" | "letters" | "mixed";

/** Запрос «Придумать коды» (FR-35, RM5-12): 1-50 кодов, длина 4-12, минуты 1-600. */
export interface GeneratePromosRequest {
  count: number;
  length: number;
  charset: PromoCharset;
  minutes: number;
}

/** Ответ «Придумать коды»: новые коды и «Готово: 20 кодов на 15 мин». */
export interface GeneratePromosReply {
  promos: PromoView[];
  message: string;
}

/** Ответ «Очистить историю» промокодов (RM5-14): «История промокодов очищена». */
export interface ClearPromoHistoryReply {
  message: string;
}

export interface RedeemPromoReply {
  card: TodayCard;
  addedMin: number;
  message: string;
}

/** Окна, которые окно ребёнка просит у службы (internal/ipc.OpenWindowRequest.Kind). */
export type WindowKind = "parent" | "mystats" | "firstrun";

export interface OpenWindowReply {
  /** Окно запущено или уже открыто. */
  opened: boolean;
}

export interface OpenLogsReply {
  path: string;
  message: string;
}

export interface BonusRateReply {
  minutes: number;
}

export interface GetTodayReply {
  children: TodayCard[];
  notices: string[];
}

export interface ChildRequest {
  childId: number;
}

/** Что считать к перерыву (R27): «all» — всё, кроме учёбы; «games» — только игры. */
export type BreakCounts = "all" | "games";

/** Вкладка «Отдых» (FR-37, R27). message «» — стандартный текст экрана перерыва. */
export interface BreakRules {
  enabled: boolean;
  afterMin: number;
  counts: BreakCounts;
  warnMin: number;
  restMin: number;
  selfRestMin: number;
  message: string;
}

export interface ChildRules {
  childId: number;
  name: string;
  limits: WeekdayLimit[];
  endMode: EndMode;
  break: BreakRules;
}

/** endMode пусто - режим не меняется; limits пусто при endMode - меняется только режим. */
export interface SaveLimitsRequest {
  childId: number;
  limits: WeekdayLimit[];
  endMode?: EndMode;
  /** «Отдых» (FR-37); нет — не меняется. */
  break?: BreakRules;
}

export interface SaveLimitsReply {
  rules: ChildRules;
  card: TodayCard;
  message: string;
}

export interface AddExtraRequest {
  childId: number;
  minutes: number;
}

export interface AddExtraReply {
  card: TodayCard;
  message: string;
  extraId: string;
  undoUntilMs: number;
}

export interface UndoExtraRequest {
  childId: number;
  extraId: string;
}

/** Состояние переключателя «Подробные логи для диагностики» (FR-25, D-42). */
export interface DiagReply {
  verbose: boolean;
  /** Папка с логами (D-48): показывается в настройках сразу. */
  logsPath?: string;
}

export interface CardReply {
  card: TodayCard;
  message: string;
}

/** Фильтр «Что» журнала (FR-24, RM5-5): "" - «Все события»; виды групп знает служба. */
export type JournalGroup = "" | "violations" | "time" | "programs" | "settings" | "system";

/** Журнал: childId 0 - все дети; fromMs <= atMs < toMs (0 - без границы); «Показать ещё» -
 *  beforeAtMs/beforeId последней полученной записи; limit 0 - 200, не больше 1000. */
export interface ListEventsRequest {
  childId: number;
  fromMs: number;
  toMs: number;
  group: JournalGroup;
  beforeAtMs: number;
  beforeId: number;
  limit: number;
}

export interface JournalEntry {
  /** Номер записи: ключ списка и курсор «Показать ещё». */
  id: number;
  atMs: number;
  tzOffsetMin: number;
  childId: number;
  childName: string;
  kind: string;
  text: string;
}

export interface ListEventsReply {
  events: JournalEntry[];
  /** Есть записи старше последней: показать «Показать ещё». */
  more: boolean;
}

export interface ChangePasswordRequest {
  oldPassword: string;
  newPassword: string;
}

export interface ChildAdd {
  sid: string;
  name: string;
}

export interface ChildRename {
  childId: number;
  name: string;
}

export interface SetChildrenRequest {
  add: ChildAdd[];
  remove: number[];
  rename: ChildRename[];
}

export interface ChildInfo {
  childId: number;
  sid: string;
  name: string;
}

export interface SetChildrenReply {
  children: ChildInfo[];
}

/**
 * Строка «Моих правил»: день недели и лимит текстом (internal/ipc.MyRule). После семи дней -
 * общие правила с weekday 0 и пустым day: «Игры: до 1 ч в день», «Когда время вышло: …» (R21).
 */
export interface MyRule {
  /** 1 = пн … 7 = вс; 0 - правило не про день. */
  weekday: number;
  /** «пн»; "" у общих правил. */
  day: string;
  /** «2 ч», «Без лимита», «Нельзя», «Игры: до 1 ч в день». */
  text: string;
}

/** Период «Итогов» (8.2.6 п.2, D-61): календарный по местному времени, включает сегодня. */
export type StatsPeriod = "today" | "week" | "month" | "year" | "all";

/**
 * Ответ MyToday и ParentStats (internal/ipc.MyTodayReply): статус, «Сегодня» и «Мои правила» за сегодня,
 * «Итоги» за период period (D-61), только чтение.
 */
export interface MyTodayReply {
  status: ChildStatus;
  /** Строки «Сегодня». */
  today: string[];
  rules: MyRule[];
  /** За какой период «Итоги». */
  period: StatsPeriod;
  /** Общее время за период в счёт лимита, с («Использовано за неделю»). */
  usedS: number;
  /** Игры и учёба за период, с (итоги, S13). */
  gamesS: number;
  studyS: number;
  /** «Видео и соцсети» за период, с (M4, R6). */
  videoS?: number;
  /** «Прочее» за период, с: общее время без игр и «Видео и соцсетей». */
  otherS: number;
  /** «Всего» за период, с: общее время и учёба (учёба в лимит не входит, D-7). */
  allS: number;
  /** Доп. время за период, мин: от родителя, из копилки и по промокодам. */
  extraMin: number;
  /** Из доп. времени взято из копилки, мин. */
  bonusSpentMin: number;
  /**
   * Время каждой программы за период (вкладки «Итогов», D-58): самые долгие сверху, скрытых и
   * запрещённых нет (D-57, D-64).
   */
  programs: ProgramTime[];
  /**
   * Столбики «По дням» за период (M5, RM5-2, RM5-3): из того же чтения дней, что «Итоги», сумма столбиков
   * равна «Итогам». Подпись и подсказку собирает служба. У старой службы нет - api.ts даёт [].
   */
  bars: DayBar[];
  /** Разбивка столбиков: по дню («Сегодня», «Неделя», «Месяц») или по месяцу («Год», «Всё время»). */
  barsBy: BarsBy;
  /** Нарушения ребёнка за период, новые сверху, не больше 200 (тексты журнала). */
  violations: JournalEntry[];
  /** Записи копилки за период, новые сверху, не больше 200 (текст строки - lib/bonus.ts). */
  bonus: BonusEntry[];
  /** С какого дня ведётся статистика, YYYY-MM-DD (FR-38). */
  statsSinceDay: string;
  /** «Статистика ведётся с 11 ноября 2026». */
  statsSinceText: string;
}

/** Разбивка столбиков «По дням» (internal/ipc.BarsBy*). */
export type BarsBy = "day" | "month";

/**
 * Столбик «По дням» (internal/ipc.DayBar): день или месяц, только время (D-72, D-78). Высота - время в
 * счёт лимита, учёба - отдельная часть сверху (D-7).
 */
export interface DayBar {
  /** YYYY-MM-DD или YYYY-MM. */
  key: string;
  /** «пн 16», «ноя», «ноя 2025». */
  label: string;
  /** Подсказка: «вт, 17 ноября: 2 ч 20 мин, учёба 0 мин». */
  title: string;
  /** В счёт лимита, с. */
  totalS: number;
  /** Учёба, с. */
  studyS: number;
  /** Сегодняшний день или месяц, в который входит сегодня. */
  today: boolean;
}

/**
 * Строка вкладки «Итогов» (internal/ipc.ProgramTime): программа, категория учёта её секунд и
 * секунды за сегодня. Строка без пути - «Остальное время»: время без программы (рабочий стол, системные
 * программы, минуты скрытых и запрещённых до скрытия или запрета; время скрытых не засчитывается, D-63);
 * с ней сумма строк категории равна итогу категории.
 */
export interface ProgramTime {
  path: string;
  name: string;
  category: "games" | "study" | "other" | "video_social";
  seconds: number;
  /** "program" или "site" (M4); у сайта path - домен (R23); "" - «Остальное время». */
  kind?: "program" | "site" | "";
}

// --- M2: расписание, пауза, копилка ---------------------------------------

/** Запретный интервал расписания (R7): день недели 1=пн..7=вс, минуты от полуночи. */
export interface Interval {
  weekday: number;
  startMin: number;
  endMin: number;
}

export interface GetScheduleRequest {
  childId: number;
}

export interface GetScheduleReply {
  childId: number;
  intervals: Interval[];
}

export interface SaveScheduleRequest {
  childId: number;
  intervals: Interval[];
}

export interface SaveScheduleReply {
  intervals: Interval[];
  card: TodayCard;
  message: string;
}

/** Срок паузы (R11): 15 мин, 1 ч, 2 ч, до конца дня. */
export type PausePreset = "min15" | "hour1" | "hour2" | "endOfDay";

export interface PauseRequest {
  childId: number;
  preset: PausePreset;
}

export interface PauseReply {
  card: TodayCard;
  message: string;
}

export interface PauseAllRequest {
  preset: PausePreset;
}

export interface PauseAllReply {
  children: TodayCard[];
  message: string;
}

export interface GrantBonusRequest {
  childId: number;
  minutes: number;
  reason: string;
}

export interface GrantBonusReply {
  balanceAfter: number;
  message: string;
  bonusId: string;
  undoUntilMs: number;
}

export interface UndoBonusRequest {
  childId: number;
  bonusId: string;
}

export interface UndoBonusReply {
  balanceAfter: number;
  message: string;
}

/** Запись истории копилки (FR-22). */
export interface BonusEntry {
  atMs: number;
  tzOffsetMin: number;
  /** + начисление, - трата. */
  deltaMin: number;
  reason: string;
  balanceAfter: number;
}

export interface GetBonusRequest {
  childId: number;
}

export interface GetBonusReply {
  childId: number;
  balance: number;
  entries: BonusEntry[];
}

export interface CopyRulesRequest {
  fromChildId: number;
  toChildId: number;
}

export interface CopyRulesReply {
  limits: WeekdayLimit[];
  intervals: Interval[];
  endMode: EndMode;
  break: BreakRules;
  message: string;
}

export interface AskMoreRequest {
  childId: number;
  minutes: number;
  password: string;
}

export interface AskMoreReply {
  card: TodayCard;
  message: string;
}

export interface SpendBonusRequest {
  childId: number;
  minutes: number;
}

export interface SpendBonusReply {
  card: TodayCard;
  message: string;
  balanceAfter: number;
}

// --- M3: программы (FR-20, FR-2) -------------------------------------------

/** Категория программы; «system» родителю как выбор не показывается. */
export type ProgramCategory = "games" | "study" | "other" | "blocked" | "system";
export type ProgramSource = "catalog" | "snapshot" | "parent";

/** Экран окна родителя для OpenWindow (RM3-4). */
export type WindowScreen = "programs";

export interface ChildName {
  childId: number;
  name: string;
}

export interface PendingChildDTO {
  childId: number;
  name: string;
  attempts: number;
}

/** Строка «Ждут решения»: «SuperGame - Загрузки - 3 попытки - вчера 18:40». */
export interface PendingProgramDTO {
  path: string;
  name: string;
  publisher: string;
  description: string;
  attempts: number;
  /** Последняя попытка, UTC мс. */
  lastAttemptAt: number;
  /** «раньше: Игра» при смене файла; "" - нет. */
  wasCategory: string;
  byChild: PendingChildDTO[];
}

export interface ProgramDTO {
  path: string;
  /** Как программа называется везде: своё название родителя, иначе из файла (D-59). */
  name: string;
  /** Название из файла, если родитель дал своё («из файла: …»); нет или "" - своего нет. */
  fileName?: string;
  publisher: string;
  category: ProgramCategory;
  source: ProgramSource;
  /** Время всех детей за 7 дней, с; у скрытой 0. */
  weekS: number;
  /** Скрыта родителем (D-57): видна только в фильтре «Скрытые». Нет поля — не скрыта. */
  hidden?: boolean;
}

export interface ProgramsReply {
  pending: PendingProgramDTO[];
  programs: ProgramDTO[];
  children: ChildName[];
}

export interface DecideProgramRequest {
  path: string;
  category: ProgramCategory;
}

export interface SetProgramsCategoryRequest {
  paths: string[];
  category: ProgramCategory;
}

/** «Скрыть» (hidden = true) и «Показать» (false) для программ списка (D-57). */
export interface SetProgramsHiddenRequest {
  paths: string[];
  hidden: boolean;
}

/** «Переименовать» (D-59): своё название 1–60 символов, "" - снова название из файла. */
export interface RenameProgramRequest {
  path: string;
  name: string;
}

export interface DecideProgramReply {
  message: string;
  undoId: string;
  undoUntilMs: number;
  programs: ProgramsReply;
}

export interface UndoProgramReply {
  message: string;
  programs: ProgramsReply;
}

export interface ScanProgramDTO {
  path: string;
  name: string;
  publisher: string;
  suggestedCategory: ProgramCategory;
}

export interface ScanProgramsReply {
  programs: ScanProgramDTO[];
}

/** Категория программы, выбранная в мастере (SetupRequest.programs). */
export interface SetupProgram {
  path: string;
  category: ProgramCategory;
}

// --- M4: сайты (FR-21, R14) ------------------------------------------------

/** Список сайтов (internal/ipc.SiteKind*). */
export type SiteKind = "blocked" | "study" | "video_social";

/** Домен в одном из списков. */
export interface SiteDTO {
  domain: string;
  kind: SiteKind;
}

/** Переключатель «Блокировать сайты 18+» ребёнка. */
export interface AdultFilterDTO {
  childId: number;
  name: string;
  on: boolean;
}

/** Экран «Сайты»: списки, переключатели детей, предел «Запрещённых», подпись источника «18+». */
export interface ListSitesReply {
  sites: SiteDTO[];
  children: AdultFilterDTO[];
  blockedMax: number;
  adultSource: string;
}

/** Ответ AddSite: домен после нормализации; moved - перенесён из другого списка. */
export interface AddSiteReply {
  message: string;
  domain: string;
  moved: boolean;
  sites: ListSitesReply;
}

/** Ответ RemoveSite: «Отменить» до undoUntilMs - это addSite(domain, kind). */
export interface RemoveSiteReply {
  message: string;
  domain: string;
  kind: SiteKind;
  undoUntilMs: number;
  sites: ListSitesReply;
}

/**
 * Ответ AddSites (B-49): added - добавленные и перенесённые домены; rejected - строки ввода, не
 * похожие на адрес (окно оставляет их в поле с fieldError); message - итог словами.
 */
export interface AddSitesReply {
  message: string;
  added: string[];
  rejected: string[];
  fieldError: string;
  sites: ListSitesReply;
}

/** Ответ RemoveSites (B-49): «Отменить» до undoUntilMs - addSites доменов removed в их списки. */
export interface RemoveSitesReply {
  message: string;
  removed: SiteDTO[];
  undoUntilMs: number;
  sites: ListSitesReply;
}

export interface SetAdultFilterReply {
  message: string;
  sites: ListSitesReply;
}

/** Шаг мастера «Сайты»: SID детей с выключенным «18+» и полный список «Видео и соцсетей». */
export interface SetupSites {
  adultOff: string[];
  video: string[];
}

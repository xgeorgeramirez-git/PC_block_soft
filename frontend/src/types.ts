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

export type LockKind = "none" | "timeUp" | "forbidden" | "bypass";

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
 */
export interface WeekdayLimit {
  weekday: number;
  totalMin: number;
  unlimited: boolean;
  gamesMin?: number;
  gamesUnlimited?: boolean;
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

export interface PromoView {
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

export interface ChildRules {
  childId: number;
  name: string;
  limits: WeekdayLimit[];
  endMode: EndMode;
}

/** endMode пусто - режим не меняется; limits пусто при endMode - меняется только режим. */
export interface SaveLimitsRequest {
  childId: number;
  limits: WeekdayLimit[];
  endMode?: EndMode;
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

export interface ListEventsRequest {
  childId: number;
  fromMs: number;
  toMs: number;
  limit: number;
}

export interface JournalEntry {
  atMs: number;
  tzOffsetMin: number;
  childId: number;
  childName: string;
  kind: string;
  text: string;
}

export interface ListEventsReply {
  events: JournalEntry[];
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

/** Ответ MyToday: «Сегодня» и «Мои правила» текстом, только чтение (internal/ipc.MyTodayReply). */
export interface MyTodayReply {
  status: ChildStatus;
  /** Строки «Сегодня». */
  today: string[];
  rules: MyRule[];
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
  name: string;
  publisher: string;
  category: ProgramCategory;
  source: ProgramSource;
  /** Время всех детей за 7 дней, с. */
  weekS: number;
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

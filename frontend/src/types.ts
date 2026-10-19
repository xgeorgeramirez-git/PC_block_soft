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
  | "bypass"
  | "unlimited"
  | "safe"
  | "admin"
  | "notChild";

export type LockKind = "none" | "timeUp" | "bypass";

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
  used: string;
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

/** Лимит дня недели: 1 = пн … 7 = вс; 0 минут - нельзя; unlimited - без лимита. */
export interface WeekdayLimit {
  weekday: number;
  totalMin: number;
  unlimited: boolean;
}

export interface SetupChild {
  sid: string;
  name: string;
  limits: WeekdayLimit[];
}

export interface SetupRequest {
  password: string;
  children: SetupChild[];
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
  canClearBlock: boolean;
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
}

export interface SaveLimitsRequest {
  childId: number;
  limits: WeekdayLimit[];
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

/** Состояние Помощника: содержимое определяет каталог Помощника (T14), окно его не разбирает. */
export interface HelperState {
  state: Record<string, unknown>;
}

/** Строка «Моих правил»: день недели и лимит текстом (internal/ipc.MyRule). */
export interface MyRule {
  /** 1 = пн … 7 = вс. */
  weekday: number;
  /** «пн». */
  day: string;
  /** «2 ч», «Без лимита», «Нельзя». */
  text: string;
}

/** Ответ MyToday: «Сегодня» и «Мои правила» текстом, только чтение (internal/ipc.MyTodayReply). */
export interface MyTodayReply {
  status: ChildStatus;
  /** Строки «Сегодня». */
  today: string[];
  rules: MyRule[];
}

package ipc

// Контракт вехи M7 «Связь с телефоном» (FR-40, D-88…D-94, docs/07_Протокол_связи.md): блок
// «Телефон» в «Настройках» окна родителя. Зеркалится в frontend/src/types.ts и проверяется
// contract_test.go + contract.test.ts. Все команды — родительская сессия.

// Команды M7. Ответ всех шести — RemoteView (свежее состояние); GetRemote, RemotePairStart,
// RemotePairCancel и RemoteUnpair — без тела запроса.
const (
	CmdGetRemote         = "GetRemote"         // состояние связи с телефоном (родитель)
	CmdSetRemote         = "SetRemote"         // включить/выключить связь и адрес сервера (родитель)
	CmdRemotePairStart   = "RemotePairStart"   // получить код привязки (родитель)
	CmdRemotePairCancel  = "RemotePairCancel"  // отменить привязку (родитель)
	CmdRemotePairConfirm = "RemotePairConfirm" // «Совпадает»/«Не совпадает» (родитель)
	CmdRemoteUnpair      = "RemoteUnpair"      // отвязать телефон (родитель)
)

// RemoteDefaultURL — адрес сервера связи, если родитель не вводил свой (D-92).
const RemoteDefaultURL = "https://pcblock.atlas404.ru"

// Состояние связи (RemoteView.State).
const (
	RemoteStateOff        = "off"        // связь выключена: ни одного соединения (S56)
	RemoteStateConnecting = "connecting" // включена, первого удачного обмена ещё не было
	RemoteStateOnline     = "online"     // последний обмен с сервером удался
	RemoteStateOffline    = "offline"    // сервер не отвечает или отказал; связь пробует сама
)

// Ошибка связи (RemoteView.Error); "" — ошибки нет.
const (
	RemoteErrNoServer   = "no_server"   // сервер не отвечает
	RemoteErrBadAddress = "bad_address" // по адресу не сервер связи
	RemoteErrLost       = "lost"        // сервер не узнал компьютер, телефон отвязан
)

// Этап привязки (RemotePair.Stage).
const (
	PairIdle     = "idle"     // привязки нет
	PairStarting = "starting" // код запрашивается у сервера
	PairCode     = "code"     // код показан, ждём телефон
	PairCompare  = "compare"  // сверка числа на обоих экранах
	PairFailed   = "failed"   // привязка не удалась (RemotePair.Error)
)

// Ошибка привязки (RemotePair.Error); "" — ошибки нет.
const (
	PairErrExpired  = "expired"   // 10 минут прошли
	PairErrMismatch = "mismatch"  // «Не совпадает»
	PairErrAborted  = "aborted"   // нарушен порядок сообщений (протокол, раздел 4) или телефон отменил
	PairErrNoCode   = "no_code"   // сервер не ответил на запрос кода
	PairErrNoServer = "no_server" // код был показан, связь с сервером оборвалась
)

// RemoteView — состояние блока «Телефон». LastOKMs — последний удачный обмен с сервером (UTC мс;
// 0 — не было). Phone — привязанный телефон; nil — не привязан (ключ в JSON отсутствует, не null,
// L-22). Pair — этап привязки.
type RemoteView struct {
	Enabled  bool         `json:"enabled"`
	URL      string       `json:"url"`
	State    string       `json:"state"`
	LastOKMs int64        `json:"lastOkMs"`
	Error    string       `json:"error"`
	Phone    *RemotePhone `json:"phone,omitempty"`
	Pair     RemotePair   `json:"pair"`
}

// RemotePhone — привязанный телефон: имя, которое он прислал, и момент привязки (UTC мс).
type RemotePhone struct {
	Name       string `json:"name"`
	PairedAtMs int64  `json:"pairedAtMs"`
}

// RemotePair — привязка: Code — 6 цифр («482913») на этапе code; ExpiresMs — до какого момента
// действуют код и сверка (UTC мс); SAS — число сверки (6 цифр) и PhoneName на этапе compare;
// Error — причина на этапе failed.
type RemotePair struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	ExpiresMs int64  `json:"expiresMs"`
	SAS       string `json:"sas"`
	PhoneName string `json:"phoneName"`
	Error     string `json:"error"`
}

// SetRemoteRequest — выключатель «Связь с телефоном» и «Адрес сервера».
type SetRemoteRequest struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// RemotePairConfirmRequest — «Совпадает» (Match = true) или «Не совпадает».
type RemotePairConfirmRequest struct {
	Match bool `json:"match"`
}

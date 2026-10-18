package ipc

import "fmt"

// Peer — кто подключился к каналу. Служба получает его от транспорта вместе с каждым
// запросом и по нему решает, можно ли выполнить команду (NFR-9, Review Focus 7); сам канал
// права не проверяет.
type Peer struct {
	PID       uint32 // процесс клиента на момент подключения
	SessionID uint32 // сеанс Windows этого процесса
	SID       string // учётка, от имени которой работает клиент, например «S-1-5-21-…»
	Elevated  bool   // токен клиента повышен (UAC)
	Admin     bool   // учётка входит в Администраторы, в том числе без повышения
	System    bool   // клиент работает от имени системы (S-1-5-18)
}

// String возвращает короткое описание для журнала работы: без паролей и содержимого запросов.
func (p Peer) String() string {
	s := fmt.Sprintf("pid=%d session=%d sid=%s", p.PID, p.SessionID, p.SID)
	switch {
	case p.System:
		s += " system"
	case p.Admin && p.Elevated:
		s += " admin+elevated"
	case p.Admin:
		s += " admin"
	}
	return s
}

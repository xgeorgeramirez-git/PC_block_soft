package guard

import "pcblock/internal/ipc"

// Имена кнопок и поля для команд TestGuard и ответа GuardState (тестовый режим, Ruling 5).
// Три первых имени заданы каналом связи (ipc.Button*); остальные кнопки панелей экрана
// блокировки в канале не описаны, их имена задаёт страж, а приёмка (T16) берёт их отсюда.
const (
	TargetParentPassword = ipc.ButtonParentPassword // «Пароль родителя» на экране
	TargetLogoff         = ipc.ButtonLogoff         // «Выйти из учётной записи» на экране
	TargetAck            = ipc.ButtonAck            // «Понятно» на плашке
	TargetSubmit         = "submit"                 // «Продолжить» на панели пароля
	TargetCancel         = "cancel"                 // «Отмена» на любой панели
	TargetAdd15          = "add15"                  // «+15»
	TargetAdd30          = "add30"                  // «+30»
	TargetAdd60          = "add60"                  // «+60»
	TargetOpenSettings   = "openSettings"           // «Открыть настройки»
	TargetClearBlock     = "clearBlock"             // «Снять блокировку»
	TargetConfirmLogoff  = "confirmLogoff"          // «Выйти» в подтверждении выхода
)

// FieldPasswordID — идентификатор поля пароля в TestGuard (действие type) и в GuardState.Fields.
const FieldPasswordID = "password"

var targetNames = map[Button]string{
	ButtonParentPassword: TargetParentPassword,
	ButtonLogoff:         TargetLogoff,
	ButtonSubmitPassword: TargetSubmit,
	ButtonCancel:         TargetCancel,
	ButtonAdd15:          TargetAdd15,
	ButtonAdd30:          TargetAdd30,
	ButtonAdd60:          TargetAdd60,
	ButtonOpenSettings:   TargetOpenSettings,
	ButtonClearBlock:     TargetClearBlock,
	ButtonConfirmLogoff:  TargetConfirmLogoff,
	ButtonWarningOK:      TargetAck,
}

// TargetName возвращает имя кнопки для канала связи; у ButtonNone и неизвестного значения пусто.
func TargetName(b Button) string { return targetNames[b] }

// ButtonByTarget находит кнопку по имени из команды TestGuard; незнакомое имя даёт false.
func ButtonByTarget(name string) (Button, bool) {
	if name == "" {
		return ButtonNone, false
	}
	for b, n := range targetNames {
		if n == name {
			return b, true
		}
	}
	return ButtonNone, false
}

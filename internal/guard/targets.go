package guard

import "pcblock/internal/ipc"

// Имена кнопок и поля для команд TestGuard и ответа GuardState (тестовый режим, Ruling 5).
// Первые пять имён заданы каналом связи (ipc.Button*); остальные кнопки панелей экрана
// блокировки в канале не описаны, их имена задаёт страж, а приёмка (T16) берёт их отсюда.
const (
	TargetParentPassword  = ipc.ButtonParentPassword // «Пароль родителя» на экране
	TargetLogoff          = ipc.ButtonLogoff         // «Выйти из учётной записи» на экране
	TargetShutdown        = ipc.ButtonShutdown       // «Выключить компьютер» на экране
	TargetSwitchUser      = ipc.ButtonSwitchUser     // «Смена пользователя» на экране
	TargetWarningClose    = ipc.ButtonWarningClose   // крестик «Закрыть» на плашке-табло
	TargetSubmit          = "submit"                 // «Продолжить» на панели пароля
	TargetCancel          = "cancel"                 // «Отмена» на любой панели
	TargetAdd15           = "add15"                  // «+15»
	TargetAdd30           = "add30"                  // «+30»
	TargetAdd60           = "add60"                  // «+60»
	TargetOpenSettings    = "openSettings"           // «Открыть настройки»
	TargetClearBlock      = "clearBlock"             // «Снять блокировку»
	TargetConfirmLogoff   = "confirmLogoff"          // «Выйти» в подтверждении выхода
	TargetConfirmShutdown = "confirmShutdown"        // «Выключить» в подтверждении выключения
	TargetAskMore         = "askMore"                // «Попросить ещё» на экране (FR-10)
	TargetSpendBonus      = "spendBonus"             // «Взять из копилки» на экране (FR-11)
	TargetPromo           = "promo"                  // «Ввести промокод» на экране (FR-35)
	TargetSubmitPromo     = "submitPromo"            // «Применить» на панели промокода
	TargetAskNow          = "askNow"                 // «Попросить сейчас» на плашке «ждёт разрешения» (RM3-4)
	TargetPlateClose      = "plateClose"             // крестик плашки о закрытой программе (FR-13)
	TargetSpendBonusPlate = "spendBonusPlate"        // «Взять из копилки» на плашке 1 мин (RM3-5)
	TargetAskMorePlate    = "askMorePlate"           // «Попросить ещё» на плашке 1 мин (RM3-5)
)

// FieldPasswordID — идентификатор поля пароля в TestGuard (действие type) и в GuardState.Fields.
const FieldPasswordID = "password"

// FieldPromoID — идентификатор поля промокода в TestGuard (действие type) и в GuardState.Fields.
const FieldPromoID = "promo"

var targetNames = map[Button]string{
	ButtonParentPassword:  TargetParentPassword,
	ButtonLogoff:          TargetLogoff,
	ButtonSubmitPassword:  TargetSubmit,
	ButtonCancel:          TargetCancel,
	ButtonAdd15:           TargetAdd15,
	ButtonAdd30:           TargetAdd30,
	ButtonAdd60:           TargetAdd60,
	ButtonOpenSettings:    TargetOpenSettings,
	ButtonClearBlock:      TargetClearBlock,
	ButtonConfirmLogoff:   TargetConfirmLogoff,
	ButtonShutdown:        TargetShutdown,
	ButtonConfirmShutdown: TargetConfirmShutdown,
	ButtonSwitchUser:      TargetSwitchUser,
	ButtonWarningClose:    TargetWarningClose,
	ButtonAskMore:         TargetAskMore,
	ButtonSpendBonus:      TargetSpendBonus,
	ButtonPromo:           TargetPromo,
	ButtonSubmitPromo:     TargetSubmitPromo,
	ButtonAskNow:          TargetAskNow,
	ButtonPlateClose:      TargetPlateClose,
	ButtonPlateSpendBonus: TargetSpendBonusPlate,
	ButtonPlateAskMore:    TargetAskMorePlate,
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

// isPlateButton: кнопка текстовой плашки (не экрана блокировки и не табло): её нажимает щелчок в
// окне плашки.
func isPlateButton(b Button) bool {
	switch b {
	case ButtonAskNow, ButtonPlateClose, ButtonPlateSpendBonus, ButtonPlateAskMore:
		return true
	}
	return false
}

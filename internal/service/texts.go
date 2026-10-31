package service

import (
	"fmt"
	"strings"
)

// Тексты ответов службы (L-112, NFR-11): родителю на «вы», ребёнку на «ты», без длинных
// тире и без технических подробностей. Подробности ошибок идут только в журнал работы.
const (
	textDenyKind      = "Это действие недоступно из этой учётной записи"
	textNeedParent    = "Нужен вход родителя. Введите пароль ещё раз"
	textNeedAdmin     = "Для этого нужны права администратора с подтверждением Windows"
	textTestOnly      = "Это действие доступно только в тестовом режиме"
	textNotConfigured = "Программа ещё не настроена. Сначала пройдите настройку"
	textAlreadySetup  = "Программа уже настроена"
	textSessionEnded  = "Сессия родителя закончилась. Введите пароль ещё раз"
	textRetry         = "Не удалось выполнить. Попробуйте ещё раз"
	textSaveFailed    = "Не удалось сохранить. Попробуйте ещё раз"
	textNoSuchChild   = "Такого ребёнка нет в списке"
	textEnterPassword = "Введите пароль"
	textWrongPassword = "Неверный пароль"
	textHashBroken    = "Пароль сейчас проверить не получается. Администратор Windows может задать новый"
	textNotAChild     = "Эта страница только для детской учётной записи"
	textNoGuard       = "Экран защиты сейчас не запущен"
	textManyGuards    = "Запущено несколько экранов защиты, укажите сеанс"
	textNoPause       = "Часы нельзя остановить: тестовое ускорение не включено"
	textWindowKind    = "Такого окна нет"
	textUndoGone      = "Отменить уже нельзя"
	textSaved         = "Сохранено. Новые правила уже действуют"
	textBlockCleared  = "Блокировка снята"
	textPasswordSet   = "Пароль изменён"
	textMinutesRange  = "Можно добавить от 1 до 240 минут"
	textReduceRange   = "Можно убрать от 1 до 240 минут"
	textIntervalSame  = "Начало и конец совпадают"
	textIntervalRange = "Время должно быть от 00:00 до 23:59"
	textBonusRange    = "Можно начислить от 1 до 600 минут"
	textBonusReason   = "Укажите, за что начисляете"
	textLimitTooBig   = "Не больше 24 ч"
	textLimitNegative = "Не меньше 0 мин"
	textLimitsCount   = "Нужны лимиты на все семь дней"
	textEndModeBad    = "Выберите, что делать, когда время вышло"
	// «Программы» (FR-20, FR-2).
	textCategoryBad     = "Выберите: игра, учёба, прочее или запретить"
	textNoProgram       = "Такой программы нет в списке"
	textTooManyPrograms = "Можно изменить не больше 200 программ за раз"
	textProgramsChanged = "Изменено программ: %d"
	textProgramUndone   = "Решение отменено"
	textProgramHidden   = "%s скрыта"
	textProgramsHidden  = "Скрыто программ: %d"
	textProgramShown    = "%s снова в списке"
	textProgramsShown   = "Снова в списке программ: %d"
	textScanFailed      = "Не удалось найти программы. Их можно настроить позже в разделе «Программы»"
	// Плашки ребёнку о закрытой программе (FR-13, 8.2.4): на «ты»; закрытие по лимиту игр и в
	// режиме «только учёба» берёт текст ядра (Decision.CloseReason).
	textPlateUnknown   = "Программа «%s» ждёт разрешения родителя"
	textPlateForbidden = "Программа «%s» запрещена родителем"
	textNoChildren     = "Выберите хотя бы одну детскую учётку"
	textNoAccount      = "Такой учётки нет на этом компьютере"
	textAdminAccount   = "Учётку администратора нельзя сделать детской. Сначала сделайте её обычной"
	textNameEmpty      = "Введите имя"
	textAdminNotice    = "Администратор: ограничения не действуют"
	// Событие журнала о сбое, который программа пережила (NFR-11: без технических слов).
	textInternalFailure   = "Программа не смогла выполнить одно из действий и продолжила работу"
	textInternalFailureOf = "Не удалось обработать учётку «%s», программа продолжила работу"
	// textRecoveredNotice — что видит родитель после замены испорченной базы (R18, Ruling 25, NFR-11):
	// без слов «база данных» и «служба».
	textRecoveredNotice = "Не удалось прочитать правила, поэтому сейчас ограничения не действуют. Прежние данные сохранены. Задайте правила заново в разделе «Дети»"

	// Подробности блокировки за обход (R17в), которые видит ребёнок: сдержанно и без обвинений.
	detailTrayClosed   = "Значок программы был закрыт"
	detailGuardClosed  = "Защита программы была остановлена"
	detailWindowClosed = "Окно программы было закрыто не кнопкой окна"
)

// minutesWord возвращает слово «минута» в нужной форме для числа n (1 минуту, 2 минуты, 5 минут).
func minutesWord(n int) string {
	n = abs(n)
	switch {
	case n%100 >= 11 && n%100 <= 14:
		return "минут"
	case n%10 == 1:
		return "минуту"
	case n%10 >= 2 && n%10 <= 4:
		return "минуты"
	}
	return "минут"
}

// minutesText возвращает «30 минут», «1 минуту», «21 минуту».
func minutesText(n int) string { return fmt.Sprintf("%d %s", n, minutesWord(n)) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// clockText возвращает длительность в виде м:сс («0:59», «1:00») для пауз ввода пароля.
func clockText(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// wrongPasswordText возвращает подпись под полем после неверного пароля.
func wrongPasswordText(attemptsLeft int) string {
	return fmt.Sprintf("%s. Осталось попыток: %d", textWrongPassword, attemptsLeft)
}

// pauseText возвращает подпись, пока ввод пароля закрыт после серии ошибок (R19).
func pauseText(seconds int) string {
	return "Слишком много попыток. Повторить можно через " + clockText(seconds)
}

// capitalize делает первую букву строки заглавной.
func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

// itoa превращает число в строку.
func itoa(n int) string { return fmt.Sprint(n) }

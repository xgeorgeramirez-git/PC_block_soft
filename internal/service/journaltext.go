package service

import (
	"fmt"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/programs"
)

// months — месяцы в родительном падеже для «19 октября».
var months = [12]string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

// detInt читает число из подробностей события: из базы оно приходит как float64.
func detInt(d map[string]any, key string) int {
	switch v := d[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func detStr(d map[string]any, key string) string {
	s, _ := d[key].(string)
	return s
}

// dayText превращает «2026-10-19» в «19 октября»; не разобралось: день как есть.
func dayText(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return fmt.Sprintf("%d %s", t.Day(), months[t.Month()-1])
}

// eventText собирает строку журнала простыми словами (R22, ТЗ 8.3.7): «Миша: доп. время +30 мин».
// Событие не про ребёнка начинается с заглавной буквы без имени.
func eventText(kind, name string, d map[string]any) string {
	var body string
	switch kind {
	case core.EvExtraTime:
		// Убавленное время хранится отрицательным числом: «−60», а не «+-60» (знак U+2212, как в окне родителя).
		if m := detInt(d, "minutes"); m < 0 {
			body = fmt.Sprintf("доп. время −%d мин", -m)
		} else {
			body = fmt.Sprintf("доп. время +%d мин", m)
		}
	case "extra_undone":
		body = "изменение доп. времени отменено"
	case core.EvBypass:
		body = "попытка обойти защиту"
		if det := detStr(d, "detail"); det != "" {
			body += ". " + det
		}
	case core.EvBlockCleared:
		body = "блокировка снята родителем"
	case core.EvSafeMode:
		body = "правила не прочитаны, ограничения не действуют"
	case core.EvDayClosed:
		body = fmt.Sprintf("итог дня %s: %s", dayText(detStr(d, "day")), core.FormatDuration(detInt(d, "total_s")))
	case "child_admin":
		body = "учётка получила права администратора, ограничения не действуют"
	case "child_admin_off":
		body = "права администратора сняты, ограничения снова действуют"
	case "password_failed":
		body = "неверный пароль родителя"
	case "password_changed":
		body = "пароль родителя изменён"
	case "password_reset":
		body = "пароль родителя сброшен администратором"
	case "setup_done":
		body = "программа настроена"
	case "db_recovered":
		body = "не удалось прочитать правила, ограничения не действовали, прежние данные сохранены"
	case "uninstall_forgot":
		body = "удаление программы без пароля, пароль забыт"
	case evProgramDecided:
		body = decidedText(d)
	case evProgramUndone:
		body = "решение по программам отменено"
	case evProgramPending:
		body = "программа «" + eventProgram(d) + "» ждёт решения"
	case evProgramForbiddenTry:
		body = "попытка открыть запрещённое: «" + eventProgram(d) + "»"
	case evProgramClosedGames:
		body = "закрыта «" + eventProgram(d) + "»: время на игры закончилось"
	case evProgramClosedStudyOnly:
		body = "закрыта «" + eventProgram(d) + "»: время вышло, доступна только учёба"
	default:
		body = detStr(d, "text")
		if body == "" {
			body = kind
		}
	}
	if name == "" {
		return capitalize(body)
	}
	return name + ": " + body
}

// eventProgram — имя программы события: из подробностей (name), иначе из пути; не длиннее 120 символов.
func eventProgram(d map[string]any) string {
	if n := programs.ClipName(detStr(d, "name")); n != "" {
		return n
	}
	return programs.NameFromPath(detStr(d, "path"))
}

// decidedText — решение родителя о программах (R22): «вы отнесли «SuperGame» к категории «Игры»»,
// «вы запретили 5 программ». Начало строки делает заглавным eventText.
func decidedText(d map[string]any) string {
	cat := detStr(d, "category")
	subject := "«" + eventProgram(d) + "»"
	if n := detInt(d, "count"); n > 1 || eventProgram(d) == "" {
		subject = fmt.Sprintf("%d %s", n, programsWord(n))
	}
	if cat == programs.CatBlocked {
		return "вы запретили " + subject
	}
	return "вы отнесли " + subject + " к категории «" + categoryTitle[cat] + "»"
}

// programsWord — «программу», «программы», «программ» для числа n (винительный падеж).
func programsWord(n int) string {
	n = abs(n)
	switch {
	case n%100 >= 11 && n%100 <= 14:
		return "программ"
	case n%10 == 1:
		return "программу"
	case n%10 >= 2 && n%10 <= 4:
		return "программы"
	}
	return "программ"
}

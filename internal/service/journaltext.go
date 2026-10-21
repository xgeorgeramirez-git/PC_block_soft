package service

import (
	"fmt"
	"time"

	"pcblock/internal/core"
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
		body = fmt.Sprintf("доп. время +%d мин", detInt(d, "minutes"))
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

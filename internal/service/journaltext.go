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

// bonusGrantedText — строка журнала автобонуса (R25): «Бонус за свободное время 24 ноября: +40 мин».
func bonusGrantedText(day string, minutes int) string {
	return fmt.Sprintf("Бонус за свободное время %s: +%d мин", dayText(day), minutes)
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
	case evExtraUndone:
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
	case core.EvBonusGranted:
		// Дата словами и у записей прежних версий, где в тексте «2026-11-24» (FB-21).
		body = detStr(d, "text")
		if day := detStr(d, "day"); day != "" {
			body = decapitalize(bonusGrantedText(day, detInt(d, "minutes")))
		}
	case core.EvDayClosed:
		body = fmt.Sprintf("итог дня %s: %s", dayText(detStr(d, "day")), core.FormatDuration(detInt(d, "total_s")))
	case evChildAdmin:
		body = "учётка получила права администратора, ограничения не действуют"
	case evChildAdminOff:
		body = "права администратора сняты, ограничения снова действуют"
	case evPasswordFailed:
		body = "неверный пароль родителя"
	case evPasswordChanged:
		body = "пароль родителя изменён"
	case evPasswordReset:
		body = "пароль родителя сброшен администратором"
	case evSetupDone:
		body = "программа настроена"
	case evDBRecovered:
		body = "не удалось прочитать правила, ограничения не действовали, прежние данные сохранены"
	case evUninstallForgot:
		body = "удаление программы без пароля, пароль забыт"
	case evProgramDecided:
		body = decidedText(d)
	case evProgramUndone:
		body = "решение по программам отменено"
	case evProgramHidden:
		body = "вы скрыли " + hiddenSubject(d)
	case evProgramShown:
		body = "вы снова показали " + hiddenSubject(d)
	case evProgramRenamed:
		if d["reset"] == true {
			body = "вы вернули «" + programs.ClipName(detStr(d, "from")) + "» название из файла «" + programs.ClipName(detStr(d, "to")) + "»"
		} else {
			body = "вы переименовали «" + programs.ClipName(detStr(d, "from")) + "» в «" + programs.ClipName(detStr(d, "to")) + "»"
		}
	case evProgramPending:
		body = "программа «" + eventProgram(d) + "» ждёт решения"
	case evProgramForbiddenTry:
		body = "попытка открыть запрещённое: «" + eventProgram(d) + "»"
	case evProgramAutoClosed:
		body = "программа «" + eventProgram(d) + "» запускалась сама и закрыта"
	case evProgramClosedGames:
		body = "закрыта «" + eventProgram(d) + "»: время на игры закончилось"
	case evProgramClosedStudyOnly:
		body = "закрыта «" + eventProgram(d) + "»: время вышло, доступна только учёба"
	case evSiteForbiddenTry:
		if detStr(d, "kind") == siteKindAdult {
			body = "попытка открыть запрещённое: сайт для взрослых " + detStr(d, "domain")
		} else {
			body = "попытка открыть запрещённое: сайт " + detStr(d, "domain")
		}
	case evSiteClosedVideo:
		body = "закрыт сайт " + detStr(d, "domain") + ": время на видео и соцсети закончилось"
	case evSiteClosedStudyOnly:
		body = "закрыт сайт " + detStr(d, "domain") + ": время вышло, доступна только учёба"
	case evSiteUnread:
		body = textSiteUnread
	case evSiteAdded:
		body = "вы добавили сайт " + detStr(d, "domain") + " в " + siteListNames[detStr(d, "kind")].acc
	case evSiteMoved:
		body = "вы перенесли сайт " + detStr(d, "domain") + " в " + siteListNames[detStr(d, "kind")].acc
	case evSiteRemoved:
		body = "вы удалили сайт " + detStr(d, "domain") + " из " + siteListNames[detStr(d, "kind")].gen
	case evSiteAdultFilter:
		body = textAdultOff
		if d["on"] == true {
			body = textAdultOn
		}
	case evStatsCleared:
		body = "статистика очищена"
	case evJournalCleared:
		body = "журнал очищен"
	case evPromoGenerate:
		// Коды — в подробностях codes, в текст журнала не идут (RM5-12).
		body = promoGenerateText(detInt(d, "count"), detInt(d, "minutes"))
	case evPromoHistoryCleared:
		body = fmt.Sprintf("история промокодов очищена (%d)", detInt(d, "count"))
	case evBraveRestored:
		body = "правила Brave восстановлены"
	case core.EvRestBreak, core.EvRestEnded:
		// «Миша: перерыв на отдых: 15 мин после 60 мин за компьютером», «Миша: вы закончили перерыв».
		body = decapitalize(detStr(d, "text"))
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

// hiddenSubject — «Stalker2» в кавычках для одной программы, «3 программы» для нескольких.
func hiddenSubject(d map[string]any) string {
	if n := detInt(d, "count"); n > 1 || eventProgram(d) == "" {
		return fmt.Sprintf("%d %s", n, programsWord(n))
	}
	return "«" + eventProgram(d) + "»"
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

package storage

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"pcblock/internal/auth"
	"pcblock/internal/core"
)

const day1 = "2026-10-19"

func ptr[T any](v T) *T { return &v }

// weekStr печатает лимиты недели читаемо: «общее/игры», минуты или «-» для «без лимита» (в
// сообщении об ошибке указатели из core.DayLimit были бы адресами).
func weekStr(week [7]core.DayLimit) string {
	num := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	parts := make([]string, 0, 7)
	for _, l := range week {
		parts = append(parts, num(l.TotalMin)+"/"+num(l.GamesMin))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// limit — лимит дня: общий total и игр games (nil — без отдельного лимита игр).
func limit(total int, games *int) core.DayLimit {
	l := core.LimitOf(total)
	l.GamesMin = games
	return l
}

// Ловит: пустой список детей уходит во фронт как null и роняет экран (L-22); порядок списка
// плавает; список теряет сведения о блокировке за обход.
func TestListChildrenEmptyAndOrdered(t *testing.T) {
	s := openTemp(t)
	empty, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil {
		t.Fatal("ListChildren пустой базы = nil, want []Child{}")
	}
	if b, _ := json.Marshal(empty); string(b) != "[]" {
		t.Fatalf("JSON пустого списка = %s, want []", b)
	}

	fixClock(t, 5_000)
	a := mustChild(t, s, "S-1-5-21-1", "Миша")
	b := mustChild(t, s, "S-1-5-21-2", "Аня")
	if err := s.SetBypass(b, "Попытка закрыть защиту"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	want := []Child{
		{ID: a, SID: "S-1-5-21-1", Name: "Миша", EndMode: "lock", CreatedAtMs: 5_000},
		{ID: b, SID: "S-1-5-21-2", Name: "Аня", BypassDetail: "Попытка закрыть защиту", EndMode: "lock", CreatedAtMs: 5_000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListChildren = %+v, want %+v", got, want)
	}
}

// Ловит: та же учётка (тот же SID) добавляется второй строкой или получает новый id, и
// статистика, привязанная к старому id, теряется; имя не обновляется; пустые SID и имя проходят.
func TestUpsertChildSameSIDKeepsID(t *testing.T) {
	s := openTemp(t)
	fixClock(t, 1_000)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	fixClock(t, 9_000)
	again, err := s.UpsertChild(Child{SID: "S-1-5-21-1", Name: "Михаил"})
	if err != nil {
		t.Fatal(err)
	}
	if again != id {
		t.Fatalf("id при повторном UpsertChild = %d, want %d", again, id)
	}
	got, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	want := []Child{{ID: id, SID: "S-1-5-21-1", Name: "Михаил", EndMode: "lock", CreatedAtMs: 1_000}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListChildren = %+v, want %+v (имя новое, created_at прежний)", got, want)
	}
	for _, c := range []Child{{SID: "", Name: "Миша"}, {SID: "S-1", Name: ""}} {
		if _, err := s.UpsertChild(c); !errors.Is(err, ErrInvalid) {
			t.Errorf("UpsertChild(%+v) = %v, want ErrInvalid", c, err)
		}
	}
}

// Ловит: RemoveChild удаляет строки статистики и событий (FR-25 «Статистика сохранится»),
// не ставит removed_at по часам хранилища или оставляет ребёнка в списке.
func TestRemoveChildHidesChildButKeepsData(t *testing.T) {
	s := openTemp(t)
	fixClock(t, 1_000)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	other := mustChild(t, s, "S-1-5-21-2", "Аня")
	limits := [7]core.DayLimit{core.LimitOf(120), {}, core.LimitOf(0), {}, {}, {}, core.LimitOf(240)}
	if err := s.SetDayLimits(id, limits); err != nil {
		t.Fatal(err)
	}
	st := core.DayState{Day: day1, TotalS: 3600, GamesS: 600, VideoS: 300, StudyS: 120, ExtraMin: 30}
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(Event{ChildID: ptr(id), AtMs: 10, Kind: "extra_time"}); err != nil {
		t.Fatal(err)
	}

	fixClock(t, 7_000)
	if err := s.RemoveChild(id); err != nil {
		t.Fatalf("RemoveChild: %v", err)
	}
	if v := rawNull(t, s, `SELECT removed_at FROM children WHERE id = ?`, id); !v.Valid || v.Int64 != 7_000 {
		t.Errorf("removed_at = %+v, want 7000", v)
	}
	kids, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].ID != other {
		t.Fatalf("ListChildren после удаления = %+v, want только id=%d", kids, other)
	}
	gotSt, err := s.LoadDayState(id, day1)
	if err != nil || gotSt != st {
		t.Errorf("LoadDayState после удаления = %+v, %v; want %+v", gotSt, err, st)
	}
	gotLim, err := s.GetDayLimits(id)
	if err != nil || !reflect.DeepEqual(gotLim, limits) {
		t.Errorf("GetDayLimits после удаления = %s, %v; want %s", weekStr(gotLim), err, weekStr(limits))
	}
	evs, err := s.ListEvents(EventFilter{ChildID: ptr(id)})
	if err != nil || len(evs) != 1 || evs[0].Kind != "extra_time" {
		t.Errorf("ListEvents после удаления = %+v, %v; want одно событие extra_time", evs, err)
	}
}

// Ловит: учётка, снова отмеченная детской, получает новый id и новую пустую статистику, остаётся
// скрытой (removed_at не обнуляется) или теряет прежнее время создания.
func TestUpsertChildAfterRemoveRestoresSameChild(t *testing.T) {
	s := openTemp(t)
	fixClock(t, 1_000)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	st := core.DayState{Day: day1, TotalS: 600}
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	fixClock(t, 2_000)
	if err := s.RemoveChild(id); err != nil {
		t.Fatal(err)
	}
	fixClock(t, 3_000)
	back, err := s.UpsertChild(Child{SID: "S-1-5-21-1", Name: "Михаил"})
	if err != nil {
		t.Fatal(err)
	}
	if back != id {
		t.Fatalf("id после возврата = %d, want %d", back, id)
	}
	if v := rawNull(t, s, `SELECT removed_at FROM children WHERE id = ?`, id); v.Valid {
		t.Errorf("removed_at после возврата = %+v, want NULL", v)
	}
	kids, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	want := []Child{{ID: id, SID: "S-1-5-21-1", Name: "Михаил", EndMode: "lock", CreatedAtMs: 1_000}}
	if !reflect.DeepEqual(kids, want) {
		t.Fatalf("ListChildren = %+v, want %+v", kids, want)
	}
	if got, err := s.LoadDayState(id, day1); err != nil || got != st {
		t.Errorf("статистика после возврата = %+v, %v; want %+v", got, err, st)
	}
}

// Ловит: повторное удаление сдвигает removed_at на новый момент; удаление несуществующего
// ребёнка проходит молча и служба считает, что отметку сняла.
func TestRemoveChildTwiceAndUnknown(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	fixClock(t, 4_000)
	if err := s.RemoveChild(id); err != nil {
		t.Fatal(err)
	}
	fixClock(t, 8_000)
	if err := s.RemoveChild(id); err != nil {
		t.Fatalf("повторный RemoveChild: %v", err)
	}
	if v := rawNull(t, s, `SELECT removed_at FROM children WHERE id = ?`, id); v.Int64 != 4_000 {
		t.Errorf("removed_at = %d, want 4000 (первое удаление)", v.Int64)
	}
	if err := s.RemoveChild(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("RemoveChild(999) = %v, want ErrNotFound", err)
	}
}

// Ловит: блокировка за обход (R17, Ruling 18) не сохраняется, пустая строка пишется как
// текст вместо NULL (и тогда «нет блокировки» читается как блокировка), снятие не работает, а неизвестный
// ребёнок принимается молча.
func TestSetBypassStoresAndClears(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	if err := s.SetBypass(id, "Попытка завершить стража"); err != nil {
		t.Fatal(err)
	}
	kids, _ := s.ListChildren()
	if len(kids) != 1 || kids[0].BypassDetail != "Попытка завершить стража" {
		t.Fatalf("после SetBypass: %+v", kids)
	}
	if err := s.SetBypass(id, ""); err != nil {
		t.Fatal(err)
	}
	if v := rawNull(t, s, `SELECT count(*) FROM children WHERE id = ? AND bypass_detail IS NULL`, id); v.Int64 != 1 {
		t.Error("пустая строка не записала NULL в bypass_detail")
	}
	kids, _ = s.ListChildren()
	if kids[0].BypassDetail != "" {
		t.Errorf("BypassDetail после снятия = %q, want пусто", kids[0].BypassDetail)
	}
	if err := s.SetBypass(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetBypass(999) = %v, want ErrNotFound", err)
	}
}

// Ловит: режим «Когда время вышло» (R8, children.end_mode) не сохраняется, не читается
// списком детей или чтением одного ребёнка, по умолчанию не «экран блокировки», принимает
// мусорное значение (в базе CHECK, у службы безопасный «lock») или молча меняет чужого ребёнка.
func TestSetGetEndMode(t *testing.T) {
	s := openTemp(t)
	a := mustChild(t, s, "S-1-5-21-1", "Миша")
	b := mustChild(t, s, "S-1-5-21-2", "Аня")
	if m, err := s.GetEndMode(a); err != nil || m != "lock" {
		t.Fatalf("GetEndMode по умолчанию = %q, %v; want lock", m, err)
	}
	if err := s.SetEndMode(a, "study_only"); err != nil {
		t.Fatalf("SetEndMode: %v", err)
	}
	if m, err := s.GetEndMode(a); err != nil || m != "study_only" {
		t.Errorf("GetEndMode после записи = %q, %v; want study_only", m, err)
	}
	kids, err := s.ListChildren()
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 2 || kids[0].EndMode != "study_only" || kids[1].EndMode != "lock" {
		t.Errorf("ListChildren = %+v; want Миша study_only, Аня lock", kids)
	}
	for _, bad := range []string{"", "kill", "STUDY_ONLY"} {
		if err := s.SetEndMode(b, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("SetEndMode(%q) = %v, want ErrInvalid", bad, err)
		}
	}
	if m, _ := s.GetEndMode(b); m != "lock" {
		t.Errorf("режим Ани после отказов = %q, want lock", m)
	}
	if err := s.SetEndMode(a, "lock"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.GetEndMode(a); m != "lock" {
		t.Errorf("режим после возврата = %q, want lock", m)
	}
	if err := s.SetEndMode(999, "lock"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetEndMode(999) = %v, want ErrNotFound", err)
	}
	if m, err := s.GetEndMode(999); err != nil || m != "lock" {
		t.Errorf("GetEndMode(999) = %q, %v; want lock, nil (пусто — норма)", m, err)
	}
}

// Ловит: у ребёнка без сохранённых лимитов появляется лимит 0 («нельзя») вместо «без лимита»,
// и первый же вход ребёнка блокируется.
func TestGetDayLimitsDefaultsToUnlimited(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	got, err := s.GetDayLimits(id)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range got {
		if !l.Unlimited() {
			t.Errorf("день %d: лимит %d мин, want без лимита", i+1, *l.TotalMin)
		}
	}
	if got, err := s.GetDayLimits(999); err != nil || got != ([7]core.DayLimit{}) {
		t.Errorf("GetDayLimits неизвестного ребёнка = %s, %v; want все без лимита, nil", weekStr(got), err)
	}
}

// Ловит: дни недели сдвинуты (пн записан как 0 или вс как 7→1), «без лимита» и 0 минут
// путаются (и для общего лимита, и для лимита игр R5), повторное сохранение дублирует строки,
// лимиты одного ребёнка видны другому, запись лимитов теряет игры или затирает video_min (M4).
func TestSetGetDayLimitsRoundTrip(t *testing.T) {
	s := openTemp(t)
	a := mustChild(t, s, "S-1-5-21-1", "Миша")
	b := mustChild(t, s, "S-1-5-21-2", "Аня")
	week := [7]core.DayLimit{
		limit(120, ptr(60)), {}, limit(0, ptr(0)), limit(30, nil), {GamesMin: ptr(45)}, limit(1440, ptr(1440)), limit(5, ptr(90)),
	}
	if err := s.SetDayLimits(a, week); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE day_limits SET video_min = 20 WHERE child_id = ? AND weekday = 1`, a); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDayLimits(a)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, week) {
		t.Fatalf("GetDayLimits = %s, want %s", weekStr(got), weekStr(week))
	}
	if n := rawNull(t, s, `SELECT count(*) FROM day_limits WHERE child_id = ?`, a); n.Int64 != 7 {
		t.Errorf("строк day_limits = %d, want 7", n.Int64)
	}
	if v := rawNull(t, s, `SELECT total_min FROM day_limits WHERE child_id = ? AND weekday = 2`, a); v.Valid {
		t.Errorf("вторник без лимита записан как %d, want NULL", v.Int64)
	}
	if v := rawNull(t, s, `SELECT total_min FROM day_limits WHERE child_id = ? AND weekday = 7`, a); v.Int64 != 5 {
		t.Errorf("воскресенье total_min = %d, want 5", v.Int64)
	}
	if g := rawNull(t, s, `SELECT games_min FROM day_limits WHERE child_id = ? AND weekday = 1`, a); !g.Valid || g.Int64 != 60 {
		t.Errorf("games_min понедельника = %+v, want 60", g)
	}
	if g := rawNull(t, s, `SELECT games_min FROM day_limits WHERE child_id = ? AND weekday = 4`, a); g.Valid {
		t.Errorf("четверг без лимита игр записан как %d, want NULL", g.Int64)
	}
	if g := rawNull(t, s, `SELECT games_min FROM day_limits WHERE child_id = ? AND weekday = 3`, a); !g.Valid || g.Int64 != 0 {
		t.Errorf("games_min среды = %+v, want 0 (игры нельзя, не NULL)", g)
	}

	week2 := [7]core.DayLimit{{}, limit(60, ptr(20)), {}, {}, {}, {}, {}}
	if err := s.SetDayLimits(a, week2); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetDayLimits(a); !reflect.DeepEqual(got, week2) {
		t.Errorf("после перезаписи GetDayLimits = %s, want %s", weekStr(got), weekStr(week2))
	}
	if n := rawNull(t, s, `SELECT count(*) FROM day_limits WHERE child_id = ?`, a); n.Int64 != 7 {
		t.Errorf("строк day_limits после перезаписи = %d, want 7", n.Int64)
	}
	if g := rawNull(t, s, `SELECT games_min FROM day_limits WHERE child_id = ? AND weekday = 1`, a); g.Valid {
		t.Errorf("games_min понедельника после перезаписи = %d, want NULL (игры без лимита)", g.Int64)
	}
	if v := rawNull(t, s, `SELECT video_min FROM day_limits WHERE child_id = ? AND weekday = 1`, a); v.Int64 != 20 {
		t.Errorf("video_min понедельника = %+v, want 20 (поле M4 не трогаем)", v)
	}
	if got, _ := s.GetDayLimits(b); got != ([7]core.DayLimit{}) {
		t.Errorf("лимиты второго ребёнка = %s, want все без лимита", weekStr(got))
	}
}

// Ловит: лимит (общий или игр) вне 0…1440 доходит до базы и даёт сырую ошибку CHECK либо
// записывает часть недели до ошибки; неизвестный ребёнок принимается.
func TestSetDayLimitsRejectsBadInput(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	good := [7]core.DayLimit{core.LimitOf(60), core.LimitOf(60), core.LimitOf(60), core.LimitOf(60), core.LimitOf(60), core.LimitOf(60), core.LimitOf(60)}
	if err := s.SetDayLimits(id, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []int{-1, 1441} {
		week := good
		week[6] = core.LimitOf(bad)
		week[0] = core.LimitOf(10)
		if err := s.SetDayLimits(id, week); !errors.Is(err, ErrInvalid) {
			t.Errorf("SetDayLimits с %d мин = %v, want ErrInvalid", bad, err)
		}
	}
	for _, bad := range []int{-1, 1441} {
		week := good
		week[2] = limit(60, ptr(bad))
		if err := s.SetDayLimits(id, week); !errors.Is(err, ErrInvalid) {
			t.Errorf("SetDayLimits с играми %d мин = %v, want ErrInvalid", bad, err)
		}
	}
	if got, _ := s.GetDayLimits(id); !reflect.DeepEqual(got, good) {
		t.Errorf("после отказов лимиты = %s, want прежние %s", weekStr(got), weekStr(good))
	}
	if err := s.SetDayLimits(999, good); err == nil {
		t.Error("SetDayLimits для ребёнка 999 прошёл, want ошибку")
	}
}

// Ловит: нет строки дня трактуется как ошибка (ErrNoRows) вместо пустого состояния «0 секунд»
// (грабля «пусто = норма»), и ядро получает состояние без даты.
func TestLoadDayStateMissingIsEmptyState(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	got, err := s.LoadDayState(id, day1)
	if err != nil {
		t.Fatalf("LoadDayState без строки: %v", err)
	}
	if want := (core.DayState{Day: day1}); got != want {
		t.Fatalf("LoadDayState = %+v, want %+v", got, want)
	}
}

// Ловит: поля состояния перепутаны местами (игры/видео/учёба), второй Save дублирует строку
// или стирает другие дни и другого ребёнка; nil-пауза сохраняется как NULL (R11).
func TestSaveLoadDayStateRoundTrip(t *testing.T) {
	s := openTemp(t)
	a := mustChild(t, s, "S-1-5-21-1", "Миша")
	b := mustChild(t, s, "S-1-5-21-2", "Аня")
	st := core.DayState{Day: day1, TotalS: 5400, GamesS: 1800, VideoS: 900, StudyS: 600, ExtraMin: 15}
	if err := s.SaveDayState(a, st); err != nil {
		t.Fatal(err)
	}
	prev := core.DayState{Day: "2026-10-18", TotalS: 100}
	if err := s.SaveDayState(a, prev); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDayState(b, core.DayState{Day: day1, TotalS: 7}); err != nil {
		t.Fatal(err)
	}
	st2 := core.DayState{Day: day1, TotalS: 5401, GamesS: 1801, VideoS: 901, StudyS: 601, ExtraMin: 16}
	if err := s.SaveDayState(a, st2); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadDayState(a, day1); err != nil || got != st2 {
		t.Errorf("LoadDayState = %+v, %v; want %+v", got, err, st2)
	}
	if got, _ := s.LoadDayState(a, "2026-10-18"); got != prev {
		t.Errorf("прошлый день = %+v, want %+v", got, prev)
	}
	if got, _ := s.LoadDayState(b, day1); got.TotalS != 7 {
		t.Errorf("день второго ребёнка = %+v, want TotalS 7", got)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM day_state WHERE child_id = ? AND day = ?`, a, day1); n.Int64 != 1 {
		t.Errorf("строк за день = %d, want 1", n.Int64)
	}
	// SaveDayState c PauseUntilMs=nil записывает pause_until как NULL (M2: пауза в R11
	// управляется этим полем; round-trip установленной паузы — в TestSaveLoadDayStatePauseUntil).
	if p := rawNull(t, s, `SELECT pause_until FROM day_state WHERE child_id = ? AND day = ?`, a, day1); p.Valid {
		t.Errorf("pause_until = %+v, want NULL (nil-пауза сохраняется как NULL)", p)
	}
}

// Ловит: новые флаги дня (bonus_granted — автобонус начислен, no_forbidden — запрет снят
// на сегодня) не сохраняются или путаются местами; по умолчанию день — без флагов.
func TestSaveLoadDayStateFlags(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	// Строки нет — оба флага false.
	if got, _ := s.LoadDayState(id, day1); got.BonusGranted || got.NoForbidden {
		t.Errorf("пустой день = %+v; want оба флага false", got)
	}
	st := core.DayState{Day: day1, TotalS: 1200, BonusGranted: true, NoForbidden: true}
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadDayState(id, day1); err != nil || got != st {
		t.Errorf("LoadDayState = %+v, %v; want %+v", got, err, st)
	}
	// Снять оба флага перезаписью (ON CONFLICT обновляет столбцы).
	st.BonusGranted = false
	st.NoForbidden = false
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadDayState(id, day1); got.BonusGranted || got.NoForbidden {
		t.Errorf("после снятия = %+v; want оба флага false", got)
	}
}

// Ловит: в день попадает мусор (пусто, «19.10.2026», 2026-13-45), по которому ядро потом не
// найдёт сегодняшнюю строку и начнёт день с нуля.
func TestSaveDayStateRejectsBadDay(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	for _, day := range []string{"", "19.10.2026", "2026-13-45", "2026-1-5", "2026-10-19 "} {
		if err := s.SaveDayState(id, core.DayState{Day: day}); !errors.Is(err, ErrInvalid) {
			t.Errorf("SaveDayState(day=%q) = %v, want ErrInvalid", day, err)
		}
	}
	if n := rawNull(t, s, `SELECT count(*) FROM day_state`); n.Int64 != 0 {
		t.Errorf("строк day_state = %d, want 0", n.Int64)
	}
}

// Ловит: пустой журнал уходит во фронт как null (L-22).
func TestListEventsEmptyIsEmptySlice(t *testing.T) {
	s := openTemp(t)
	got, err := s.ListEvents(EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("ListEvents пустого журнала = nil, want []Event{}")
	}
	if b, _ := json.Marshal(got); string(b) != "[]" {
		t.Fatalf("JSON = %s, want []", b)
	}
}

// Ловит: журнал отдаёт старые события сверху, при равных моментах порядок плавает, предел
// числа строк не действует, нулевой предел даёт пустой журнал.
func TestListEventsNewestFirstWithLimit(t *testing.T) {
	s := openTemp(t)
	for _, e := range []struct {
		kind string
		at   int64
	}{{"a", 100}, {"b", 300}, {"c", 300}, {"d", 200}, {"e", 50}} {
		if err := s.AppendEvent(Event{AtMs: e.at, Kind: e.kind}); err != nil {
			t.Fatal(err)
		}
	}
	kinds := func(f EventFilter) string {
		t.Helper()
		evs, err := s.ListEvents(f)
		if err != nil {
			t.Fatal(err)
		}
		var ks []string
		for _, e := range evs {
			ks = append(ks, e.Kind)
		}
		return strings.Join(ks, "")
	}
	if got := kinds(EventFilter{}); got != "cbdae" {
		t.Errorf("без предела: %q, want %q", got, "cbdae")
	}
	if got := kinds(EventFilter{Limit: 3}); got != "cbd" {
		t.Errorf("Limit 3: %q, want %q", got, "cbd")
	}
	if got := kinds(EventFilter{Limit: -1}); got != "cbdae" {
		t.Errorf("Limit -1: %q, want предел по умолчанию (все 5)", got)
	}
}

// Ловит: предел числа строк журнала без верхней границы (запрос «весь журнал» грузит базу)
// или нулевой предел превращается в «ничего».
func TestEventLimit(t *testing.T) {
	cases := []struct{ in, want int }{{-5, 200}, {0, 200}, {1, 1}, {500, 500}, {1000, 1000}, {1001, 1000}, {1 << 30, 1000}}
	for _, tc := range cases {
		if got := eventLimit(tc.in); got != tc.want {
			t.Errorf("eventLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// Ловит: подробности события теряют русский текст или числа, пустые подробности пишутся как
// null (NOT NULL в схеме) и читаются как nil-карта, ChildID теряется или путается с нулём.
func TestEventRoundTrip(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")
	full := Event{
		ChildID: ptr(id), AtMs: 1_792_423_845_000, TzOffsetMin: 180, Kind: "bypass",
		Details: map[string]any{"detail": "Попытка завершить стража", "n": 2, "tags": []any{"a", "б"}},
	}
	if err := s.AppendEvent(full); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(Event{AtMs: 1, TzOffsetMin: -300, Kind: "password_changed"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListEvents(EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{
			ID: 1, ChildID: ptr(id), AtMs: 1_792_423_845_000, TzOffsetMin: 180, Kind: "bypass",
			// Числа из JSON читаются как float64, это часть договора Details.
			Details: map[string]any{"detail": "Попытка завершить стража", "n": float64(2), "tags": []any{"a", "б"}},
		},
		{ID: 2, ChildID: nil, AtMs: 1, TzOffsetMin: -300, Kind: "password_changed", Details: map[string]any{}},
	}
	// Новые сверху: у первого события момент больше.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListEvents = %+v, want %+v", got, want)
	}
	if raw := rawNull(t, s, `SELECT count(*) FROM events WHERE details = '{}'`); raw.Int64 != 1 {
		t.Errorf("событий с details '{}' = %d, want 1", raw.Int64)
	}
}

// Ловит: фильтр по ребёнку не работает (родитель видит чужие события) или события без
// ребёнка попадают в журнал конкретного ребёнка.
func TestListEventsFilterByChild(t *testing.T) {
	s := openTemp(t)
	a := mustChild(t, s, "S-1-5-21-1", "Миша")
	b := mustChild(t, s, "S-1-5-21-2", "Аня")
	for _, e := range []Event{
		{ChildID: ptr(a), AtMs: 1, Kind: "a1"},
		{ChildID: ptr(b), AtMs: 2, Kind: "b1"},
		{AtMs: 3, Kind: "none"},
		{ChildID: ptr(a), AtMs: 4, Kind: "a2"},
	} {
		if err := s.AppendEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := s.ListEvents(EventFilter{ChildID: ptr(a)})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range evs {
		kinds = append(kinds, e.Kind)
	}
	if want := []string{"a2", "a1"}; !reflect.DeepEqual(kinds, want) {
		t.Errorf("события ребёнка a = %v, want %v", kinds, want)
	}
}

// Ловит: событие без вида, с неупаковываемыми подробностями или для несуществующего ребёнка
// записывается и ломает журнал позже.
func TestAppendEventRejectsBadInput(t *testing.T) {
	s := openTemp(t)
	if err := s.AppendEvent(Event{AtMs: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("событие без вида = %v, want ErrInvalid", err)
	}
	if err := s.AppendEvent(Event{AtMs: 1, Kind: "x", Details: map[string]any{"ch": make(chan int)}}); err == nil {
		t.Error("событие с каналом в подробностях прошло, want ошибку")
	}
	if err := s.AppendEvent(Event{ChildID: ptr(int64(999)), AtMs: 1, Kind: "x"}); err == nil {
		t.Error("событие для ребёнка 999 прошло, want ошибку внешнего ключа")
	}
	if n := rawNull(t, s, `SELECT count(*) FROM events`); n.Int64 != 0 {
		t.Errorf("строк events = %d, want 0", n.Int64)
	}
}

// Ловит: одна испорченная строка журнала молча превращается в пустое событие или в панику.
func TestListEventsReportsBrokenDetails(t *testing.T) {
	s := openTemp(t)
	if _, err := s.db.Exec(`INSERT INTO events (at, tz_offset_min, kind, details) VALUES (1, 0, 'x', 'не json')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListEvents(EventFilter{}); err == nil {
		t.Fatal("ListEvents со сломанными подробностями прошёл, want ошибку")
	}
}

// Ловит: «пароль не задан» читается как ошибка (ErrNoRows) или как найденный пустой хеш,
// и мастер первого запуска не открывается (грабля «пусто = норма»).
func TestGetAuthNotFound(t *testing.T) {
	s := openTemp(t)
	got, found, err := s.GetAuth()
	if err != nil || found || got != (Auth{}) {
		t.Fatalf("GetAuth пустой базы = %+v, %v, %v; want Auth{}, false, nil", got, found, err)
	}
}

// Ловит: серия пауз R19 хранится не по решению оркестратора (failed_count = серия×5 + счётчик)
// или теряется при перезапуске службы; ноль в BlockedUntilMs пишется не NULL.
func TestSetGetAuthStoresSeriesInFailedCount(t *testing.T) {
	s := openTemp(t)
	in := Auth{Hash: "$argon2id$v=19$m=65536,t=3,p=1$c2FsdA$aGFzaA", Failed: 3, Series: 2, BlockedUntilMs: 9_000_000_000_000, ChangedAtMs: 7}
	if err := s.SetAuth(in); err != nil {
		t.Fatal(err)
	}
	got, found, err := s.GetAuth()
	if err != nil || !found || got != in {
		t.Fatalf("GetAuth = %+v, %v, %v; want %+v, true, nil", got, found, err, in)
	}
	if v := rawNull(t, s, `SELECT failed_count FROM parent_auth WHERE id = 1`); v.Int64 != 13 {
		t.Errorf("failed_count = %d, want 13 (2 серии × 5 + 3)", v.Int64)
	}
	if v := rawNull(t, s, `SELECT blocked_until FROM parent_auth WHERE id = 1`); v.Int64 != 9_000_000_000_000 {
		t.Errorf("blocked_until = %d, want 9000000000000", v.Int64)
	}

	second := Auth{Hash: "новый-хеш", ChangedAtMs: 8}
	if err := s.SetAuth(second); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.GetAuth()
	if got != second {
		t.Errorf("после сброса GetAuth = %+v, want %+v", got, second)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM parent_auth`); n.Int64 != 1 {
		t.Errorf("строк parent_auth = %d, want 1", n.Int64)
	}
	if v := rawNull(t, s, `SELECT blocked_until FROM parent_auth WHERE id = 1`); v.Valid {
		t.Errorf("blocked_until при 0 = %+v, want NULL", v)
	}
}

// Ловит: пустой хеш, счётчик вне 0…4 или отрицательная серия записываются и после чтения
// дают другие числа (формула серия = failed_count / 5 перестаёт быть обратимой); ноль в
// ChangedAtMs остаётся нулём вместо момента записи.
func TestSetAuthValidationAndDefaults(t *testing.T) {
	s := openTemp(t)
	for _, bad := range []Auth{
		{Hash: ""},
		{Hash: "h", Failed: 5},
		{Hash: "h", Failed: -1},
		{Hash: "h", Series: -1},
	} {
		if err := s.SetAuth(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("SetAuth(%+v) = %v, want ErrInvalid", bad, err)
		}
	}
	if _, found, _ := s.GetAuth(); found {
		t.Error("после отказов пароль найден, want не найден")
	}
	fixClock(t, 4_242)
	if err := s.SetAuth(Auth{Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.GetAuth(); got.ChangedAtMs != 4_242 {
		t.Errorf("ChangedAtMs = %d, want 4242 (момент записи)", got.ChangedAtMs)
	}
}

// Ловит: расхождение между хранилищем и политикой пауз R19 (internal/auth): состояние после
// перезапуска службы не равно состоянию до перезапуска, и пауза или серия сбиваются.
func TestAuthRoundTripMatchesAttemptsPolicy(t *testing.T) {
	s := openTemp(t)
	var a auth.Attempts
	now := int64(1_000_000)
	for i := 0; i < 12; i++ {
		a.Fail(now)
		now = max(now, a.BlockedUntilMs) + 1
	}
	if a.Series != 2 || a.Failed != 2 {
		t.Fatalf("исходное состояние: серий %d, в серии %d; want 2 и 2", a.Series, a.Failed)
	}
	if err := s.SetAuth(Auth{Hash: "h", Failed: a.Failed, Series: a.Series, BlockedUntilMs: a.BlockedUntilMs, ChangedAtMs: 1}); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.GetAuth()
	if err != nil {
		t.Fatal(err)
	}
	back := auth.Attempts{Failed: got.Failed, Series: got.Series, BlockedUntilMs: got.BlockedUntilMs}
	if back != a {
		t.Fatalf("после чтения %+v, want %+v", back, a)
	}
	if v := rawNull(t, s, `SELECT failed_count FROM parent_auth WHERE id = 1`); int(v.Int64) != a.FailedInRow() {
		t.Errorf("failed_count = %d, want %d", v.Int64, a.FailedInRow())
	}
}

// Ловит: настройка не находится после записи, перезапись создаёт вторую строку, пустое
// значение путается с «нет настройки», русский текст портится, пустой ключ принимается.
func TestSettings(t *testing.T) {
	s := openTemp(t)
	if v, ok, err := s.GetSetting("diag.verbose"); v != "" || ok || err != nil {
		t.Fatalf("GetSetting без записи = %q, %v, %v; want \"\", false, nil", v, ok, err)
	}
	if err := s.SetSetting("diag.verbose", `{"подробно":"да"}`); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := s.GetSetting("diag.verbose"); v != `{"подробно":"да"}` || !ok || err != nil {
		t.Fatalf("GetSetting = %q, %v, %v", v, ok, err)
	}
	if err := s.SetSetting("diag.verbose", ""); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := s.GetSetting("diag.verbose"); v != "" || !ok || err != nil {
		t.Fatalf("пустое значение: %q, %v, %v; want \"\", true, nil", v, ok, err)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM settings`); n.Int64 != 1 {
		t.Errorf("строк settings = %d, want 1", n.Int64)
	}
	if err := s.SetSetting("", "x"); !errors.Is(err, ErrInvalid) {
		t.Errorf("SetSetting с пустым ключом = %v, want ErrInvalid", err)
	}
}

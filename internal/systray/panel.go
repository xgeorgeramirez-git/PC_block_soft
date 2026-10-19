package systray

import "pcblock/internal/ipc"

// PanelKind — что делает кнопка мини-панели.
type PanelKind int

const (
	PanelMyStats PanelKind = iota // открыть «Мою статистику»
	PanelParent                   // открыть окно родителя («Настройки для родителя»)
	PanelRefresh                  // повторить запрос к службе (урок L-114)
)

// PanelButton — кнопка мини-панели.
type PanelButton struct {
	Kind  PanelKind
	Label string
	Main  bool // главная кнопка панели (графитовая)
	Small bool // мелкая кнопка внизу, без рамки (ТЗ 8.2.1 п.3)
}

// PanelBar — полоса «остаток из лимита».
type PanelBar struct {
	Fraction float64 // доля остатка от лимита дня, от 0 до 1
	Status   Status  // цвет полосы: тот же, что у значка
}

// PanelView — что показать в мини-панели (ТЗ 8.2.1 п.3-5). Окно только рисует это.
type PanelView struct {
	Title   string
	Bar     *PanelBar // nil: полосы нет (без лимита, ошибка, учётка не ребёнка)
	Note    string    // итог последнего нажатия, если не получилось
	Buttons []PanelButton
	Error   bool // службе не удалось ответить
}

// BuildPanel собирает мини-панель из состояния и итога последнего нажатия (note — готовый
// текст ошибки или пустая строка).
func BuildPanel(s Snapshot, note string) PanelView {
	notChild := s.Has && s.Status.Status == ipc.StatusNotChild
	switch {
	case s.Failed || !s.Has || !s.Fresh:
		title := textPanelError
		if notChild {
			title = textPanelErrorParent
		}
		return PanelView{
			Title:   title,
			Note:    note,
			Error:   true,
			Buttons: []PanelButton{{Kind: PanelRefresh, Label: textButtonRefresh, Main: true}},
		}
	case notChild:
		return PanelView{
			Title:   textPanelNotChild,
			Note:    note,
			Buttons: []PanelButton{{Kind: PanelParent, Label: textButtonParent, Main: true}},
		}
	}
	st := s.Status
	title := st.Summary
	if title == "" {
		title = st.Tooltip
	}
	v := PanelView{
		Title: title,
		Note:  note,
		Buttons: []PanelButton{
			{Kind: PanelMyStats, Label: textButtonMyStats, Main: true},
			{Kind: PanelParent, Label: textButtonParent, Small: true},
		},
	}
	if !st.Unlimited && st.LimitS > 0 && !isPaused(st) {
		d := DisplayFor(st)
		v.Bar = &PanelBar{Fraction: remainingFraction(st.RemainingS, st.LimitS), Status: d.Status}
	}
	return v
}

// remainingFraction — доля остатка от лимита, ограниченная отрезком 0…1.
func remainingFraction(remainingS, limitS int) float64 {
	if limitS <= 0 || remainingS <= 0 {
		return 0
	}
	if remainingS >= limitS {
		return 1
	}
	return float64(remainingS) / float64(limitS)
}

// OpenKind — вид окна для команды службы OpenWindow; ok=false у кнопки, которая окно не открывает.
func (k PanelKind) OpenKind() (kind string, ok bool) {
	switch k {
	case PanelMyStats:
		return ipc.WindowMyStats, true
	case PanelParent:
		return ipc.WindowParent, true
	}
	return "", false
}

// OpenFailedText — что сказать, когда окно открыть не удалось: текст службы, если она дала
// человеческий ответ, иначе общий; родителю на «вы», ребёнку на «ты».
func OpenFailedText(serviceText string, notChild bool) string {
	if serviceText != "" {
		return serviceText
	}
	if notChild {
		return textOpenFailedParent
	}
	return textOpenFailed
}

// Rect — прямоугольник в экранных координатах Windows.
type Rect struct{ Left, Top, Right, Bottom int }

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// panelGap — зазор между панелью и значком и между панелью и краем рабочей области, px.
const panelGap = 8

// PlacePanel выбирает левый верхний угол панели размером w на h: по центру над значком, если
// панель задач снизу (значок ниже середины рабочей области), иначе под ним; целиком внутри
// рабочей области work. Если панель больше области, она прижимается к левому верхнему углу.
func PlacePanel(anchor, work Rect, w, h int) (x, y int) {
	cx := (anchor.Left + anchor.Right) / 2
	x = clampInt(cx-w/2, work.Left+panelGap, work.Right-w-panelGap)
	mid := (work.Top + work.Bottom) / 2
	if (anchor.Top+anchor.Bottom)/2 > mid {
		y = anchor.Top - h - panelGap
	} else {
		y = anchor.Bottom + panelGap
	}
	y = clampInt(y, work.Top+panelGap, work.Bottom-h-panelGap)
	return x, y
}

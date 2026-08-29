package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tools/interaction"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// questionnaireState walks through questions → review → submit.
type questionnaireState int

const (
	qStateSelecting questionnaireState = iota // picking an option / typing custom
	qStateCustom                              // free-text entry for one question
	qStateReview                              // review all answers before send
)

// Questionnaire renders a structured ask_user flow: multiple questions,
// multiple-choice options with custom-answer fallback, and an explicit
// review/send step. Styled like the Confirm panel.
type Questionnaire struct {
	styles   *themes.Styles
	visible  bool
	width    int
	request  interaction.QuestionnaireRequest
	answers  []string
	state    questionnaireState
	current  int // question index being answered / reviewed
	cursor   int // selected option index
	custom   textinput.Model
	onSubmit func(formatted string)
	onCancel func()
}

// NewQuestionnaire creates the component.
func NewQuestionnaire(styles *themes.Styles) *Questionnaire {
	ti := textinput.New()
	ti.Placeholder = "Type your answer…"
	return &Questionnaire{
		styles: styles,
		custom: ti,
	}
}

// SetSize updates dimensions.
func (q *Questionnaire) SetSize(w, _ int) {
	q.width = w
	fw := w - 8
	if fw < 20 {
		fw = 20
	}
	if fw > 80 {
		fw = 80
	}
	q.custom.SetWidth(fw)
}

// Visible reports whether the questionnaire is showing.
func (q *Questionnaire) Visible() bool { return q.visible }

// Begin starts a session; onSubmit receives the formatted Q/A transcript,
// onCancel fires when the user dismisses everything.
func (q *Questionnaire) Begin(req interaction.QuestionnaireRequest, onSubmit func(string), onCancel func()) bool {
	if len(req.Questions) == 0 {
		return false
	}
	for i := range req.Questions {
		if len(req.Questions[i].Options) == 0 {
			req.Questions[i].AllowCustom = true // otherwise unanswerable
		}
	}
	q.request = req
	q.answers = make([]string, len(req.Questions))
	q.state = qStateSelecting
	q.current = 0
	q.cursor = 0
	q.onSubmit = onSubmit
	q.onCancel = onCancel
	q.custom.Reset()
	q.custom.Blur()
	if q.width == 0 {
		q.width = 80
	}
	q.visible = true
	return true
}

// Cancel dismisses an active questionnaire without invoking callbacks.
func (q *Questionnaire) Cancel() {
	q.visible = false
}

// Update handles keys while visible.
func (q *Questionnaire) Update(msg tea.Msg) {
	if !q.visible {
		return
	}
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch q.state {
		case qStateSelecting:
			q.handleSelecting(m.String())
		case qStateCustom:
			q.handleCustom(m)
		case qStateReview:
			q.handleReview(m.String())
		}
	default:
		if q.state == qStateCustom {
			inp, _ := q.custom.Update(msg)
			q.custom = inp
		}
	}
}

func (q *Questionnaire) handleSelecting(key string) {
	qi := &q.request.Questions[q.current]
	switch key {
	case "up", "k":
		if q.cursor > 0 {
			q.cursor--
		}
	case "down", "j", "tab":
		max := len(qi.Options) // last row = custom option when allowed
		if !qi.AllowCustom || len(qi.Options) == 0 {
			max--
			if max < 0 {
				max = 0
			}
		}
		if q.cursor < max {
			q.cursor++
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(key[0] - '0')
		if n >= 1 && n <= len(qi.Options) {
			q.cursor = n - 1
			q.pick(qi.Options[n-1])
		}
	case "c", "C", "i":
		if qi.AllowCustom || len(qi.Options) == 0 {
			q.enterCustom()
		}
	case "enter":
		hasCustomRow := qi.AllowCustom || len(qi.Options) == 0
		if q.cursor < len(qi.Options) {
			q.pick(qi.Options[q.cursor])
		} else if hasCustomRow {
			q.enterCustom()
		}
	case "esc":
		q.cancel()
	}
}

func (q *Questionnaire) pick(answer string) {
	q.answers[q.current] = answer
	if q.current+1 < len(q.request.Questions) {
		q.current++
		q.cursor = 0
		return
	}
	q.state = qStateReview
}

func (q *Questionnaire) enterCustom() {
	q.state = qStateCustom
	q.custom.SetValue(q.answers[q.current])
	q.custom.Focus()
	q.custom.CursorEnd()
}

func (q *Questionnaire) handleCustom(m tea.KeyMsg) {
	switch m.String() {
	case "enter":
		val := strings.TrimSpace(q.custom.Value())
		q.answers[q.current] = val
		q.custom.Blur()
		if q.current+1 < len(q.request.Questions) {
			q.current++
			q.cursor = 0
			q.state = qStateSelecting
		} else {
			q.state = qStateReview
		}
	case "esc":
		q.custom.Blur()
		q.state = qStateSelecting
	default:
		inp, _ := q.custom.Update(m)
		q.custom = inp
	}
}

func (q *Questionnaire) handleReview(key string) {
	switch key {
	case "up", "k":
		if q.current > 0 {
			q.current--
		}
	case "down", "j":
		if q.current < len(q.request.Questions)-1 {
			q.current++
		}
	case "enter", "e", "E":
		q.cursor = 0
		q.state = qStateSelecting
	case "s", "S", "enter_submit":
		q.submit()
	case "esc":
		q.cancel()
	}
}

func (q *Questionnaire) submit() {
	var b strings.Builder
	for i, question := range q.request.Questions {
		ans := q.answers[i]
		if ans == "" {
			ans = "(skipped)"
		}
		fmt.Fprintf(&b, "Q: %s\nA: %s\n", question.Text, ans)
	}
	out := strings.TrimRight(b.String(), "\n")
	q.visible = false
	if q.onSubmit != nil {
		q.onSubmit(out)
	}
}

func (q *Questionnaire) cancel() {
	q.visible = false
	if q.onCancel != nil {
		q.onCancel()
	}
}

// View renders the inline panel, styled like the confirmation card.
func (q *Questionnaire) View() string {
	if !q.visible {
		return ""
	}
	var lines []string
	total := len(q.request.Questions)

	title := lipgloss.NewStyle().Foreground(q.styles.T.Accent).Bold(true).
		Render(fmt.Sprintf("▸ Question %d of %d", q.current+1, total))

	switch q.state {
	case qStateSelecting:
		lines = append(lines, title)
		qi := &q.request.Questions[q.current]
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(wrap(qi.Text, q.width-6)))
		for i, opt := range qi.Options {
			row := fmt.Sprintf("  %d. %s", i+1, opt)
			if i == q.cursor {
				row = lipgloss.NewStyle().Foreground(q.styles.T.AccentAlt).Render("▸ ") + lipgloss.NewStyle().Bold(true).Render(row)
			} else {
				row = "  " + row
			}
			lines = append(lines, row)
		}
		if qi.AllowCustom || len(qi.Options) == 0 {
			row := "  c. Type your own answer"
			last := len(qi.Options)
			if len(qi.Options) == 0 || q.cursor == last {
				row = lipgloss.NewStyle().Foreground(q.styles.T.AccentAlt).Render("▸ ") + lipgloss.NewStyle().Bold(true).Render(strings.TrimLeft(row, " "))
			}
			lines = append(lines, row)
		}
		lines = append(lines, q.hint("↑↓ choose · 1-9 quick-pick · enter select · esc dismiss"))

	case qStateCustom:
		lines = append(lines, title)
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(wrap(q.request.Questions[q.current].Text, q.width-6)))
		lines = append(lines, q.custom.View())
		lines = append(lines, q.hint("enter save answer · esc back"))

	case qStateReview:
		lines = append(lines, title+" — Review & send")
		for i, question := range q.request.Questions {
			marker := "  "
			if i == q.current {
				marker = lipgloss.NewStyle().Foreground(q.styles.T.AccentAlt).Render("▸ ")
			}
			answer := q.answers[i]
			if answer == "" {
				answer = lipgloss.NewStyle().Foreground(q.styles.T.Muted).Render("(empty)")
			}
			lines = append(lines, marker+lipgloss.NewStyle().Foreground(q.styles.T.Muted).Render(truncate(question.Text, q.width-10)))
			lines = append(lines, "      "+lipgloss.NewStyle().Foreground(q.styles.T.Green).Bold(true).Render(truncate("→ "+answer, q.width-12)))
		}
		lines = append(lines, "")
		lines = append(lines, q.hint("[s] send  ·  [e]/enter edit highlighted  ·  ↑↓ move  ·  esc dismiss"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	w := q.width
	if w <= 0 {
		w = lipgloss.Width(content) + 4
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(q.styles.T.BorderFocused).
		Padding(0, 1).
		Width(w - 2).
		Render(content)
}

func (q *Questionnaire) hint(s string) string {
	return lipgloss.NewStyle().Foreground(q.styles.T.Muted).Render(s)
}

// wrap hard-wraps text to width (plain-text safe).
func wrap(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > width {
				out = append(out, line)
				line = word
				continue
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func truncate(s string, width int) string {
	if width < 4 {
		width = 4
	}
	return ansi.Truncate(s, width, "…")
}

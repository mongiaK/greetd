package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ───────────────────────── 主题：Catppuccin Mocha ─────────────────────────

var (
	cBase     = lipgloss.Color("#1e1e2e")
	cSurface0 = lipgloss.Color("#313244")
	cSurface2 = lipgloss.Color("#585b70")
	cOverlay  = lipgloss.Color("#6c7086")
	cSubtext  = lipgloss.Color("#a6adc8")
	cText     = lipgloss.Color("#cdd6f4")
	cLavender = lipgloss.Color("#b4befe")
	cMauve    = lipgloss.Color("#cba6f7")
	cPink     = lipgloss.Color("#f5c2e7")
	cRed      = lipgloss.Color("#f38ba8")
	cPeach    = lipgloss.Color("#fab387")
	cYellow   = lipgloss.Color("#f9e2af")
	cGreen    = lipgloss.Color("#a6e3a1")
	cTeal     = lipgloss.Color("#94e2d5")
	cBlue     = lipgloss.Color("#89b4fa")
)

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// ───────────────────────── 大号时钟（3x5 点阵，横向放大 2 倍） ─────────────────────────

var glyphs = map[rune][5]string{
	'0': {"███", "█ █", "█ █", "█ █", "███"},
	'1': {" █ ", "██ ", " █ ", " █ ", "███"},
	'2': {"███", "  █", "███", "█  ", "███"},
	'3': {"███", "  █", "███", "  █", "███"},
	'4': {"█ █", "█ █", "███", "  █", "  █"},
	'5': {"███", "█  ", "███", "  █", "███"},
	'6': {"███", "█  ", "███", "█ █", "███"},
	'7': {"███", "  █", "  █", "  █", "  █"},
	'8': {"███", "█ █", "███", "█ █", "███"},
	'9': {"███", "█ █", "███", "  █", "███"},
	':': {" ", "█", " ", "█", " "},
}

func scaleX(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c == '█' {
			b.WriteString("██")
		} else {
			b.WriteString("  ")
		}
	}
	return b.String()
}

func bigClock(t time.Time) string {
	var rows [5]string
	for i, r := range t.Format("15:04") {
		g := glyphs[r]
		for y := 0; y < 5; y++ {
			if i > 0 {
				rows[y] += "  "
			}
			rows[y] += scaleX(g[y])
		}
	}
	return fg(cMauve).Render(strings.Join(rows[:], "\n"))
}

var weekdays = [...]string{"日", "一", "二", "三", "四", "五", "六"}

func dateLine(t time.Time) string {
	return fmt.Sprintf("%d 年 %d 月 %d 日 · 星期%s", t.Year(), int(t.Month()), t.Day(), weekdays[t.Weekday()])
}

// ───────────────────────── Nerd Font 图标 ─────────────────────────

const (
	icoLaptop = "\uf109"
	icoWifi   = "\uf1eb"
	icoBolt   = "\uf0e7"
	icoLock   = "\uf023"
	icoPower  = "\uf011"
	icoReboot = "\uf021"
	icoMoon   = "\uf186"
)

func batteryIcon(pct int, charging bool) string {
	switch {
	case charging:
		return icoBolt
	case pct >= 88:
		return "\uf240"
	case pct >= 63:
		return "\uf241"
	case pct >= 38:
		return "\uf242"
	case pct >= 13:
		return "\uf243"
	default:
		return "\uf244"
	}
}

// ───────────────────────── Model ─────────────────────────

type (
	tickMsg      time.Time
	infoMsg      sysInfo
	refreshMsg   struct{}
	loginDoneMsg struct{ err error }
)

type model struct {
	w, h      int
	now       time.Time
	info      sysInfo
	user      textinput.Model
	pass      textinput.Model
	focus     int // 0 = 用户名, 1 = 密码
	sessions  []Session
	sel       int
	busy      bool
	errMsg    string
	statePath string
	demo      bool // 没有 GREETD_SOCK 时进入演示模式，方便在普通终端里调 UI
}

func newInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.Width = width
	ti.TextStyle = fg(cText)
	ti.PlaceholderStyle = fg(cOverlay)
	ti.Cursor.Style = fg(cPink)
	return ti
}

func newModel(statePath string) *model {
	st := loadState(statePath)
	m := &model{
		now:       time.Now(),
		sessions:  loadSessions(),
		statePath: statePath,
		demo:      os.Getenv("GREETD_SOCK") == "",
	}
	for i, s := range m.sessions {
		if s.Name == st.Session {
			m.sel = i
		}
	}

	m.user = newInput("用户名", 24)
	m.pass = newInput("密码", 28)
	m.pass.EchoMode = textinput.EchoPassword
	m.pass.EchoCharacter = '●'

	user := st.User
	if user == "" && m.demo {
		user = os.Getenv("USER")
	}
	m.user.SetValue(user)
	if user != "" {
		m.focus = 1
	}
	return m
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchInfo() tea.Msg { return infoMsg(readSysInfo()) }

func (m *model) setFocus(i int) tea.Cmd {
	m.focus = i
	if i == 0 {
		m.pass.Blur()
		return m.user.Focus()
	}
	m.user.Blur()
	return m.pass.Focus()
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.setFocus(m.focus), tick(), fetchInfo)
}

func loginCmd(user, pass string, s Session, demo bool) tea.Cmd {
	return func() tea.Msg {
		if demo {
			time.Sleep(700 * time.Millisecond)
			if pass == "demo" {
				return loginDoneMsg{}
			}
			return loginDoneMsg{err: ErrAuth}
		}
		return loginDoneMsg{err: login(user, pass, s.Exec, []string{"XDG_SESSION_TYPE=wayland"})}
	}
}

func (m *model) power(action string) tea.Cmd {
	if m.demo {
		m.errMsg = "demo: systemctl " + action
		return nil
	}
	return func() tea.Msg {
		_ = exec.Command("systemctl", action).Run()
		return nil
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()

	case infoMsg:
		m.info = sysInfo(msg)
		return m, tea.Tick(30*time.Second, func(time.Time) tea.Msg { return refreshMsg{} })

	case refreshMsg:
		return m, fetchInfo

	case loginDoneMsg:
		m.busy = false
		if msg.err == nil {
			saveState(m.statePath, state{
				User:    strings.TrimSpace(m.user.Value()),
				Session: m.sessions[m.sel].Name,
			})
			return m, tea.Quit // greeter 退出后 greetd 才会真正启动会话
		}
		m.pass.SetValue("")
		if errors.Is(msg.err, ErrAuth) {
			m.errMsg = "密码错误，请重试"
		} else {
			m.errMsg = msg.err.Error()
		}
		return m, m.setFocus(1)

	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		n := len(m.sessions)
		switch msg.String() {
		case "ctrl+c":
			if m.demo {
				return m, tea.Quit
			}
			return m, nil
		case "tab":
			m.sel = (m.sel + 1) % n
			return m, nil
		case "shift+tab":
			m.sel = (m.sel - 1 + n) % n
			return m, nil
		case "up":
			return m, m.setFocus(0)
		case "down":
			return m, m.setFocus(1)
		case "enter":
			user := strings.TrimSpace(m.user.Value())
			if m.focus == 0 || user == "" {
				if user == "" {
					return m, m.setFocus(0)
				}
				return m, m.setFocus(1)
			}
			m.busy, m.errMsg = true, ""
			return m, loginCmd(user, m.pass.Value(), m.sessions[m.sel], m.demo)
		case "f10":
			return m, m.power("suspend")
		case "f11":
			return m, m.power("reboot")
		case "f12":
			return m, m.power("poweroff")
		}
		m.errMsg = ""
	}

	// 其余消息（按键、光标闪烁）交给当前聚焦的输入框
	var cmd tea.Cmd
	if m.focus == 0 {
		m.user, cmd = m.user.Update(msg)
	} else {
		m.pass, cmd = m.pass.Update(msg)
	}
	return m, cmd
}

// ───────────────────────── View ─────────────────────────

func (m *model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	bodyH := m.h - 2
	if bodyH < 1 {
		bodyH = 1
	}
	body := lipgloss.Place(m.w, bodyH, lipgloss.Center, lipgloss.Center, m.center())
	return lipgloss.JoinVertical(lipgloss.Left, m.topBar(), body, m.bottomBar())
}

func (m *model) topBar() string {
	left := fg(cSubtext).Render(icoLaptop + "  " + m.info.host)

	var parts []string
	if m.info.ssid != "" {
		parts = append(parts, fg(cTeal).Render(icoWifi+"  "+m.info.ssid))
	}
	if m.info.hasBat {
		c := cGreen
		if m.info.bat < 15 && !m.info.charging {
			c = cRed
		}
		parts = append(parts, fg(c).Render(fmt.Sprintf("%s  %d%%", batteryIcon(m.info.bat, m.info.charging), m.info.bat)))
	}
	right := strings.Join(parts, "    ")

	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	return "  " + left + strings.Repeat(" ", gap) + right + "  "
}

func (m *model) center() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		bigClock(m.now),
		fg(cSubtext).Render(dateLine(m.now)),
		"",
		m.card(),
		"",
		m.sessionRow(),
		"",
		m.statusLine(),
	)
}

func initial(s string) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) == 0 {
		return "?"
	}
	return string(unicode.ToUpper(rs[0]))
}

func (m *model) card() string {
	const cw = 36 // 卡片内容宽度

	// 头像：3 行高的色块，首字母在中间一行，与用户名同高
	avatar := lipgloss.NewStyle().
		Background(cPink).Foreground(cBase).Bold(true).
		Padding(1, 2).Render(initial(m.user.Value()))
	head := lipgloss.JoinHorizontal(lipgloss.Top,
		avatar, "  ",
		lipgloss.JoinVertical(lipgloss.Left, "", m.user.View(), fg(cOverlay).Render(m.info.host)),
	)

	bc := cSurface2
	if m.focus == 1 && !m.busy {
		bc = cMauve
	}
	passRow := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(bc).
		Padding(0, 1).Width(cw - 2).
		Render(fg(cMauve).Render(icoLock) + "  " + m.pass.View())

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(cSurface2).
		Padding(1, 2).Width(cw + 4).
		Render(lipgloss.JoinVertical(lipgloss.Left, head, "", passRow))
}

func (m *model) sessionRow() string {
	on := lipgloss.NewStyle().Background(cMauve).Foreground(cBase).Padding(0, 1)
	off := lipgloss.NewStyle().Background(cSurface0).Foreground(cSubtext).Padding(0, 1)
	parts := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		if i == m.sel {
			parts[i] = on.Render(s.Name)
		} else {
			parts[i] = off.Render(s.Name)
		}
	}
	return strings.Join(parts, " ")
}

func (m *model) statusLine() string {
	switch {
	case m.busy:
		return fg(cYellow).Render("验证中…")
	case m.errMsg != "":
		return fg(cRed).Render(m.errMsg)
	default:
		return " "
	}
}

func hint(k, label string) string {
	key := lipgloss.NewStyle().Background(cLavender).Foreground(cBase).Padding(0, 1).Render(k)
	return key + " " + fg(cSubtext).Render(label)
}

func (m *model) bottomBar() string {
	const sep = "   "
	nav := strings.Join([]string{hint("Tab", "会话"), hint("↑↓", "切换"), hint("Enter", "登录")}, sep)
	pow := strings.Join([]string{
		fg(cBlue).Render(icoMoon) + " " + hint("F10", "睡眠"),
		fg(cPeach).Render(icoReboot) + " " + hint("F11", "重启"),
		fg(cRed).Render(icoPower) + " " + hint("F12", "关机"),
	}, sep)
	return lipgloss.PlaceHorizontal(m.w, lipgloss.Center, nav+"      "+pow)
}

// ───────────────────────── main ─────────────────────────

func main() {
	statePath := flag.String("state", "/var/cache/mgreeter/state.json", "记住上次用户/会话的文件")
	flag.Parse()

	// foot 的 TERM 不含 "color"，lipgloss 可能会降级到 16 色；这里显式声明真彩色。
	if os.Getenv("COLORTERM") == "" {
		os.Setenv("COLORTERM", "truecolor")
	}

	p := tea.NewProgram(newModel(*statePath), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "greeter:", err)
		os.Exit(1)
	}
}

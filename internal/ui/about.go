package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnDovey/wshare/internal/about"
)

const (
	aboutMaxWidth = 48
	aboutAnimStep = 8
)

type aboutAnimMsg time.Time

func aboutAnimCmd() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg { return aboutAnimMsg(t) })
}

func (m Model) updateAbout(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		return m.closeAbout()
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k", "pgup", "ctrl+u":
		m.aboutView.HalfViewUp()
	case "down", "j", "pgdown", "ctrl+d":
		m.aboutView.HalfViewDown()
	}
	var cmd tea.Cmd
	m.aboutView, cmd = m.aboutView.Update(msg)
	return m, cmd
}

func (m Model) openAbout() (tea.Model, tea.Cmd) {
	m.showAbout = true
	m.aboutClosing = false
	m.aboutWidth = 0
	m.layoutAbout()
	m.refreshAboutContent()
	m.aboutView.GotoTop()
	return m, aboutAnimCmd()
}

func (m *Model) refreshAboutContent() {
	w := m.aboutView.Width
	if w < 20 {
		w = aboutMaxWidth - 4
	}
	m.aboutView.SetContent(about.PlainText(about.AppAdmin, w))
}

func (m Model) closeAbout() (tea.Model, tea.Cmd) {
	if !m.showAbout {
		return m, nil
	}
	m.aboutClosing = true
	return m, aboutAnimCmd()
}

func (m *Model) layoutAbout() {
	h := m.height - 4
	if h < 8 {
		h = 8
	}
	w := aboutMaxWidth - 2
	if w < 20 {
		w = 20
	}
	m.aboutView.Width = w
	m.aboutView.Height = h - 2
}

func (m Model) tickAboutAnim() (tea.Model, tea.Cmd) {
	if !m.showAbout {
		return m, nil
	}
	if m.aboutClosing {
		m.aboutWidth -= aboutAnimStep
		if m.aboutWidth <= 0 {
			m.aboutWidth = 0
			m.showAbout = false
			m.aboutClosing = false
			return m, nil
		}
		return m, aboutAnimCmd()
	}
	if m.aboutWidth < aboutMaxWidth {
		m.aboutWidth += aboutAnimStep
		if m.aboutWidth > aboutMaxWidth {
			m.aboutWidth = aboutMaxWidth
		}
		return m, aboutAnimCmd()
	}
	return m, nil
}

func (m Model) renderAboutDrawer() string {
	h := m.height - 4
	if h < 8 {
		h = 8
	}
	w := m.aboutWidth
	if w < 1 {
		return ""
	}
	if w > aboutMaxWidth {
		w = aboutMaxWidth
	}

	inner := panelTitleStyle.Render("About") + "\n" + m.aboutView.View()
	// Clip drawer width while animating open/closed.
	panel := panelStyle.Width(aboutMaxWidth).Height(h).Render(inner)
	if w >= aboutMaxWidth {
		return panel
	}
	// Rough left-edge reveal while sliding in.
	lines := strings.Split(panel, "\n")
	var clipped []string
	for _, line := range lines {
		plain := stripRough(line)
		if len([]rune(plain)) <= w {
			clipped = append(clipped, line)
			continue
		}
		// Keep ANSI by taking runes from the end (content slides from left).
		// Fallback: pad/truncate visually via lipgloss.
		clipped = append(clipped, lipgloss.NewStyle().MaxWidth(w).Render(line))
	}
	return strings.Join(clipped, "\n")
}

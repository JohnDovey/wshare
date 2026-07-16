package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnDovey/wshare/internal/scanner"
	"github.com/JohnDovey/wshare/internal/store"
)

const leftWidth = 34

// listItem is a navigable row in the left panel (a share root).
type listItem struct {
	Root    store.Root
	OK      int
	Missing int
	Total   int
}

// Model is the root bubbletea model (ServiceMonitor-style layout).
type Model struct {
	dbPath string
	store  *store.Store

	items   []listItem
	entries []store.Entry // all entries; filtered in detail view
	cursor  int
	width   int
	height  int
	ready   bool

	detail viewport.Model
	flash  string
	flashAt time.Time

	// Add-path overlay
	adding bool
	input  textinput.Model

	// Confirm remove root
	confirmRemove bool

	// About drawer (slides in from the left)
	showAbout     bool
	aboutClosing  bool
	aboutWidth    int
	aboutView     viewport.Model
}

type tickMsg time.Time

type refreshMsg struct {
	items   []listItem
	entries []store.Entry
	err     error
}

type actionDoneMsg struct {
	err error
	msg string
}

// New constructs the admin UI model.
func New(dbPath string, st *store.Store) Model {
	ti := textinput.New()
	ti.Placeholder = "path to file or folder…"
	ti.CharLimit = 512
	ti.Width = 48

	return Model{
		dbPath:    dbPath,
		store:     st,
		detail:    viewport.New(80, 20),
		aboutView: viewport.New(aboutMaxWidth-2, 20),
		input:     ti,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refreshCmd() tea.Cmd {
	st := m.store
	return func() tea.Msg {
		roots, err := st.ListRoots()
		if err != nil {
			return refreshMsg{err: err}
		}
		entries, err := st.ListEntries(0)
		if err != nil {
			return refreshMsg{err: err}
		}
		store.MarkAvailability(entries)

		items := make([]listItem, 0, len(roots))
		for _, r := range roots {
			it := listItem{Root: r}
			for _, e := range entries {
				if e.RootID != r.ID {
					continue
				}
				it.Total++
				if e.Available {
					it.OK++
				} else {
					it.Missing++
				}
			}
			items = append(items, it)
		}
		return refreshMsg{items: items, entries: entries}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutDetail()
		m.layoutAbout()
		m.ready = true
		m.detail.SetContent(m.renderDetail())
		if m.showAbout {
			m.refreshAboutContent()
		}
		return m, nil

	case aboutAnimMsg:
		return m.tickAboutAnim()

	case tickMsg:
		if time.Since(m.flashAt) > 4*time.Second {
			m.flash = ""
		}
		return m, tickCmd()

	case refreshMsg:
		if msg.err != nil {
			m.flash = errStyle.Render("✗ " + msg.err.Error())
			m.flashAt = time.Now()
			return m, nil
		}
		m.items = msg.items
		m.entries = msg.entries
		if m.cursor >= len(m.items) && m.cursor > 0 {
			m.cursor = len(m.items) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.detail.SetContent(m.renderDetail())
		return m, nil

	case actionDoneMsg:
		if msg.err != nil {
			m.flash = errStyle.Render("✗ " + msg.err.Error())
		} else {
			m.flash = upStyle.Render("✓ " + msg.msg)
		}
		m.flashAt = time.Now()
		m.adding = false
		m.confirmRemove = false
		m.input.Blur()
		return m, m.refreshCmd()

	case tea.KeyMsg:
		if m.showAbout {
			return m.updateAbout(msg)
		}
		if m.adding {
			return m.updateAdding(msg)
		}
		if m.confirmRemove {
			return m.updateConfirm(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			return m.openAbout()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.detail.GotoTop()
				m.detail.SetContent(m.renderDetail())
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.detail.GotoTop()
				m.detail.SetContent(m.renderDetail())
			}
		case "a":
			m.adding = true
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case "d", "x":
			if len(m.items) == 0 {
				m.flash = errStyle.Render("✗ nothing to remove")
				m.flashAt = time.Now()
				return m, nil
			}
			m.confirmRemove = true
			return m, nil
		case "c":
			return m, m.cleanupCmd()
		case "r":
			return m, m.refreshCmd()
		case "pgup", "ctrl+u":
			m.detail.HalfViewUp()
		case "pgdown", "ctrl+d":
			m.detail.HalfViewDown()
		}
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model) updateAdding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.adding = false
		m.input.Blur()
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.input.Value())
		m.adding = false
		m.input.Blur()
		if path == "" {
			return m, nil
		}
		return m, m.addCmd(path)
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirmRemove = false
		return m, m.removeSelectedCmd()
	case "n", "N", "esc", "q":
		m.confirmRemove = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) addCmd(path string) tea.Cmd {
	st := m.store
	return func() tea.Msg {
		if home, err := os.UserHomeDir(); err == nil {
			switch {
			case path == "~":
				path = home
			case strings.HasPrefix(path, "~/"), strings.HasPrefix(path, `~\`):
				path = filepath.Join(home, path[2:])
			}
		}
		res, err := scanner.AddPath(st, path)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{msg: fmt.Sprintf("added %s (%d files, %d dirs, %d skipped)",
			filepath.Base(res.RootPath), res.FilesAdded, res.DirsAdded, res.Skipped)}
	}
}

func (m Model) removeSelectedCmd() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	id := m.items[m.cursor].Root.ID
	name := m.items[m.cursor].Root.Name
	st := m.store
	return func() tea.Msg {
		if err := st.RemoveRoot(id); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{msg: "removed " + name}
	}
}

func (m Model) cleanupCmd() tea.Cmd {
	st := m.store
	return func() tea.Msg {
		n, err := st.CleanupMissing()
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{msg: fmt.Sprintf("cleaned up %d missing entr(y/ies)", n)}
	}
}

func (m *Model) layoutDetail() {
	rightW := m.width - leftWidth - 4
	if rightW < 40 {
		rightW = 40
	}
	h := m.height - 4
	if h < 8 {
		h = 8
	}
	m.detail.Width = rightW - 2
	m.detail.Height = h - 2
	m.input.Width = min(m.width-10, 64)
}

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}

	header := titleStyle.Render(" wShare Admin ") + " " + dimStyle.Render(m.dbPath)
	left := m.renderLeft()
	right := m.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	// About slides out from the left over the main layout.
	if m.showAbout && m.aboutWidth > 0 {
		drawer := m.renderAboutDrawer()
		// Dim the main body slightly by placing drawer first (covers left).
		restW := m.width - m.aboutWidth - 1
		if restW < 10 {
			restW = 10
		}
		// Show truncated main UI to the right of the drawer.
		mainClip := lipgloss.NewStyle().MaxWidth(restW).Render(body)
		body = lipgloss.JoinHorizontal(lipgloss.Top, drawer, " ", mainClip)
	}

	help := helpStyle.Render(
		keyStyle.Render("↑↓") + " select  " +
			keyStyle.Render("a") + " add  " +
			keyStyle.Render("d") + " remove  " +
			keyStyle.Render("c") + " cleanup  " +
			keyStyle.Render("r") + " refresh  " +
			keyStyle.Render("?") + " about  " +
			keyStyle.Render("q") + " quit",
	)
	flash := m.flash
	if flash != "" {
		flash = "  " + flash
	}

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, help+flash)

	if m.adding {
		box := panelStyle.Width(min(m.width-4, 72)).Render(
			panelTitleStyle.Render("Add file or folder") + "\n\n" +
				m.input.View() + "\n\n" +
				dimStyle.Render("enter confirm · esc cancel"),
		)
		return lipgloss.JoinVertical(lipgloss.Left, view, "", box)
	}
	if m.confirmRemove {
		name := "?"
		if m.cursor >= 0 && m.cursor < len(m.items) {
			name = m.items[m.cursor].Root.Name
		}
		box := panelStyle.Width(min(m.width-4, 72)).Render(
			missStyle.Render("Remove root “"+name+"” and all its entries?") + "\n\n" +
				keyStyle.Render("y") + " yes   " + keyStyle.Render("n") + " no",
		)
		return lipgloss.JoinVertical(lipgloss.Left, view, "", box)
	}
	return view
}

func (m Model) renderLeft() string {
	h := m.height - 4
	if h < 8 {
		h = 8
	}
	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("Shares") + "\n\n")
	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("No shares yet.\nPress a to add a path."))
	}
	for i, it := range m.items {
		badge := statusBadge(it)
		label := fmt.Sprintf("%s %s", badge, it.Root.Name)
		line := normalItemStyle.Render(padRight(label, leftWidth-4))
		if i == m.cursor {
			line = selectedStyle.Render(padRight(fmt.Sprintf("%s %s", badge, it.Root.Name), leftWidth-4))
		}
		b.WriteString(line)
		b.WriteByte('\n')
		// compact status dots like ServiceMonitor process line
		summary := fmt.Sprintf("%d items", it.Total)
		if it.Missing > 0 {
			summary = fmt.Sprintf("%d ok · %d missing", it.OK, it.Missing)
		}
		kind := "file"
		if it.Root.IsDir {
			kind = "folder"
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %s %s", shortDot(it), kind+" · "+summary)))
		b.WriteByte('\n')
	}
	return panelStyle.Width(leftWidth).Height(h).Render(b.String())
}

func (m Model) renderRight() string {
	h := m.height - 4
	if h < 8 {
		h = 8
	}
	w := m.width - leftWidth - 4
	if w < 40 {
		w = 40
	}
	title := panelTitleStyle.Render("Details")
	inner := title + "\n" + m.detail.View()
	return panelStyle.Width(w).Height(h).Render(inner)
}

func (m Model) renderDetail() string {
	if len(m.items) == 0 {
		return dimStyle.Render("Select a share from the left panel, or press a to add one.")
	}
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return dimStyle.Render("Select a share from the left panel.")
	}
	it := m.items[m.cursor]
	r := it.Root

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", nameStyle.Render(r.Name))
	kind := "file"
	if r.IsDir {
		kind = "folder"
	}
	fmt.Fprintf(&b, "%s %s\n", dimStyle.Render("type"), kind)
	fmt.Fprintf(&b, "%s %s\n", dimStyle.Render("path"), r.Path)
	fmt.Fprintf(&b, "%s %s\n", dimStyle.Render("status"), rootStatusLabel(it))
	fmt.Fprintf(&b, "%s %d  %s %d  %s %d\n\n",
		dimStyle.Render("entries"), it.Total,
		upStyle.Render("ok"), it.OK,
		missStyle.Render("missing"), it.Missing,
	)

	b.WriteString(panelTitleStyle.Render("Entries") + "\n")
	count := 0
	for _, e := range m.entries {
		if e.RootID != r.ID {
			continue
		}
		count++
		rel := e.Name
		if rp, err := filepath.Rel(r.Path, e.Path); err == nil && rp != "." {
			rel = rp
		}
		etype := "file"
		if e.IsDir {
			etype = "dir "
		}
		badge := upStyle.Render("OK")
		if !e.Available {
			badge = missStyle.Render("MISS")
		}
		size := "—"
		if !e.IsDir {
			size = formatSize(e.Size)
		}
		dl := fmt.Sprintf("%d×", e.DownloadCount)
		if e.IsDir {
			dl = fmt.Sprintf("%d zip", e.DownloadCount)
		}
		fmt.Fprintf(&b, "  %s  %-4s  %-8s  %-8s  %s\n", badge, etype, size, dl, rel)
		if count >= 200 {
			fmt.Fprintf(&b, "\n%s\n", dimStyle.Render(fmt.Sprintf("… and more (showing first %d)", count)))
			break
		}
	}
	if count == 0 {
		b.WriteString(dimStyle.Render("  (no entries)") + "\n")
	}

	b.WriteByte('\n')
	b.WriteString(panelTitleStyle.Render("Database") + "\n")
	fmt.Fprintf(&b, "  %s\n", m.dbPath)
	fmt.Fprintf(&b, "  %s\n", dimStyle.Render("a add · d remove root · c cleanup missing · r refresh"))

	return b.String()
}

func statusBadge(it listItem) string {
	if it.Total == 0 {
		return dimStyle.Render("—")
	}
	if it.Missing == 0 {
		return upStyle.Render("OK")
	}
	if it.OK == 0 {
		return missStyle.Render("MISS")
	}
	return missStyle.Render("MIX")
}

func rootStatusLabel(it listItem) string {
	if it.Total == 0 {
		return dimStyle.Render("EMPTY")
	}
	if it.Missing == 0 {
		return upStyle.Render("ALL AVAILABLE")
	}
	if it.OK == 0 {
		return missStyle.Render("ALL MISSING")
	}
	return missStyle.Render(fmt.Sprintf("PARTIAL (%d missing)", it.Missing))
}

func shortDot(it listItem) string {
	if it.Total == 0 {
		return dimStyle.Render("·")
	}
	if it.Missing == 0 {
		return upStyle.Render("•")
	}
	if it.OK == 0 {
		return missStyle.Render("·")
	}
	return missStyle.Render("•")
}

func formatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1fG", float64(n)/(1024*1024*1024))
}

func padRight(s string, n int) string {
	plain := stripRough(s)
	if len(plain) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(plain))
}

func stripRough(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

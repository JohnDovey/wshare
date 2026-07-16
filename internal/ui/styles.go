package ui

import "github.com/charmbracelet/lipgloss"

// Styles mirror ServiceMonitor's lipgloss palette.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true)

	normalItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	upStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	downStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	missStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	keyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)

	nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
)

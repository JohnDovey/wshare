// Command wshare-admin is a ServiceMonitor-style terminal UI for browsing
// and maintaining the wShare SQLite catalog.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnDovey/wshare/internal/store"
	"github.com/JohnDovey/wshare/internal/ui"
)

func main() {
	dbPath := flag.String("db", defaultDB(), "Path to SQLite database")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer st.Close()

	model := ui.New(*dbPath, st)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
}

func defaultDB() string {
	if v := os.Getenv("WSHARE_DB"); v != "" {
		return v
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "wshare.db"
	}
	return filepath.Join(cwd, "wshare.db")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "wshare-admin: %v\n", err)
	os.Exit(1)
}

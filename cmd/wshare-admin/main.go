package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JohnDovey/wshare/internal/scanner"
	"github.com/JohnDovey/wshare/internal/store"
)

func main() {
	dbPath := flag.String("db", defaultDB(), "Path to SQLite database")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	fmt.Printf("wShare Admin — database: %s\n", *dbPath)
	fmt.Println("(Windows / Linux / macOS console)")
	fmt.Println()

	in := bufio.NewScanner(os.Stdin)
	for {
		printMenu()
		fmt.Print("> ")
		if !in.Scan() {
			break
		}
		choice := strings.TrimSpace(in.Text())
		switch choice {
		case "1", "l", "list":
			cmdList(st)
		case "2", "a", "add":
			cmdAdd(st, in)
		case "3", "r", "remove":
			cmdRemove(st, in)
		case "4", "c", "cleanup":
			cmdCleanup(st)
		case "5", "q", "quit", "exit":
			fmt.Println("Goodbye.")
			return
		default:
			fmt.Println("Unknown option. Choose 1–5.")
		}
		fmt.Println()
	}
}

func printMenu() {
	fmt.Println("─────────────────────────────────")
	fmt.Println("  1) List / browse database")
	fmt.Println("  2) Add file or directory")
	fmt.Println("  3) Remove file or directory")
	fmt.Println("  4) Cleanup missing (on disk gone)")
	fmt.Println("  5) Quit")
	fmt.Println("─────────────────────────────────")
}

func cmdList(st *store.Store) {
	roots, err := st.ListRoots()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if len(roots) == 0 {
		fmt.Println("No roots in database.")
		return
	}

	entries, err := st.ListEntries(0)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	store.MarkAvailability(entries)

	fmt.Printf("\nRoots: %d | Entries: %d\n\n", len(roots), len(entries))
	for _, r := range roots {
		kind := "file"
		if r.IsDir {
			kind = "dir "
		}
		fmt.Printf("ROOT #%d  [%s]  %s\n", r.ID, kind, r.Path)
		for _, e := range entries {
			if e.RootID != r.ID {
				continue
			}
			status := "OK"
			if !e.Available {
				status = "MISSING"
			}
			etype := "file"
			if e.IsDir {
				etype = "dir "
			}
			rel := e.Name
			if rp, err := filepath.Rel(r.Path, e.Path); err == nil && rp != "." {
				rel = rp
			}
			fmt.Printf("  #%d  [%s] %-8s  %s\n", e.ID, etype, status, rel)
		}
		fmt.Println()
	}
}

func cmdAdd(st *store.Store, in *bufio.Scanner) {
	fmt.Print("Path to file or directory: ")
	if !in.Scan() {
		return
	}
	path := strings.TrimSpace(in.Text())
	if path == "" {
		fmt.Println("Cancelled.")
		return
	}
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
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("Added %s\n  files: %d  dirs: %d  skipped (ignore): %d  root id: %d\n",
		res.RootPath, res.FilesAdded, res.DirsAdded, res.Skipped, res.RootID)
}

func cmdRemove(st *store.Store, in *bufio.Scanner) {
	fmt.Println("Remove by: 1) entry ID  2) root ID  3) path")
	fmt.Print("Choice: ")
	if !in.Scan() {
		return
	}
	switch strings.TrimSpace(in.Text()) {
	case "1":
		fmt.Print("Entry ID: ")
		if !in.Scan() {
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(in.Text()), 10, 64)
		if err != nil {
			fmt.Println("Invalid ID")
			return
		}
		if err := st.RemoveEntry(id); err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Println("Entry removed.")
	case "2":
		fmt.Print("Root ID: ")
		if !in.Scan() {
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(in.Text()), 10, 64)
		if err != nil {
			fmt.Println("Invalid ID")
			return
		}
		if err := st.RemoveRoot(id); err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Println("Root and its entries removed.")
	case "3":
		fmt.Print("Absolute or relative path: ")
		if !in.Scan() {
			return
		}
		p := strings.TrimSpace(in.Text())
		abs, err := filepath.Abs(p)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		n, err := st.RemoveByPath(abs)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Printf("Removed %d record(s).\n", n)
	default:
		fmt.Println("Cancelled.")
	}
}

func cmdCleanup(st *store.Store) {
	n, err := st.CleanupMissing()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("Removed %d missing entr(y/ies) from the database.\n", n)
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

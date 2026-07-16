package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/JohnDovey/wshare/internal/port"
	"github.com/JohnDovey/wshare/internal/scanner"
	"github.com/JohnDovey/wshare/internal/server"
	"github.com/JohnDovey/wshare/internal/store"
)

func main() {
	filename := flag.String("f", "", "File or folder to add to the share database and list")
	dbPath := flag.String("db", defaultDB(), "Path to SQLite database")
	startPort := flag.Int("p", 8080, "Preferred listen port (tries higher ports if busy)")
	flag.Parse()

	// Positional path as alternative to -f
	pathArg := *filename
	if pathArg == "" && flag.NArg() > 0 {
		pathArg = flag.Arg(0)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	if pathArg != "" {
		res, err := scanner.AddPath(st, pathArg)
		if err != nil {
			log.Fatalf("add path: %v", err)
		}
		kind := "file"
		if res.RootIsDir {
			kind = "folder"
		}
		log.Printf("Added %s %q (root id %d): %d file(s), %d dir(s), %d skipped by ignore rules",
			kind, res.RootPath, res.RootID, res.FilesAdded, res.DirsAdded, res.Skipped)
	}

	entries, err := st.ListEntries(0)
	if err != nil {
		log.Fatalf("list entries: %v", err)
	}
	if len(entries) == 0 {
		log.Println("Share database is empty.")
		log.Println("Usage: wshare -f <file-or-folder>")
		log.Println("   or: wshare-admin  (to manage the database)")
		flag.PrintDefaults()
		os.Exit(1)
	}

	freePort, err := port.FindFree(*startPort, 1000)
	if err != nil {
		log.Fatalf("find free port: %v", err)
	}
	if freePort != *startPort {
		log.Printf("Port %d is in use; using port %d instead", *startPort, freePort)
	}

	ip := server.GetOutboundIP().String()
	srv := &server.Server{Store: st, IP: ip, Port: freePort}

	addr := fmt.Sprintf(":%d", freePort)
	log.Printf("Share list:  http://%s:%d/", ip, freePort)
	log.Printf("Local list:  http://127.0.0.1:%d/", freePort)
	log.Printf("Database:    %s", *dbPath)
	log.Printf("Entries:     %d", len(entries))
	log.Printf("Listening on port %d", freePort)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func defaultDB() string {
	if v := os.Getenv("WSHARE_DB"); v != "" {
		return v
	}
	// Prefer current working directory so admin and server share the same file.
	cwd, err := os.Getwd()
	if err != nil {
		return "wshare.db"
	}
	return filepath.Join(cwd, "wshare.db")
}

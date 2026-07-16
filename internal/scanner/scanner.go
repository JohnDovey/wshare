package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JohnDovey/wshare/internal/ignore"
	"github.com/JohnDovey/wshare/internal/store"
)

// Result summarizes a scan/add operation.
type Result struct {
	RootID       int64
	FilesAdded   int
	DirsAdded    int
	Skipped      int
	RootPath     string
	RootIsDir    bool
}

// AddPath adds a file or directory to the store, respecting .gitignore and robots.txt.
func AddPath(s *store.Store, path string) (*Result, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	matchRoot := abs
	if !info.IsDir() {
		matchRoot = filepath.Dir(abs)
	}
	matcher, err := ignore.NewForRoot(matchRoot)
	if err != nil {
		return nil, err
	}

	// If the path itself is ignored, refuse.
	if info.IsDir() {
		// root dir itself is never ignored via relative "." — OK
	} else if matcher.Ignored(abs) {
		return nil, fmt.Errorf("path is excluded by .gitignore or robots.txt: %s", abs)
	}

	rootID, err := s.AddRoot(abs, info.IsDir())
	if err != nil {
		return nil, err
	}

	res := &Result{RootID: rootID, RootPath: abs, RootIsDir: info.IsDir()}

	if !info.IsDir() {
		id, err := s.UpsertEntry(abs, info.Name(), false, info.Size(), nil, rootID)
		if err != nil {
			return nil, err
		}
		_ = id
		res.FilesAdded = 1
		return res, nil
	}

	// Map absolute directory path -> entry ID for parent linkage.
	parentIDs := map[string]int64{}

	// Register the root directory as an entry with no parent.
	rootEntryID, err := s.UpsertEntry(abs, filepath.Base(abs), true, 0, nil, rootID)
	if err != nil {
		return nil, err
	}
	parentIDs[abs] = rootEntryID
	res.DirsAdded = 1

	err = filepath.Walk(abs, func(p string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable
		}
		p = filepath.Clean(p)
		if p == abs {
			return nil // already added
		}

		if matcher.Ignored(p) {
			res.Skipped++
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		var parentPtr *int64
		parentPath := filepath.Dir(p)
		if pid, ok := parentIDs[parentPath]; ok {
			parentPtr = &pid
		}

		if fi.IsDir() {
			id, err := s.UpsertEntry(p, fi.Name(), true, 0, parentPtr, rootID)
			if err != nil {
				return err
			}
			parentIDs[p] = id
			res.DirsAdded++
			return nil
		}

		_, err := s.UpsertEntry(p, fi.Name(), false, fi.Size(), parentPtr, rootID)
		if err != nil {
			return err
		}
		res.FilesAdded++
		return nil
	})
	if err != nil {
		return res, err
	}
	return res, nil
}

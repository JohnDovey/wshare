package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// Matcher decides whether a path under a share root should be excluded from sharing.
type Matcher struct {
	root     string
	git      *ignore.GitIgnore
	robots   []string // disallow path prefixes relative to root (slash-separated)
	hasRules bool
}

// NewForRoot builds a matcher from .gitignore and robots.txt under rootPath.
// rootPath must be an absolute directory (or the parent of a shared file).
func NewForRoot(rootPath string) (*Matcher, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	dir := rootPath
	if !info.IsDir() {
		dir = filepath.Dir(rootPath)
	}
	dir = filepath.Clean(dir)

	m := &Matcher{root: dir}

	// Load .gitignore (root-level). Nested .gitignore files are handled while walking via ReloadNested.
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		gi, err := ignore.CompileIgnoreFile(gitignorePath)
		if err == nil {
			m.git = gi
			m.hasRules = true
		}
	}

	// Also always ignore VCS metadata and the share database.
	defaultPatterns := []string{
		".git/",
		".git",
		".svn/",
		".hg/",
		"wshare.db",
		"wshare.db-journal",
		"wshare.db-wal",
		"wshare.db-shm",
	}
	if m.git == nil {
		m.git = ignore.CompileIgnoreLines(defaultPatterns...)
	} else {
		// recompile with extras prepended
		lines := defaultPatterns
		if data, err := os.ReadFile(gitignorePath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
		}
		m.git = ignore.CompileIgnoreLines(lines...)
	}
	m.hasRules = true

	robotsPath := filepath.Join(dir, "robots.txt")
	if _, err := os.Stat(robotsPath); err == nil {
		disallows, err := parseRobots(robotsPath)
		if err == nil && len(disallows) > 0 {
			m.robots = disallows
			m.hasRules = true
		}
	}

	return m, nil
}

// parseRobots extracts Disallow paths for User-agent * or wshare.
func parseRobots(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var disallows []string
	inStarAgent := false
	sawAgent := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "user-agent:") {
			parts := strings.SplitN(line, ":", 2)
			agent := "*"
			if len(parts) == 2 {
				agent = strings.TrimSpace(parts[1])
			}
			inStarAgent = agent == "*" || strings.EqualFold(agent, "wshare")
			sawAgent = true
			continue
		}
		// If no User-agent was declared, treat Disallow as global.
		if sawAgent && !inStarAgent {
			continue
		}
		if strings.HasPrefix(lower, "disallow:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			p := strings.TrimSpace(parts[1])
			if p == "" {
				continue // empty Disallow means allow all
			}
			p = strings.TrimPrefix(p, "/")
			disallows = append(disallows, p)
		}
	}
	return disallows, scanner.Err()
}

// Ignored reports whether absPath should be excluded from sharing.
func (m *Matcher) Ignored(absPath string) bool {
	if m == nil {
		return false
	}
	absPath = filepath.Clean(absPath)
	rel, err := filepath.Rel(m.root, absPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return false
	}
	// Never share the ignore rule files themselves as "secret" config (optional).
	// robots.txt and .gitignore can still be listed if user wants; exclude robots and gitignore by default.
	base := filepath.Base(absPath)
	if base == ".gitignore" || base == "robots.txt" {
		return true
	}

	relSlash := filepath.ToSlash(rel)

	if m.git != nil {
		// Directory patterns like "build/" often need a trailing slash to match.
		if m.git.MatchesPath(relSlash) || m.git.MatchesPath(relSlash+"/") {
			return true
		}
		// Also match if any path segment is an ignored directory name via prefix.
		// e.g. pattern "build/" should exclude "build/out.bin".
		parts := strings.Split(relSlash, "/")
		acc := ""
		for i, part := range parts {
			if i == 0 {
				acc = part
			} else {
				acc = acc + "/" + part
			}
			if m.git.MatchesPath(acc) || m.git.MatchesPath(acc+"/") {
				return true
			}
		}
	}

	for _, d := range m.robots {
		if d == "" {
			continue
		}
		d = strings.TrimPrefix(filepath.ToSlash(d), "/")
		d = strings.TrimSuffix(d, "/")
		// Disallow /foo matches foo, foo/bar, etc.
		if relSlash == d || strings.HasPrefix(relSlash, d+"/") {
			return true
		}
	}
	return false
}

// ShouldSkipDir returns true if the directory itself is ignored (walk can SkipDir).
func (m *Matcher) ShouldSkipDir(absPath string) bool {
	return m.Ignored(absPath)
}

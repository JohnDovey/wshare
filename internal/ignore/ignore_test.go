package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitignoreAndRobots(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "build/\n*.bin\n")
	mustWrite(t, filepath.Join(dir, "robots.txt"), "User-agent: *\nDisallow: /secret\n")
	mustWrite(t, filepath.Join(dir, "ok.txt"), "x")
	_ = os.MkdirAll(filepath.Join(dir, "build"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "secret"), 0o755)
	mustWrite(t, filepath.Join(dir, "build", "out.bin"), "b")
	mustWrite(t, filepath.Join(dir, "secret", "private.txt"), "s")

	m, err := NewForRoot(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]bool{
		filepath.Join(dir, "ok.txt"):              false,
		filepath.Join(dir, "build"):               true,
		filepath.Join(dir, "build", "out.bin"):    true,
		filepath.Join(dir, "secret"):              true,
		filepath.Join(dir, "secret", "private.txt"): true,
		filepath.Join(dir, ".gitignore"):          true,
		filepath.Join(dir, "robots.txt"):          true,
	}
	for p, want := range cases {
		if got := m.Ignored(p); got != want {
			t.Errorf("Ignored(%s)=%v want %v", p, got, want)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

package port

import (
	"fmt"
	"net"
)

// FindFree finds the first free TCP port starting at start (inclusive), up to start+maxTries-1.
func FindFree(start, maxTries int) (int, error) {
	if start < 1 || start > 65535 {
		return 0, fmt.Errorf("invalid start port: %d", start)
	}
	if maxTries < 1 {
		maxTries = 100
	}
	for i := 0; i < maxTries; i++ {
		p := start + i
		if p > 65535 {
			break
		}
		if available(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port found from %d upward (%d attempts)", start, maxTries)
}

func available(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

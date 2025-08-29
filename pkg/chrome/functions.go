package chrome

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// isPortOpen checks if a port is open and accepting connections
func isPortOpen(port string) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+port, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// waitForChromeDebugger waits for Chrome's debugging port to become available
func waitForChromeDebugger(port string, timeout time.Duration) error {
	start := time.Now()
	for {
		if isPortOpen(port) {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json/version", port))
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return nil
			}
			if err == nil {
				resp.Body.Close()
			}
		}
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for Chrome debugger on port %s", port)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Check if the port was recently used (e.g., in TIME_WAIT state)
func isPortRecentlyUsed(port string) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+port, 100*time.Millisecond)
	if err == nil {
		conn.Close()
		return true
	}
	// Check if the port is in TIME_WAIT or similar state
	cmd := exec.Command("netstat", "-an")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), ":"+port)
}

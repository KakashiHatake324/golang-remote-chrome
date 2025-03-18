package chrome

import (
	"fmt"
	"log"
	"net"
	"net/http"
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
			// Additional check to ensure the debugger is fully ready
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json/version", port))
			if err == nil && resp.StatusCode == 200 {
				log.Printf("Chrome debugger is ready on port %s", port)
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
		time.Sleep(100 * time.Millisecond)
	}
}

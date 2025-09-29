//go:build windows
// +build windows

package chrome

import (
	"os/exec"
	"strconv"
)

func killProcess(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	_, err := cmd.CombinedOutput()
	return err
}

//go:build darwin || linux
// +build darwin linux

package chrome

import "syscall"

func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

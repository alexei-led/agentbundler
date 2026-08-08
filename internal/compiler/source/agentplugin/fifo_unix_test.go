//go:build !windows && !plan9 && !js && !wasip1

package agentplugin

import "syscall"

func createFIFO(path string) error {
	return syscall.Mkfifo(path, 0o644)
}

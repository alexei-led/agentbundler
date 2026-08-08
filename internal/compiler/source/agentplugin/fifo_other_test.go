//go:build windows || plan9 || js || wasip1

package agentplugin

import "errors"

func createFIFO(path string) error {
	return errors.New("FIFOs not supported on this platform")
}

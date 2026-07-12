//go:build !linux || (!amd64 && !arm64)

package write

func exchangeDirectories(first, second string) bool {
	return false
}

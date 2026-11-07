//go:build !windows

package promotion

import "os"

func unsafeFileIdentity(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

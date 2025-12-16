package kafkautil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// CleanKRaftDeletedFiles removes Kafka KRaft tombstone files from META_DIR.
func CleanKRaftDeletedFiles(metaDir string) (deleted []string, err error) {
	info, statErr := os.Stat(metaDir)
	if statErr != nil {
		return nil, fmt.Errorf("metaDir not accessible: %s: %w", metaDir, statErr)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("metaDir is not a directory: %s", metaDir)
	}

	var firstErr error

	_ = filepath.WalkDir(metaDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if firstErr == nil {
				firstErr = walkErr
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".deleted") &&
			!strings.HasSuffix(name, ".checkpoint.deleted") {
			return nil
		}

		// Windows: clear READONLY before delete
		if runtime.GOOS == "windows" {
			_ = clearReadOnlyWindows(path)
		}

		if remErr := os.Remove(path); remErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("delete failed %s: %w", path, remErr)
			}
			return nil
		}

		deleted = append(deleted, path)
		return nil
	})

	return deleted, firstErr
}

func clearReadOnlyWindows(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attrs&syscall.FILE_ATTRIBUTE_READONLY == 0 {
		return nil
	}
	return syscall.SetFileAttributes(p, attrs&^syscall.FILE_ATTRIBUTE_READONLY)
}

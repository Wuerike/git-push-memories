package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type writeStage int

const (
	writeCreated writeStage = iota + 1
	writeWritten
	writeSynced
	writeRenamed
)

type writeFault func(writeStage) error

func (stage writeStage) String() string {
	switch stage {
	case writeCreated:
		return "created"
	case writeWritten:
		return "written"
	case writeSynced:
		return "synced"
	case writeRenamed:
		return "renamed"
	default:
		return "unknown"
	}
}

func durableReplace(rootFD int, relativePath string, content []byte, mode os.FileMode, fault writeFault) error {
	parentPath, base := path.Split(relativePath)
	parentPath = strings.TrimSuffix(parentPath, "/")
	if parentPath == "" {
		parentPath = "."
	}
	parentFD, err := unix.Openat2(rootFD, parentPath, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)

	temporaryName, temporary, err := createTemporaryAt(parentFD)
	if err != nil {
		return err
	}
	defer func() {
		_ = temporary.Close()
		_ = unix.Unlinkat(parentFD, temporaryName, 0)
	}()
	if err := injectWriteFault(fault, writeCreated); err != nil {
		return err
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := injectWriteFault(fault, writeWritten); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := injectWriteFault(fault, writeSynced); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := unix.Renameat(parentFD, temporaryName, parentFD, base); err != nil {
		return err
	}
	if err := injectWriteFault(fault, writeRenamed); err != nil {
		return err
	}
	if err := unix.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync memory directory: %w", err)
	}
	return nil
}

func createTemporaryAt(parentFD int) (string, *os.File, error) {
	for range 10 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := ".git-push-memories-" + hex.EncodeToString(random[:]) + ".tmp"
		fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if err == nil {
			return name, os.NewFile(uintptr(fd), name), nil
		}
		if err != unix.EEXIST {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("create unique memory temporary")
}

func injectWriteFault(fault writeFault, stage writeStage) error {
	if fault == nil {
		return nil
	}
	return fault(stage)
}

func (repository *Repository) cleanupTemporaryFiles() error {
	root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", repository.root.Fd()))
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), ".", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() && controlledTemporaryName(entry.Name()) {
			return root.Remove(filepath.ToSlash(filePath))
		}
		return nil
	})
}

func controlledTemporaryName(name string) bool {
	const prefix = ".git-push-memories-"
	const suffix = ".tmp"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	random := name[len(prefix) : len(name)-len(suffix)]
	if len(random) != 16 {
		return false
	}
	_, err := hex.DecodeString(random)
	return err == nil
}

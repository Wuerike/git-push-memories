package memory

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

func durableReplace(rootFD int, relativePath string, content []byte, mode os.FileMode, expectedHash string, fault writeFault) error {
	return durableWrite(rootFD, relativePath, content, mode, expectedHash, false, fault)
}

func durableCreate(rootFD int, relativePath string, content []byte, mode os.FileMode, fault writeFault) error {
	return durableWrite(rootFD, relativePath, content, mode, "", true, fault)
}

func durableRemove(rootFD int, relativePath, expectedHash string, expectedMode os.FileMode) error {
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
		return ErrDrift
	}
	defer unix.Close(parentFD)
	quarantine, err := quarantineAt(parentFD, base)
	if err != nil {
		return ErrDrift
	}
	if err := quarantine.validate(expectedHash, expectedMode); err != nil {
		return errors.Join(err, quarantine.restore(base))
	}
	var replacement unix.Stat_t
	if err := unix.Fstatat(parentFD, base, &replacement, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return errors.Join(ErrDrift, quarantine.restore(base))
	}
	return quarantine.remove()
}

type fileQuarantine struct {
	parentFD      int
	directoryFD   int
	directoryName string
}

const quarantinedFile = "target"

func quarantineAt(parentFD int, base string) (*fileQuarantine, error) {
	for range 10 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, err
		}
		name := ".git-push-memories-quarantine-" + hex.EncodeToString(random[:])
		if err := unix.Mkdirat(parentFD, name, 0o700); errors.Is(err, unix.EEXIST) {
			continue
		} else if err != nil {
			return nil, err
		}
		directoryFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
			return nil, err
		}
		if err := unix.Renameat2(parentFD, base, directoryFD, quarantinedFile, unix.RENAME_NOREPLACE); err != nil {
			_ = unix.Close(directoryFD)
			_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
			return nil, err
		}
		return &fileQuarantine{parentFD: parentFD, directoryFD: directoryFD, directoryName: name}, nil
	}
	return nil, fmt.Errorf("create unique memory quarantine")
}

func (quarantine *fileQuarantine) validate(expectedHash string, expectedMode os.FileMode) error {
	currentFD, err := unix.Openat(quarantine.directoryFD, quarantinedFile, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrDrift
	}
	current := os.NewFile(uintptr(currentFD), quarantinedFile)
	if current == nil {
		_ = unix.Close(currentFD)
		return ErrDrift
	}
	info, statErr := current.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = current.Close()
		return ErrDrift
	}
	contents, readErr := io.ReadAll(current)
	closeErr := current.Close()
	if readErr != nil || closeErr != nil || hash(contents) != expectedHash || expectedMode != 0 && info.Mode().Perm() != expectedMode.Perm() {
		return ErrDrift
	}
	return nil
}

func (quarantine *fileQuarantine) restore(base string) error {
	if err := unix.Renameat2(quarantine.directoryFD, quarantinedFile, quarantine.parentFD, base, unix.RENAME_NOREPLACE); err != nil {
		_ = unix.Close(quarantine.directoryFD)
		return fmt.Errorf("restore quarantined memory file: %w", err)
	}
	return quarantine.removeDirectory()
}

func (quarantine *fileQuarantine) remove() error {
	if err := unix.Unlinkat(quarantine.directoryFD, quarantinedFile, 0); err != nil {
		_ = unix.Close(quarantine.directoryFD)
		return ErrDrift
	}
	if err := unix.Fsync(quarantine.directoryFD); err != nil {
		_ = unix.Close(quarantine.directoryFD)
		return err
	}
	return quarantine.removeDirectory()
}

func (quarantine *fileQuarantine) removeDirectory() error {
	if err := unix.Close(quarantine.directoryFD); err != nil {
		return err
	}
	if err := unix.Unlinkat(quarantine.parentFD, quarantine.directoryName, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(quarantine.parentFD)
}

func targetAbsent(rootFD int, relativePath string) bool {
	var stat unix.Stat_t
	return errors.Is(unix.Fstatat(rootFD, relativePath, &stat, unix.AT_SYMLINK_NOFOLLOW), unix.ENOENT)
}

func ensureMemoryParentDirectories(rootFD int, targetPath string) error {
	parent, _ := path.Split(targetPath)
	segments := strings.Split(strings.TrimSuffix(parent, "/"), "/")
	currentFD, err := unix.Dup(rootFD)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(currentFD) }()
	for _, segment := range segments {
		created := false
		if err := unix.Mkdirat(currentFD, segment, 0o755); err == nil {
			created = true
		} else if !errors.Is(err, unix.EEXIST) {
			return err
		}
		nextFD, err := unix.Openat2(currentFD, segment, &unix.OpenHow{
			Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC),
			Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
		})
		if err != nil {
			return err
		}
		if created {
			if err := unix.Fsync(currentFD); err != nil {
				_ = unix.Close(nextFD)
				return err
			}
		}
		_ = unix.Close(currentFD)
		currentFD = nextFD
	}
	return nil
}

func durableWrite(rootFD int, relativePath string, content []byte, mode os.FileMode, expectedHash string, create bool, fault writeFault) error {
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
	if create {
		if err := unix.Renameat2(parentFD, temporaryName, parentFD, base, unix.RENAME_NOREPLACE); err != nil {
			if errors.Is(err, unix.EEXIST) {
				return ErrDrift
			}
			return err
		}
	} else {
		quarantine, err := quarantineAt(parentFD, base)
		if err != nil {
			return ErrDrift
		}
		if err := quarantine.validate(expectedHash, mode); err != nil {
			return errors.Join(err, quarantine.restore(base))
		}
		if err := unix.Renameat2(parentFD, temporaryName, parentFD, base, unix.RENAME_NOREPLACE); err != nil {
			return errors.Join(err, quarantine.restore(base))
		}
		if err := quarantine.remove(); err != nil {
			return err
		}
	}
	if err := injectWriteFault(fault, writeRenamed); err != nil {
		return err
	}
	if err := unix.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync memory directory: %w", err)
	}
	return nil
}

func syncParentDirectory(rootFD int, relativePath string) error {
	parentPath, _ := path.Split(relativePath)
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
	return unix.Fsync(parentFD)
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

package memory

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/markdown"
	"golang.org/x/sys/unix"
)

type Repository struct {
	root  *os.File
	mutex sync.Mutex
}

// Open opens a memory repository for reading without modifying its files.
func Open(root string) (*Repository, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	rootHandle, err := os.Open(canonical)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	info, err := rootHandle.Stat()
	if err != nil || !info.IsDir() {
		_ = rootHandle.Close()
		return nil, fmt.Errorf("memory root is invalid")
	}
	return &Repository{root: rootHandle}, nil
}

func (repository *Repository) Close() error {
	return repository.root.Close()
}

func (repository *Repository) Inspect(ctx context.Context, scope Scope) (Snapshot, error) {
	var snapshot Snapshot
	err := repository.withLock(ctx, func() error {
		var err error
		snapshot, err = repository.load(ctx, scope.Path)
		return err
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (repository *Repository) load(ctx context.Context, requestedPath string) (Snapshot, error) {
	canonicalPath, source, err := repository.read(requestedPath)
	if err != nil {
		return Snapshot{}, err
	}
	parsed, err := validateDocument(source)
	if err != nil {
		return Snapshot{}, err
	}
	if err := repository.validateGitIdentity(ctx, canonicalPath); err != nil {
		return Snapshot{}, err
	}

	sections := make([]Section, len(parsed))
	for i, parsedSection := range parsed {
		body, err := markdown.SectionContent(source, parsedSection)
		if err != nil {
			return Snapshot{}, fmt.Errorf("read section content: %w", ErrInvalidSchema)
		}
		leaf := i+1 >= len(parsed) || parsed[i+1].ByteStart >= parsedSection.ByteEnd
		sections[i] = Section{
			ID:          parsedSection.AnchorID,
			Heading:     parsedSection.HeadingText,
			Body:        string(body),
			ContentHash: parsedSection.ContentHash,
			Leaf:        leaf,
		}
	}
	return Snapshot{Path: canonicalPath, Sections: sections}, nil
}

func (repository *Repository) read(requestedPath string) (string, []byte, error) {
	canonicalPath, err := canonicalRelativePath(requestedPath)
	if err != nil {
		return "", nil, err
	}
	fileDescriptor, err := unix.Openat2(int(repository.root.Fd()), canonicalPath, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return "", nil, ErrInvalidPath
	}
	file := os.NewFile(uintptr(fileDescriptor), canonicalPath)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return "", nil, ErrInvalidPath
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, ErrInvalidPath
	}
	source, err := io.ReadAll(file)
	if err != nil {
		return "", nil, ErrInvalidPath
	}
	return canonicalPath, source, nil
}

func canonicalRelativePath(requestedPath string) (string, error) {
	normalized := strings.ReplaceAll(requestedPath, "\\", "/")
	if normalized == "" || path.IsAbs(normalized) || filepath.IsAbs(requestedPath) {
		return "", ErrInvalidPath
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", ErrInvalidPath
		}
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || !strings.HasPrefix(clean, "memories/") || path.Ext(clean) != ".md" {
		return "", ErrInvalidPath
	}
	return clean, nil
}

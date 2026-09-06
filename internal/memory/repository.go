package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/markdown"
	"golang.org/x/sys/unix"
)

type Repository struct {
	root  *os.File
	mutex sync.Mutex
	fault writeFault
}

func New(root string) (*Repository, error) {
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
	repository := &Repository{root: rootHandle}
	if err := repository.withLock(context.Background(), true, repository.cleanupTemporaryFiles); err != nil {
		_ = rootHandle.Close()
		return nil, fmt.Errorf("clean memory temporaries: %w", err)
	}
	return repository, nil
}

func (repository *Repository) Close() error {
	return repository.root.Close()
}

func (repository *Repository) Inspect(ctx context.Context, scope Scope) (Snapshot, error) {
	var snapshot Snapshot
	err := repository.withLock(ctx, false, func() error {
		state, err := repository.load(ctx, scope.Path)
		if err != nil {
			return err
		}
		snapshot = state.snapshot
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

type repositoryState struct {
	snapshot Snapshot
	source   []byte
	parsed   []markdown.Section
	mode     os.FileMode
}

func (repository *Repository) load(ctx context.Context, requestedPath string) (repositoryState, error) {
	canonicalPath, source, mode, err := repository.read(requestedPath)
	if err != nil {
		return repositoryState{}, err
	}
	parsed, err := validateDocument(source)
	if err != nil {
		return repositoryState{}, err
	}
	head, blob, err := repository.gitIdentity(ctx, canonicalPath)
	if err != nil {
		return repositoryState{}, err
	}

	sections := make([]Section, len(parsed))
	for i, parsedSection := range parsed {
		body, err := markdown.SectionContent(source, parsedSection)
		if err != nil {
			return repositoryState{}, fmt.Errorf("read section content: %w", ErrInvalidSchema)
		}
		leaf := i+1 >= len(parsed) || parsed[i+1].ByteStart >= parsedSection.ByteEnd
		sections[i] = Section{
			ID:          parsedSection.AnchorID,
			Heading:     parsedSection.HeadingText,
			Level:       parsedSection.HeadingLevel,
			Body:        string(body),
			ContentHash: parsedSection.ContentHash,
			Leaf:        leaf,
		}
	}
	return repositoryState{snapshot: Snapshot{
		Path:     canonicalPath,
		FileHash: hash(source),
		Head:     head,
		Blob:     blob,
		Sections: sections,
	}, source: source, parsed: parsed, mode: mode}, nil
}

func (repository *Repository) Apply(ctx context.Context, change ChangeSet) (Receipt, error) {
	var receipt Receipt
	err := repository.withLock(ctx, true, func() error {
		var err error
		receipt, err = repository.apply(ctx, change)
		return err
	})
	return receipt, err
}

func (repository *Repository) apply(ctx context.Context, change ChangeSet) (Receipt, error) {
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	canonicalPath, source, mode, err := repository.read(change.Target.Path)
	if err != nil {
		return Receipt{}, err
	}
	baselineHash := hash(source)
	if baselineHash != change.BaselineHash {
		return Receipt{}, ErrDrift
	}
	parsed, err := validateDocument(source)
	if err != nil {
		return Receipt{}, err
	}
	if !utf8.ValidString(change.Replacement) || bytes.Contains([]byte(change.Replacement), []byte("<!-- @id:")) {
		return Receipt{}, fmt.Errorf("%w: invalid replacement", ErrInvalidSchema)
	}
	replacementSections, err := markdown.ParseSections([]byte(change.Replacement))
	if err != nil || len(replacementSections) != 0 {
		return Receipt{}, fmt.Errorf("%w: replacement contains heading", ErrInvalidSchema)
	}

	targetIndex := -1
	for i, section := range parsed {
		if section.AnchorID == change.Target.SectionID {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		return Receipt{}, ErrSectionNotFound
	}
	target := parsed[targetIndex]
	if targetIndex+1 < len(parsed) && parsed[targetIndex+1].ByteStart < target.ByteEnd {
		return Receipt{}, ErrSectionNotLeaf
	}
	postState, err := markdown.ReplaceSectionContent(source, target, []byte(change.Replacement))
	if err != nil {
		return Receipt{}, fmt.Errorf("replace section: %w", ErrInvalidSchema)
	}
	postSections, err := validateDocument(postState)
	if err != nil || !sameDocumentShape(parsed, postSections) {
		return Receipt{}, fmt.Errorf("%w: replacement changes document structure", ErrInvalidSchema)
	}
	if err := durableReplace(int(repository.root.Fd()), canonicalPath, postState, mode, repository.fault); err != nil {
		return Receipt{}, fmt.Errorf("write memory file: %w", err)
	}
	return Receipt{
		Target:        Target{Path: canonicalPath, SectionID: change.Target.SectionID},
		BaselineHash:  baselineHash,
		PostStateHash: hash(postState),
	}, nil
}

func sameDocumentShape(before, after []markdown.Section) bool {
	if len(before) != len(after) {
		return false
	}
	for i := range before {
		if before[i].HeadingText != after[i].HeadingText || before[i].HeadingLevel != after[i].HeadingLevel || before[i].AnchorID != after[i].AnchorID {
			return false
		}
	}
	return true
}

func (repository *Repository) read(requestedPath string) (string, []byte, os.FileMode, error) {
	canonicalPath, err := canonicalRelativePath(requestedPath)
	if err != nil {
		return "", nil, 0, err
	}
	fileDescriptor, err := unix.Openat2(int(repository.root.Fd()), canonicalPath, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return "", nil, 0, ErrInvalidPath
	}
	file := os.NewFile(uintptr(fileDescriptor), canonicalPath)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return "", nil, 0, ErrInvalidPath
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, 0, ErrInvalidPath
	}
	source, err := io.ReadAll(file)
	if err != nil {
		return "", nil, 0, ErrInvalidPath
	}
	return canonicalPath, source, info.Mode(), nil
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
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.Ext(clean) != ".md" {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

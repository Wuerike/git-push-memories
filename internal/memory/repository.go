package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/markdown"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
	"golang.org/x/sys/unix"
)

var createPagePath = regexp.MustCompile(`^memories/[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)*\.md$`)

var reservedPageNames = map[string]struct{}{
	"aux": {}, "con": {}, "nul": {}, "prn": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {}, "com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {}, "lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

func IsCanonicalPagePath(requestedPath string) bool {
	canonicalPath, err := canonicalRelativePath(requestedPath)
	return err == nil && canonicalPath == requestedPath
}

func IsCreatePagePath(requestedPath string) bool {
	return createPagePath.MatchString(requestedPath)
}

func PageID(pagePath string) string {
	return strings.TrimSuffix(path.Base(pagePath), ".md")
}

type Repository struct {
	root  *os.File
	mutex sync.Mutex
	fault writeFault
}

func New(root string) (*Repository, error) {
	return open(root, true)
}

// Open avoids startup cleanup so callers can keep discovery and preview side-effect free.
func Open(root string) (*Repository, error) {
	return open(root, false)
}

func open(root string, cleanTemporaries bool) (*Repository, error) {
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
	if cleanTemporaries {
		if err := repository.withLock(context.Background(), true, repository.cleanupTemporaryFiles); err != nil {
			_ = rootHandle.Close()
			return nil, fmt.Errorf("clean memory temporaries: %w", err)
		}
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

func (repository *Repository) Preview(ctx context.Context, request PreviewRequest) (Preview, error) {
	var preview Preview
	err := repository.withLock(ctx, false, func() error {
		state, err := repository.load(ctx, request.Target.Path)
		if err != nil {
			return err
		}
		if err := repository.ensureTargetClean(ctx, state.snapshot.Path, state.snapshot.Head); err != nil {
			return err
		}
		postState, err := replaceSection(state.source, state.parsed, request.Target.SectionID, request.Replacement)
		if err != nil {
			return err
		}
		if err := repository.confirmPreviewState(ctx, state.snapshot); err != nil {
			return err
		}
		preview = Preview{
			Target:       Target{Path: state.snapshot.Path, SectionID: request.Target.SectionID},
			BaselineHash: state.snapshot.FileHash, PostStateHash: hash(postState),
			Head: state.snapshot.Head, Blob: state.snapshot.Blob,
			Baseline: append([]byte(nil), state.source...), PostState: postState,
		}
		return nil
	})
	return preview, err
}

func (repository *Repository) PreviewCreatePage(ctx context.Context, request CreatePageRequest) (CreatePagePreview, error) {
	var preview CreatePagePreview
	err := repository.withLock(ctx, false, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		targetPath, pageID, err := validateCreatePageRequest(request)
		if err != nil {
			return err
		}
		head, err := repository.ensureTargetAbsent(ctx, targetPath)
		if err != nil {
			return err
		}
		postState, err := renderPage(pageID, request.Title, request.Body)
		if err != nil {
			return err
		}
		confirmedHead, err := repository.ensureTargetAbsent(ctx, targetPath)
		if err != nil {
			return err
		}
		if confirmedHead != head {
			return ErrTargetDirty
		}
		preview = CreatePagePreview{
			Target: Target{Path: targetPath, SectionID: pageID + "-overview"}, PageID: pageID,
			PostStateHash: hash(postState), Head: head, PostState: postState, Diff: unifieddiff.Create(targetPath, postState),
		}
		return nil
	})
	return preview, err
}

func (repository *Repository) ReconcileCreatePage(ctx context.Context, request ReconcileCreatePageRequest) (ReconcileResult, error) {
	var result ReconcileResult
	err := repository.withLock(ctx, true, func() error {
		if request.Preflight != nil {
			if err := request.Preflight(); err != nil {
				return err
			}
		}
		targetPath, pageID, err := validateCreatePageRequest(request.CreatePageRequest)
		if err != nil {
			return err
		}
		postState, err := renderPage(pageID, request.Title, request.Body)
		if err != nil || hash(postState) != request.PostStateHash {
			return ErrDrift
		}
		if request.Validate != nil {
			if err := request.Validate(postState); err != nil {
				return err
			}
		}
		head, err := repository.ensureGitTargetAbsent(ctx, targetPath)
		if err != nil || head != request.Head {
			return ErrDrift
		}
		result = ReconcileResult{Target: Target{Path: targetPath, SectionID: pageID + "-overview"}, PostState: postState}
		var stat unix.Stat_t
		statErr := unix.Fstatat(int(repository.root.Fd()), targetPath, &stat, unix.AT_SYMLINK_NOFOLLOW)
		if statErr == nil {
			_, current, _, readErr := repository.read(targetPath)
			if readErr != nil || hash(current) != request.PostStateHash {
				return ErrDrift
			}
			if err := syncParentDirectory(int(repository.root.Fd()), targetPath); err != nil {
				return fmt.Errorf("sync created memory file: %w", err)
			}
			result.State = ReconcilePostState
			return nil
		}
		if !errors.Is(statErr, unix.ENOENT) {
			return ErrDrift
		}
		result.State = ReconcileBaseline
		if request.Mode == ReconcileOnly {
			return nil
		}
		confirmedHead, err := repository.ensureTargetAbsent(ctx, targetPath)
		if err != nil || confirmedHead != request.Head {
			return ErrDrift
		}
		if err := ensureMemoryParentDirectories(int(repository.root.Fd()), targetPath); err != nil {
			return fmt.Errorf("create memory directories: %w", err)
		}
		if err := durableCreate(int(repository.root.Fd()), targetPath, postState, 0o644, repository.fault); err != nil {
			return fmt.Errorf("write memory file: %w", err)
		}
		result.State = ReconcilePostState
		return nil
	})
	return result, err
}

func (repository *Repository) PreviewCreateSection(ctx context.Context, request CreateSectionRequest) (Preview, error) {
	var preview Preview
	err := repository.withLock(ctx, false, func() error {
		state, err := repository.load(ctx, request.TargetPath)
		if err != nil {
			return err
		}
		if err := repository.ensureTargetClean(ctx, state.snapshot.Path, state.snapshot.Head); err != nil {
			return err
		}
		postState, sectionID, err := renderSection(state.source, state.parsed, request.Heading, request.Body)
		if err != nil {
			return err
		}
		if err := repository.confirmPreviewState(ctx, state.snapshot); err != nil {
			return err
		}
		preview = Preview{
			Target: Target{Path: state.snapshot.Path, SectionID: sectionID}, BaselineHash: state.snapshot.FileHash, PostStateHash: hash(postState),
			Head: state.snapshot.Head, Blob: state.snapshot.Blob, Baseline: append([]byte(nil), state.source...), PostState: postState,
			Diff: unifieddiff.Between(state.snapshot.Path, state.source, postState),
		}
		return nil
	})
	return preview, err
}

func (repository *Repository) ReconcileCreateSection(ctx context.Context, request ReconcileCreateSectionRequest) (ReconcileResult, error) {
	var result ReconcileResult
	err := repository.withLock(ctx, true, func() error {
		if request.Preflight != nil {
			if err := request.Preflight(); err != nil {
				return err
			}
		}
		canonicalPath, current, mode, err := repository.read(request.TargetPath)
		if err != nil {
			return err
		}
		head, blob, err := repository.gitIdentity(ctx, canonicalPath)
		if err != nil || head != request.Head || blob != request.Blob {
			return ErrDrift
		}
		baseline, err := repository.gitOutput(ctx, "cat-file", "blob", request.Blob)
		if err != nil || hash(baseline) != request.BaselineHash {
			return ErrDrift
		}
		parsed, err := validateDocument(baseline)
		if err != nil {
			return err
		}
		postState, sectionID, err := renderSection(baseline, parsed, request.Heading, request.Body)
		if err != nil || sectionID != request.ExpectedSectionID || hash(postState) != request.PostStateHash {
			return ErrDrift
		}
		result = ReconcileResult{Target: Target{Path: canonicalPath, SectionID: sectionID}, Baseline: append([]byte(nil), baseline...), PostState: postState}
		if request.Validate != nil {
			if err := request.Validate(result.Baseline, result.PostState); err != nil {
				return err
			}
		}
		switch hash(current) {
		case request.PostStateHash:
			result.State = ReconcilePostState
			return nil
		case request.BaselineHash:
			result.State = ReconcileBaseline
			if request.Mode == ReconcileOnly {
				return nil
			}
			if err := repository.ensureTargetClean(ctx, canonicalPath, request.Head); err != nil {
				return err
			}
			_, confirmed, _, err := repository.read(canonicalPath)
			if err != nil || hash(confirmed) != request.BaselineHash {
				return ErrDrift
			}
			if err := durableReplace(int(repository.root.Fd()), canonicalPath, postState, mode, request.BaselineHash, repository.fault); err != nil {
				return fmt.Errorf("write memory file: %w", err)
			}
			result.State = ReconcilePostState
			return nil
		default:
			return ErrDrift
		}
	})
	return result, err
}

func validateCreatePageRequest(request CreatePageRequest) (string, string, error) {
	if !createPagePath.MatchString(request.TargetPath) {
		return "", "", ErrInvalidPath
	}
	pageID := PageID(request.TargetPath)
	if _, reserved := reservedPageNames[pageID]; reserved {
		return "", "", ErrInvalidPath
	}
	if len(pageID) > 64 || len(pageID+"-overview") > 64 {
		return "", "", ErrGeneratedIDTooLong
	}
	if request.Title == "" || len(request.Title) > 256 || !utf8.ValidString(request.Title) || hasDisallowedControl(request.Title, false) || strings.ContainsAny(request.Title, "\r\n") ||
		strings.Contains(request.Title, "<!-- @id:") || hasUnresolvedMerge([]byte(request.Title)) {
		return "", "", fmt.Errorf("%w: invalid page title", ErrInvalidSchema)
	}
	sections, err := markdown.ParseSections([]byte(request.Title))
	if err != nil || len(sections) != 0 {
		return "", "", fmt.Errorf("%w: page title contains heading", ErrInvalidSchema)
	}
	return request.TargetPath, pageID, nil
}

func renderPage(pageID, title, body string) ([]byte, error) {
	normalizedBody := strings.ReplaceAll(body, "\r\n", "\n")
	if normalizedBody == "" || !utf8.ValidString(normalizedBody) || hasDisallowedControl(normalizedBody, true) || strings.Contains(normalizedBody, "\r") ||
		strings.Contains(normalizedBody, "<!-- @id:") || hasUnresolvedMerge([]byte(normalizedBody)) {
		return nil, fmt.Errorf("%w: invalid page body", ErrInvalidSchema)
	}
	sections, err := markdown.ParseSections([]byte(normalizedBody))
	if err != nil || len(sections) != 0 {
		return nil, fmt.Errorf("%w: page body contains heading", ErrInvalidSchema)
	}
	document := markdown.RenderPage(pageID, title, normalizedBody)
	if _, err := validateDocument(document); err != nil {
		return nil, err
	}
	return document, nil
}

func renderSection(source []byte, parsed []markdown.Section, heading, body string) ([]byte, string, error) {
	if heading == "" || len(heading) > 256 || !utf8.ValidString(heading) || hasDisallowedControl(heading, false) || strings.ContainsAny(heading, "\r\n") ||
		strings.Contains(heading, "<!-- @id:") || hasUnresolvedMerge([]byte(heading)) {
		return nil, "", fmt.Errorf("%w: invalid section heading", ErrInvalidSchema)
	}
	headingSections, err := markdown.ParseSections([]byte(heading))
	if err != nil || len(headingSections) != 0 {
		return nil, "", fmt.Errorf("%w: section heading contains heading", ErrInvalidSchema)
	}
	normalizedBody := strings.ReplaceAll(body, "\r\n", "\n")
	if normalizedBody == "" || !utf8.ValidString(normalizedBody) || hasDisallowedControl(normalizedBody, true) || strings.Contains(normalizedBody, "\r") ||
		strings.Contains(normalizedBody, "<!-- @id:") || hasUnresolvedMerge([]byte(normalizedBody)) {
		return nil, "", fmt.Errorf("%w: invalid section body", ErrInvalidSchema)
	}
	bodySections, err := markdown.ParseSections([]byte(normalizedBody))
	if err != nil || len(bodySections) != 0 {
		return nil, "", fmt.Errorf("%w: section body contains heading", ErrInvalidSchema)
	}
	pageID := ""
	for _, section := range parsed {
		if section.HeadingLevel == 1 {
			pageID = section.AnchorID
		}
		if section.HeadingText == heading {
			return nil, "", ErrSectionExists
		}
	}
	slug := slugHeading(heading)
	sectionID := pageID + "-" + slug
	if slug == "" {
		return nil, "", ErrInvalidID
	}
	if len(sectionID) > 64 {
		return nil, "", ErrGeneratedIDTooLong
	}
	for _, section := range parsed {
		if section.AnchorID == sectionID {
			return nil, "", ErrSectionExists
		}
	}
	separator := "\n\n"
	if bytes.HasSuffix(source, []byte("\n\n")) {
		separator = ""
	} else if bytes.HasSuffix(source, []byte("\n")) {
		separator = "\n"
	}
	section := fmt.Sprintf("## %s\n<!-- @id: %s -->\n%s", heading, sectionID, strings.TrimSuffix(normalizedBody, "\n")+"\n")
	postState := append(append(append([]byte(nil), source...), separator...), section...)
	if _, err := validateDocument(postState); err != nil {
		return nil, "", err
	}
	return postState, sectionID, nil
}

func slugHeading(heading string) string {
	var slug strings.Builder
	separator := false
	for _, character := range strings.ToLower(heading) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(character)
			separator = false
		} else if slug.Len() > 0 {
			separator = true
		}
	}
	return slug.String()
}

func hasDisallowedControl(value string, allowLayout bool) bool {
	for _, character := range value {
		if allowLayout && (character == '\t' || character == '\n') {
			continue
		}
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func (repository *Repository) ReconcileApply(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
	var result ReconcileResult
	err := repository.withLock(ctx, true, func() error {
		if request.Preflight != nil {
			if err := request.Preflight(); err != nil {
				return err
			}
		}
		canonicalPath, current, mode, err := repository.read(request.Target.Path)
		if err != nil {
			return err
		}
		head, blob, err := repository.gitIdentity(ctx, canonicalPath)
		if err != nil || head != request.Head || blob != request.Blob {
			return ErrDrift
		}
		baseline, err := repository.gitOutput(ctx, "cat-file", "blob", request.Blob)
		if err != nil || hash(baseline) != request.BaselineHash {
			return ErrDrift
		}
		parsed, err := validateDocument(baseline)
		if err != nil {
			return err
		}
		postState, err := replaceSection(baseline, parsed, request.Target.SectionID, request.Replacement)
		if err != nil {
			return err
		}
		if hash(postState) != request.PostStateHash {
			return ErrDrift
		}
		result = ReconcileResult{
			Target:   memoryTarget(canonicalPath, request.Target.SectionID),
			Baseline: append([]byte(nil), baseline...), PostState: postState,
		}
		if request.Validate != nil {
			if err := request.Validate(result.Baseline, result.PostState); err != nil {
				return err
			}
		}
		switch hash(current) {
		case request.PostStateHash:
			result.State = ReconcilePostState
			return nil
		case request.BaselineHash:
			result.State = ReconcileBaseline
			if request.Mode == ReconcileOnly {
				return nil
			}
			if err := repository.ensureTargetClean(ctx, canonicalPath, request.Head); err != nil {
				return err
			}
			_, confirmed, _, err := repository.read(canonicalPath)
			if err != nil || hash(confirmed) != request.BaselineHash {
				return ErrDrift
			}
			if err := durableReplace(int(repository.root.Fd()), canonicalPath, postState, mode, request.BaselineHash, repository.fault); err != nil {
				return fmt.Errorf("write memory file: %w", err)
			}
			result.State = ReconcilePostState
			return nil
		default:
			return ErrDrift
		}
	})
	return result, err
}

func memoryTarget(path, sectionID string) Target {
	return Target{Path: path, SectionID: sectionID}
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
	postState, err := replaceSection(source, parsed, change.Target.SectionID, change.Replacement)
	if err != nil {
		return Receipt{}, err
	}
	if err := durableReplace(int(repository.root.Fd()), canonicalPath, postState, mode, baselineHash, repository.fault); err != nil {
		return Receipt{}, fmt.Errorf("write memory file: %w", err)
	}
	return Receipt{
		Target:        Target{Path: canonicalPath, SectionID: change.Target.SectionID},
		BaselineHash:  baselineHash,
		PostStateHash: hash(postState),
	}, nil
}

func replaceSection(source []byte, parsed []markdown.Section, sectionID, replacement string) ([]byte, error) {
	if !utf8.ValidString(replacement) || bytes.Contains([]byte(replacement), []byte("<!-- @id:")) {
		return nil, fmt.Errorf("%w: invalid replacement", ErrInvalidSchema)
	}
	replacementSections, err := markdown.ParseSections([]byte(replacement))
	if err != nil || len(replacementSections) != 0 {
		return nil, fmt.Errorf("%w: replacement contains heading", ErrInvalidSchema)
	}
	targetIndex := -1
	for i, section := range parsed {
		if section.AnchorID == sectionID {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		return nil, ErrSectionNotFound
	}
	target := parsed[targetIndex]
	if targetIndex+1 < len(parsed) && parsed[targetIndex+1].ByteStart < target.ByteEnd {
		return nil, ErrSectionNotLeaf
	}
	postState, err := markdown.ReplaceSectionContent(source, target, []byte(replacement))
	if err != nil {
		return nil, fmt.Errorf("replace section: %w", ErrInvalidSchema)
	}
	postSections, err := validateDocument(postState)
	if err != nil || !sameDocumentShape(parsed, postSections) {
		return nil, fmt.Errorf("%w: replacement changes document structure", ErrInvalidSchema)
	}
	return postState, nil
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
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || !strings.HasPrefix(clean, "memories/") || path.Ext(clean) != ".md" {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

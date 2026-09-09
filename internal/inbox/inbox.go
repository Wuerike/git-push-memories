package inbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionRemove Action = "remove"
)

var (
	ErrInvalidProposal = errors.New("invalid proposal")
	ErrNotMain         = errors.New("memory repository must be on main")
	ErrIndexLocked     = errors.New("Git index is locked")
)

type Request struct {
	Title           string
	SuggestedAction Action
	Target          string
	Knowledge       string
	SourcePaths     []string
}

type Proposal struct {
	SchemaVersion   int      `json:"schema_version"`
	ID              string   `json:"id"`
	Path            string   `json:"path"`
	Commit          string   `json:"commit"`
	Title           string   `json:"title"`
	SuggestedAction Action   `json:"suggested_action"`
	Target          string   `json:"target,omitempty"`
	Knowledge       string   `json:"knowledge"`
	SourcePaths     []string `json:"source_paths"`
}

type Inbox struct {
	memoryRoot *os.File
	rootPath   string
	stateRoot  string
}

func New(memoryRoot, stateRoot string) (*Inbox, error) {
	canonical, err := filepath.EvalSymlinks(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	root, err := os.Open(canonical)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	info, err := root.Stat()
	if err != nil || !info.IsDir() {
		_ = root.Close()
		return nil, fmt.Errorf("invalid memory root")
	}
	stateAbsolute, err := filepath.Abs(stateRoot)
	if err != nil || stateRoot == "" {
		_ = root.Close()
		return nil, fmt.Errorf("state root is required")
	}
	return &Inbox{memoryRoot: root, rootPath: canonical, stateRoot: stateAbsolute}, nil
}

func (inbox *Inbox) Close() error {
	return inbox.memoryRoot.Close()
}

func (inbox *Inbox) Add(ctx context.Context, request Request) (Proposal, error) {
	normalized, err := validate(request)
	if err != nil {
		return Proposal{}, err
	}
	var proposal Proposal
	err = inbox.withLock(ctx, func() error {
		branch, err := inbox.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil || branch != "main" {
			return ErrNotMain
		}
		id, err := identifier()
		if err != nil {
			return fmt.Errorf("create proposal identifier")
		}
		proposal = Proposal{
			SchemaVersion:   1,
			ID:              id,
			Path:            path.Join("proposals", id+"-"+slug(normalized.Title)+".md"),
			Title:           normalized.Title,
			SuggestedAction: normalized.SuggestedAction,
			Target:          normalized.Target,
			Knowledge:       normalized.Knowledge,
			SourcePaths:     append([]string(nil), normalized.SourcePaths...),
		}
		contents := render(proposal)
		root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", inbox.memoryRoot.Fd()))
		if err != nil {
			return fmt.Errorf("open memory repository")
		}
		defer root.Close()
		if err := root.Mkdir("proposals", 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create proposal directory")
		}
		file, err := root.OpenFile(proposal.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create proposal file")
		}
		published := false
		defer func() {
			if !published {
				_ = root.Remove(proposal.Path)
			}
		}()
		if _, err := file.Write(contents); err != nil {
			_ = file.Close()
			return fmt.Errorf("write proposal file")
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync proposal file")
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close proposal file")
		}
		parent, err := inbox.gitText(ctx, nil, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("read proposal parent")
		}
		gitDirectory, err := inbox.gitPath(ctx, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return fmt.Errorf("resolve Git directory")
		}
		indexPath, err := inbox.gitPath(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index")
		if err != nil || !within(indexPath, gitDirectory) {
			return fmt.Errorf("resolve Git index")
		}
		indexLockPath := indexPath + ".lock"
		indexLock, err := os.OpenFile(indexLockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			return ErrIndexLocked
		}
		if err != nil {
			return fmt.Errorf("acquire Git index lock")
		}
		lockInstalled := false
		defer func() {
			_ = indexLock.Close()
			if !lockInstalled {
				_ = os.Remove(indexLockPath)
			}
		}()
		originalIndex, indexMode, err := readRegularFile(indexPath)
		if err != nil {
			return fmt.Errorf("read Git index")
		}
		if err := indexLock.Chmod(indexMode.Perm()); err != nil {
			return fmt.Errorf("set Git index lock mode")
		}

		blob, err := inbox.gitText(ctx, contents, "hash-object", "-w", "--stdin")
		if err != nil || blob == "" {
			return fmt.Errorf("write proposal blob")
		}
		commitIndex, err := temporaryIndex(inbox.stateRoot)
		if err != nil {
			return err
		}
		defer os.Remove(commitIndex)
		commitEnvironment := map[string]string{"GIT_INDEX_FILE": commitIndex}
		if _, err := inbox.gitInputEnv(ctx, commitEnvironment, nil, "read-tree", parent); err != nil {
			return fmt.Errorf("initialize proposal tree")
		}
		cacheInfo := "100644," + blob + "," + proposal.Path
		if _, err := inbox.gitInputEnv(ctx, commitEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
			return fmt.Errorf("update proposal tree")
		}
		tree, err := inbox.gitTextEnv(ctx, commitEnvironment, nil, "write-tree")
		if err != nil || tree == "" {
			return fmt.Errorf("write proposal tree")
		}
		message := []byte("Add memory proposal: " + proposal.Title + "\n")
		commit, err := inbox.gitText(ctx, message, "commit-tree", tree, "-p", parent)
		if err != nil || commit == "" {
			return fmt.Errorf("write proposal commit")
		}
		changed, err := inbox.gitInput(ctx, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", commit)
		if err != nil || !bytes.Equal(changed, []byte(proposal.Path+"\x00")) {
			return fmt.Errorf("verify proposal commit")
		}
		stored, err := inbox.gitInput(ctx, nil, "show", commit+":"+proposal.Path)
		if err != nil || !bytes.Equal(stored, contents) {
			return fmt.Errorf("verify proposal contents")
		}

		finalIndex, err := temporaryIndex(inbox.stateRoot)
		if err != nil {
			return err
		}
		defer os.Remove(finalIndex)
		if err := os.WriteFile(finalIndex, originalIndex, indexMode.Perm()); err != nil {
			return fmt.Errorf("copy Git index")
		}
		finalEnvironment := map[string]string{"GIT_INDEX_FILE": finalIndex}
		if _, err := inbox.gitInputEnv(ctx, finalEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
			return fmt.Errorf("reconcile Git index")
		}
		finalBytes, _, err := readRegularFile(finalIndex)
		if err != nil {
			return fmt.Errorf("read reconciled Git index")
		}
		if _, err := indexLock.Write(finalBytes); err != nil {
			return fmt.Errorf("write Git index lock")
		}
		if err := indexLock.Sync(); err != nil {
			return fmt.Errorf("sync Git index lock")
		}
		if err := indexLock.Close(); err != nil {
			return fmt.Errorf("close Git index lock")
		}
		branch, err = inbox.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
		confirmedHead, headErr := inbox.gitText(ctx, nil, "rev-parse", "HEAD")
		if err != nil || branch != "main" {
			return ErrNotMain
		}
		if headErr != nil || confirmedHead != parent {
			return fmt.Errorf("memory repository HEAD changed")
		}
		if _, err := inbox.gitInput(ctx, nil, "update-ref", "refs/heads/main", commit, parent); err != nil {
			publishedHead, verifyErr := inbox.gitText(context.Background(), nil, "rev-parse", "refs/heads/main")
			if verifyErr != nil || publishedHead != commit {
				return fmt.Errorf("memory repository HEAD changed")
			}
		}
		proposal.Commit = commit
		published = true
		if err := os.Rename(indexLockPath, indexPath); err == nil {
			lockInstalled = true
			_ = syncDirectory(gitDirectory)
		}
		return nil
	})
	if err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func validate(request Request) (Request, error) {
	if request.SuggestedAction != ActionCreate && request.SuggestedAction != ActionUpdate && request.SuggestedAction != ActionRemove {
		return Request{}, ErrInvalidProposal
	}
	if request.Title == "" || len(request.Title) > 256 || !utf8.ValidString(request.Title) || strings.ContainsAny(request.Title, "\r\n") || hasControl(request.Title, false) {
		return Request{}, ErrInvalidProposal
	}
	request.Knowledge = strings.ReplaceAll(request.Knowledge, "\r\n", "\n")
	if request.Knowledge == "" || len(request.Knowledge) > 128*1024 || !utf8.ValidString(request.Knowledge) || strings.Contains(request.Knowledge, "\r") || hasControl(request.Knowledge, true) {
		return Request{}, ErrInvalidProposal
	}
	if request.Target != "" && !canonicalPath(request.Target) {
		return Request{}, ErrInvalidProposal
	}
	if len(request.SourcePaths) > 16 {
		return Request{}, ErrInvalidProposal
	}
	for _, sourcePath := range request.SourcePaths {
		if !canonicalPath(sourcePath) {
			return Request{}, ErrInvalidProposal
		}
	}
	return request, nil
}

func canonicalPath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || hasControl(value, false) {
		return false
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if normalized != value || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func hasControl(value string, allowLayout bool) bool {
	for _, character := range value {
		if allowLayout && (character == '\n' || character == '\t') {
			continue
		}
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func identifier() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func slug(title string) string {
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(title) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && result.Len() > 0 && result.Len() < 64 {
				result.WriteByte('-')
			}
			if result.Len() < 64 {
				result.WriteRune(character)
			}
			separator = false
		} else if result.Len() > 0 {
			separator = true
		}
		if result.Len() == 64 {
			break
		}
	}
	value := strings.TrimSuffix(result.String(), "-")
	if value == "" {
		return "proposal"
	}
	return value
}

func render(proposal Proposal) []byte {
	var result strings.Builder
	result.WriteString("---\n")
	result.WriteString("schema_version: 1\n")
	result.WriteString("suggested_action: ")
	result.WriteString(strconv.Quote(string(proposal.SuggestedAction)))
	result.WriteByte('\n')
	if proposal.Target != "" {
		result.WriteString("target: ")
		result.WriteString(strconv.Quote(proposal.Target))
		result.WriteByte('\n')
	}
	if len(proposal.SourcePaths) == 0 {
		result.WriteString("source_paths: []\n")
	} else {
		result.WriteString("source_paths:\n")
		for _, sourcePath := range proposal.SourcePaths {
			result.WriteString("  - ")
			result.WriteString(strconv.Quote(sourcePath))
			result.WriteByte('\n')
		}
	}
	result.WriteString("---\n\n# ")
	result.WriteString(proposal.Title)
	result.WriteString("\n\n")
	result.WriteString(strings.TrimSuffix(proposal.Knowledge, "\n"))
	result.WriteByte('\n')
	return []byte(result.String())
}

func (inbox *Inbox) withLock(ctx context.Context, action func() error) error {
	if err := os.MkdirAll(inbox.stateRoot, 0o700); err != nil {
		return fmt.Errorf("create inbox state")
	}
	if err := os.Chmod(inbox.stateRoot, 0o700); err != nil {
		return fmt.Errorf("secure inbox state")
	}
	lockFD, err := unix.Open(filepath.Join(inbox.stateRoot, "inbox.lock"), unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return fmt.Errorf("open inbox lock")
	}
	lock := os.NewFile(uintptr(lockFD), "inbox.lock")
	if lock == nil {
		_ = unix.Close(lockFD)
		return fmt.Errorf("open inbox lock")
	}
	defer lock.Close()
	for {
		err := unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock inbox")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(lockFD, unix.LOCK_UN)
	return action()
}

func (inbox *Inbox) gitText(ctx context.Context, input []byte, arguments ...string) (string, error) {
	output, err := inbox.gitInput(ctx, input, arguments...)
	return strings.TrimSpace(string(output)), err
}

func (inbox *Inbox) gitTextEnv(ctx context.Context, environment map[string]string, input []byte, arguments ...string) (string, error) {
	output, err := inbox.gitInputEnv(ctx, environment, input, arguments...)
	return strings.TrimSpace(string(output)), err
}

func (inbox *Inbox) gitInput(ctx context.Context, input []byte, arguments ...string) ([]byte, error) {
	return inbox.gitInputEnv(ctx, nil, input, arguments...)
}

func (inbox *Inbox) gitInputEnv(ctx context.Context, extraEnvironment map[string]string, input []byte, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"--no-optional-locks", "--no-replace-objects", "-C", "/proc/self/fd/3"}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	command.Env = gitEnvironment(extraEnvironment)
	command.ExtraFiles = []*os.File{inbox.memoryRoot}
	command.Stdin = bytes.NewReader(input)
	return command.Output()
}

func (inbox *Inbox) gitPath(ctx context.Context, arguments ...string) (string, error) {
	value, err := inbox.gitText(ctx, nil, arguments...)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(inbox.rootPath, value)
	}
	return filepath.Clean(value), nil
}

func gitEnvironment(extra map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(extra))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			environment = append(environment, entry)
		}
	}
	for key, value := range extra {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func temporaryIndex(directory string) (string, error) {
	file, err := os.CreateTemp(directory, ".git-push-memories-index-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary Git index")
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close temporary Git index")
	}
	return name, nil
}

func readRegularFile(filePath string) ([]byte, os.FileMode, error) {
	fileDescriptor, err := unix.Open(filePath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, err
	}
	file := os.NewFile(uintptr(fileDescriptor), filePath)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return nil, 0, errors.New("open regular file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, 0, errors.New("not a regular file")
	}
	contents, err := io.ReadAll(file)
	return contents, info.Mode(), err
}

func within(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

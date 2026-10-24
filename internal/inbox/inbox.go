package inbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/reference"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionRemove Action = "remove"
)

var (
	ErrInvalidProposal = errors.New("invalid proposal")
	ErrProposalDrift   = errors.New("proposal drifted")
	ErrWrongBranch     = errors.New("memory worktree is not on the expected branch")
)

type Request struct {
	Title            string
	SuggestedAction  Action
	Target           string
	Knowledge        string
	SourcePaths      []string
	SourceReferences []reference.Reference
}

type Proposal struct {
	SchemaVersion    int                   `json:"schema_version"`
	ID               string                `json:"id"`
	Path             string                `json:"path"`
	Commit           string                `json:"commit"`
	Title            string                `json:"title"`
	SuggestedAction  Action                `json:"suggested_action"`
	Target           string                `json:"target,omitempty"`
	Knowledge        string                `json:"knowledge"`
	SourcePaths      []string              `json:"source_paths"`
	SourceReferences []reference.Reference `json:"source_references,omitempty"`
	Duplicate        bool                  `json:"duplicate,omitempty"`
}

type Inbox struct {
	memoryRoot *os.File
	rootPath   string
	stateRoot  string
	branch     string
	workspace  *workspace.Workspace // nil: read-only
}

// New opens a read-only inbox over a worktree that must be on branch.
func New(memoryRoot, stateRoot, branch string) (*Inbox, error) {
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
	return &Inbox{memoryRoot: root, rootPath: canonical, stateRoot: stateAbsolute, branch: branch}, nil
}

// NewWriter opens an inbox that can Add and Publish through ws.
func NewWriter(ws *workspace.Workspace, memoryRoot, branch string) (*Inbox, error) {
	opened, err := New(memoryRoot, ws.Paths.State, branch)
	if err != nil {
		return nil, err
	}
	opened.workspace = ws
	return opened, nil
}

func (inbox *Inbox) Close() error {
	return inbox.memoryRoot.Close()
}

// Add writes the proposal on the inbox branch and commits it, touching only its own path.
// The commit stays local until Publish. A byte-identical existing proposal is returned with Duplicate set.
func (inbox *Inbox) Add(ctx context.Context, request Request) (Proposal, error) {
	normalized, err := validate(request)
	if err != nil {
		return Proposal{}, err
	}
	if inbox.workspace == nil {
		return Proposal{}, errors.New("inbox is read-only")
	}
	var proposal Proposal
	err = inbox.withLock(ctx, func() error {
		if err := inbox.checkBranch(ctx); err != nil {
			return err
		}
		if err := workspace.Normalize(ctx, inbox.rootPath); err != nil {
			return err
		}
		// Deduplicate against what was already sent; offline, the local view is all there is.
		if err := inbox.workspace.Fetch(ctx); err == nil && inbox.workspace.RefExists(ctx, "refs/remotes/origin/"+inbox.branch) {
			if err := inbox.workspace.Merge(ctx, inbox.rootPath, "origin/"+inbox.branch); err != nil {
				return err
			}
		}
		schemaVersion := 1
		if len(normalized.SourceReferences) > 0 {
			schemaVersion = 2
		}
		candidate := Proposal{
			SchemaVersion: schemaVersion, Title: normalized.Title, SuggestedAction: normalized.SuggestedAction, Target: normalized.Target,
			Knowledge: normalized.Knowledge, SourcePaths: append([]string(nil), normalized.SourcePaths...), SourceReferences: append([]reference.Reference(nil), normalized.SourceReferences...),
		}
		candidateContents := render(candidate)
		existing, err := inbox.List(ctx)
		if err != nil {
			return err
		}
		for _, snapshot := range existing {
			if !bytes.Equal(snapshot.Contents, candidateContents) {
				continue
			}
			if err := inbox.Confirm(ctx, snapshot); err != nil {
				return err
			}
			commit, err := inbox.gitText(ctx, nil, "log", "--format=%H", "--diff-filter=A", "--max-count=1", "--", snapshot.Proposal.Path)
			if err != nil || commit == "" {
				return ErrInvalidProposal
			}
			proposal = snapshot.Proposal
			proposal.Commit = commit
			proposal.Duplicate = true
			return nil
		}
		id, err := identifier()
		if err != nil {
			return fmt.Errorf("create proposal identifier")
		}
		proposal = Proposal{
			SchemaVersion:    schemaVersion,
			ID:               id,
			Path:             path.Join("proposals", id+"-"+slug(normalized.Title)+".md"),
			Title:            normalized.Title,
			SuggestedAction:  normalized.SuggestedAction,
			Target:           normalized.Target,
			Knowledge:        normalized.Knowledge,
			SourcePaths:      append([]string(nil), normalized.SourcePaths...),
			SourceReferences: append([]reference.Reference(nil), normalized.SourceReferences...),
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
		committed := false
		defer func() {
			if !committed {
				_ = root.Remove(proposal.Path)
			}
		}()
		if _, err := file.Write(contents); err != nil {
			_ = file.Close()
			return fmt.Errorf("write proposal file")
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close proposal file")
		}
		if err := inbox.workspace.Commit(ctx, inbox.rootPath, "Add memory proposal: "+proposal.Title, proposal.Path); err != nil {
			return fmt.Errorf("commit proposal: %w", err)
		}
		committed = true
		commit, err := inbox.gitText(ctx, nil, "rev-parse", "HEAD")
		if err != nil || commit == "" {
			return fmt.Errorf("read proposal commit")
		}
		changed, err := inbox.gitInput(ctx, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", commit)
		if err != nil || !bytes.Equal(changed, []byte(proposal.Path+"\x00")) {
			return fmt.Errorf("verify proposal commit")
		}
		stored, err := inbox.gitInput(ctx, nil, "show", commit+":"+proposal.Path)
		if err != nil || !bytes.Equal(stored, contents) {
			return fmt.Errorf("verify proposal contents")
		}
		proposal.Commit = commit
		return nil
	})
	if err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// Publish sends local proposal commits to the remote. pushed is false when nothing was sent
// because the network failed; the commits stay local and the next call retries.
func (inbox *Inbox) Publish(ctx context.Context) (pushed bool, err error) {
	if inbox.workspace == nil {
		return false, errors.New("inbox is read-only")
	}
	err = inbox.withLock(ctx, func() error {
		if err := inbox.checkBranch(ctx); err != nil {
			return err
		}
		if err := workspace.Normalize(ctx, inbox.rootPath); err != nil {
			return err
		}
		err := inbox.workspace.Push(ctx, inbox.rootPath, inbox.branch)
		if errors.Is(err, workspace.ErrNetwork) {
			return nil
		}
		if err != nil {
			return err
		}
		pushed = true
		return nil
	})
	return pushed, err
}

func (inbox *Inbox) checkBranch(ctx context.Context) error {
	branch, err := inbox.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != inbox.branch {
		return ErrWrongBranch
	}
	return nil
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
	if len(request.SourceReferences) > reference.MaxReferences || len(request.SourcePaths) > 0 && len(request.SourceReferences) > 0 {
		return Request{}, ErrInvalidProposal
	}
	if len(request.SourceReferences) > 0 && reference.ValidateCollection(request.SourceReferences) != nil {
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
	result.WriteString("schema_version: ")
	result.WriteString(strconv.Itoa(proposal.SchemaVersion))
	result.WriteByte('\n')
	result.WriteString("suggested_action: ")
	result.WriteString(strconv.Quote(string(proposal.SuggestedAction)))
	result.WriteByte('\n')
	if proposal.Target != "" {
		result.WriteString("target: ")
		result.WriteString(strconv.Quote(proposal.Target))
		result.WriteByte('\n')
	}
	if proposal.SchemaVersion == 2 {
		result.WriteString("source_references:\n")
		for _, sourceReference := range proposal.SourceReferences {
			result.WriteString("  - repository: ")
			result.WriteString(strconv.Quote(sourceReference.Repository))
			result.WriteString("\n    revision: ")
			result.WriteString(strconv.Quote(sourceReference.Revision))
			result.WriteString("\n    path: ")
			result.WriteString(strconv.Quote(sourceReference.Path))
			result.WriteString("\n    content_hash: ")
			result.WriteString(strconv.Quote(sourceReference.ContentHash))
			result.WriteByte('\n')
		}
	} else if len(proposal.SourcePaths) == 0 {
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
	return repolock.With(ctx, inbox.stateRoot, action)
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

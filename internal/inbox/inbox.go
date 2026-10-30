package inbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

var (
	ErrInvalidProposal = errors.New("invalid proposal")
	ErrProposalDrift   = errors.New("proposal drifted")
	ErrWrongBranch     = errors.New("memory worktree is not on the expected branch")
)

// Request is a proposal as the model wrote it: Title names the file and the commit, Content is free Markdown.
type Request struct {
	Title   string
	Content string
}

type Proposal struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Commit  string `json:"commit"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type Inbox struct {
	rootPath  string
	stateRoot string
	branch    string
	workspace *workspace.Workspace // nil: read-only
}

// New opens a read-only inbox over a worktree that must be on branch.
func New(memoryRoot, stateRoot, branch string) (*Inbox, error) {
	canonical, err := filepath.EvalSymlinks(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("invalid memory root")
	}
	stateAbsolute, err := filepath.Abs(stateRoot)
	if err != nil || stateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	return &Inbox{rootPath: canonical, stateRoot: stateAbsolute, branch: branch}, nil
}

// NewWriter opens an inbox that can Add through ws; the repository must be on GitHub.
func NewWriter(ws *workspace.Workspace, memoryRoot, branch string) (*Inbox, error) {
	opened, err := New(memoryRoot, ws.Paths.State, branch)
	if err != nil {
		return nil, err
	}
	opened.workspace = ws
	return opened, nil
}

// Add commits the proposal to the remote inbox branch through the forge API: one call, all or nothing,
// with no local worktree involved. The local worktree catches up on the next Refresh.
func (inbox *Inbox) Add(ctx context.Context, request Request) (Proposal, error) {
	if inbox.workspace == nil {
		return Proposal{}, errors.New("inbox is read-only")
	}
	title := strings.TrimSpace(request.Title)
	if title == "" || strings.TrimSpace(request.Content) == "" {
		return Proposal{}, ErrInvalidProposal
	}
	id, err := identifier()
	if err != nil {
		return Proposal{}, fmt.Errorf("create proposal identifier")
	}
	content := strings.TrimSuffix(request.Content, "\n") + "\n"
	proposal := Proposal{ID: id, Path: path.Join("proposals", id+"-"+slug(title)+".md"), Title: title, Content: content}
	client, err := github.New(inbox.workspace.URL)
	if err != nil {
		return Proposal{}, err
	}
	proposal.Commit, err = client.CreateFile(ctx, inbox.branch, proposal.Path, "Add memory proposal: "+title, []byte(content))
	if errors.Is(err, github.ErrPermission) {
		return Proposal{}, workspace.ErrPermission
	}
	if err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// Refresh fast-forwards the local inbox worktree to the remote branch so List sees every pushed proposal.
func (inbox *Inbox) Refresh(ctx context.Context) error {
	if inbox.workspace == nil {
		return errors.New("inbox is read-only")
	}
	return repolock.With(ctx, inbox.stateRoot, func() error {
		if err := inbox.checkBranch(ctx); err != nil {
			return err
		}
		if err := workspace.Normalize(ctx, inbox.rootPath); err != nil {
			return err
		}
		if err := inbox.workspace.Fetch(ctx); err != nil {
			return err
		}
		return inbox.workspace.Merge(ctx, inbox.rootPath, "origin/"+inbox.branch)
	})
}

func (inbox *Inbox) git() workspace.Git { return workspace.Git{Dir: inbox.rootPath} }

func (inbox *Inbox) checkBranch(ctx context.Context) error {
	branch, err := inbox.git().Output(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != inbox.branch {
		return ErrWrongBranch
	}
	return nil
}

func identifier() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// slug turns a title into a file-name-safe fragment: a-z0-9 and dashes, up to 64 characters, "proposal" if empty.
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

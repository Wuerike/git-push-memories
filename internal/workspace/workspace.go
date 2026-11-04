// Package workspace owns the on-disk layout of a memory repository: a bare
// clone plus the read, proposals and promotion worktrees, and every Git
// operation that moves them. No other package builds these paths or branch names.
package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Wuerike/git-push-memories/internal/repolock"
)

const (
	MainBranch      = "main"
	ProposalsBranch = "proposals"
)

// FetchTTL is how long a fetch attempt keeps the read worktree fresh.
const FetchTTL = 10 * time.Minute

// Paths are the absolute locations under <state>/<repo-id>.
type Paths struct {
	State      string // <state>/<repo-id>; also the repolock root
	Repo       string // bare clone
	Read       string
	Proposals  string
	Promotions string
	Published  string
}

func RepoID(url string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(url)))[:16]
}

// StateBase is $XDG_STATE_HOME/git-push-memories.
func StateBase() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Abs(filepath.Join(root, "git-push-memories"))
}

func PathsFor(url string) (Paths, error) {
	base, err := StateBase()
	if err != nil {
		return Paths{}, err
	}
	state := filepath.Join(base, RepoID(url))
	return Paths{
		State:      state,
		Repo:       filepath.Join(state, "repo"),
		Read:       filepath.Join(state, "read"),
		Proposals:  filepath.Join(state, "proposals"),
		Promotions: filepath.Join(state, "promotions"),
		Published:  filepath.Join(state, "promotion-published.json"),
	}, nil
}

type Workspace struct {
	URL   string
	Paths Paths
}

// PromotionPaths locates a fresh worktree and its separate, read-only inputs.
type PromotionPaths struct {
	Worktree, InputsRoot, Branch string
}

// NewPromotion creates a new branch at base without moving any existing worktree.
func (w *Workspace) NewPromotion(ctx context.Context, base string) (PromotionPaths, error) {
	id := time.Now().UTC().Format("20060102T150405Z") + "-" + strings.ToLower(rand.Text()[:8])
	root := filepath.Join(w.Paths.Promotions, id)
	paths := PromotionPaths{
		Worktree:   filepath.Join(root, "worktree"),
		InputsRoot: filepath.Join(root, "inputs"),
		Branch:     "promote/" + id,
	}
	if err := os.MkdirAll(paths.InputsRoot, 0o700); err != nil {
		return PromotionPaths{}, err
	}
	if _, err := w.Repo().Output(ctx, "worktree", "add", "-b", paths.Branch, paths.Worktree, base); err != nil {
		return PromotionPaths{}, err
	}
	return paths, nil
}

func Open(url string) (*Workspace, error) {
	paths, err := PathsFor(url)
	if err != nil {
		return nil, errors.New("state root is unavailable")
	}
	return &Workspace{URL: url, Paths: paths}, nil
}

func (w *Workspace) Repo() Git { return Git{Dir: w.Paths.Repo} }

// Cloned reports whether the bare clone exists.
func (w *Workspace) Cloned() bool {
	_, err := os.Stat(filepath.Join(w.Paths.Repo, "HEAD"))
	return err == nil
}

// Clone creates the bare clone and fetches into refs/remotes/origin/*. It is a no-op when already cloned.
func (w *Workspace) Clone(ctx context.Context) error {
	if w.Cloned() {
		return nil
	}
	if err := os.MkdirAll(w.Paths.State, 0o700); err != nil {
		return errors.New("repository state could not be created")
	}
	if err := os.RemoveAll(w.Paths.Repo); err != nil {
		return errors.New("repository state could not be reset")
	}
	if _, err := (Git{Dir: w.Paths.State}).Net(ctx, "clone", "--bare", "--", w.URL, w.Paths.Repo); err != nil {
		os.RemoveAll(w.Paths.Repo)
		return err
	}
	git := w.Repo()
	if _, err := git.Output(ctx, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	// A bare clone copies the remote branches as local ones; only origin/* is meaningful here.
	if heads, err := w.LocalBranches(ctx, ""); err == nil {
		for _, head := range heads {
			_, _ = git.Output(ctx, "update-ref", "-d", "refs/heads/"+head)
		}
	}
	return w.Fetch(ctx)
}

func (w *Workspace) Fetch(ctx context.Context) error {
	_, err := w.Repo().Net(ctx, "fetch", "--prune", "origin")
	return err
}

// FetchIfStale fetches unless an attempt happened within ttl. The attempt time
// is recorded before the fetch, so a failing remote is retried once per ttl.
func (w *Workspace) FetchIfStale(ctx context.Context, ttl time.Duration) (bool, error) {
	stamp := filepath.Join(w.Paths.State, "fetch-attempt")
	if info, err := os.Stat(stamp); err == nil && time.Since(info.ModTime()) < ttl {
		return false, nil
	}
	if err := os.WriteFile(stamp, nil, 0o600); err != nil {
		return false, errors.New("fetch timestamp could not be written")
	}
	return true, w.Fetch(ctx)
}

// RefExists reports whether a ref resolves in the bare clone.
func (w *Workspace) RefExists(ctx context.Context, ref string) bool {
	return w.Repo().Ok(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// EnsureWorktree creates dir as a worktree on branch, starting at start, when it does not exist yet.
func (w *Workspace) EnsureWorktree(ctx context.Context, dir, branch, start string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	_, _ = w.Repo().Output(ctx, "worktree", "prune")
	if w.RefExists(ctx, "refs/heads/"+branch) {
		// Keep local commits that were never pushed.
		_, err := w.Repo().Output(ctx, "worktree", "add", dir, branch)
		return err
	}
	_, err := w.Repo().Output(ctx, "worktree", "add", "-b", branch, dir, start)
	return err
}

// EnsureRead creates the detached read worktree on origin/main.
func (w *Workspace) EnsureRead(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(w.Paths.Read, ".git")); err == nil {
		return nil
	}
	_, _ = w.Repo().Output(ctx, "worktree", "prune")
	_, err := w.Repo().Output(ctx, "worktree", "add", "--detach", "-f", w.Paths.Read, "origin/main")
	return err
}

// UpdateRead moves the read worktree to origin/main under the repository lock.
// It also takes the directory lock memory readers share, so a reader never sees a half-written checkout.
func (w *Workspace) UpdateRead(ctx context.Context) error {
	return repolock.With(ctx, w.Paths.State, func() error { return w.UpdateReadLocked(ctx) })
}

// UpdateReadLocked is UpdateRead for callers that already hold the repository lock.
func (w *Workspace) UpdateReadLocked(ctx context.Context) error {
	dir, err := os.Open(w.Paths.Read)
	if err != nil {
		return errors.New("read worktree is missing; run setup")
	}
	defer dir.Close()
	for {
		err := unix.Flock(int(dir.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(int(dir.Fd()), unix.LOCK_UN)
	_, err = Git{Dir: w.Paths.Read}.Output(ctx, "checkout", "--detach", "-f", "origin/main")
	return err
}

// Refresh fetches when the TTL expired and, on success, moves the read worktree.
func (w *Workspace) Refresh(ctx context.Context) error {
	ran, err := w.FetchIfStale(ctx, FetchTTL)
	if !ran || err != nil {
		return err
	}
	return w.UpdateRead(ctx)
}

var refreshing atomic.Bool

// ResolveRead returns the read worktree root right away and refreshes it in the
// background when the fetch TTL has expired.
func (w *Workspace) ResolveRead() (string, error) {
	if _, err := os.Stat(filepath.Join(w.Paths.Read, ".git")); err != nil {
		return "", errors.New("memory repository is not prepared; run setup")
	}
	if refreshing.CompareAndSwap(false, true) {
		go func() {
			defer refreshing.Store(false)
			_ = w.Refresh(context.Background())
		}()
	}
	return w.Paths.Read, nil
}

// LocalBranches lists local branch names starting with prefix.
func (w *Workspace) LocalBranches(ctx context.Context, prefix string) ([]string, error) {
	out, err := w.Repo().Output(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+prefix)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Normalize prepares a worktree for writing: a pending merge is aborted, any
// other uncommitted change is ErrDirty.
func Normalize(ctx context.Context, dir string) error {
	git := Git{Dir: dir}
	if git.Ok(ctx, "rev-parse", "-q", "--verify", "MERGE_HEAD") {
		if _, err := git.Output(ctx, "merge", "--abort"); err != nil {
			return err
		}
	}
	status, err := git.Output(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return ErrDirty
	}
	return nil
}

func (w *Workspace) identity(ctx context.Context) ([]string, error) {
	name, err := w.Repo().Output(ctx, "config", "user.name")
	if err != nil || name == "" {
		return nil, errors.New("Git identity is required: set user.name and user.email")
	}
	email, err := w.Repo().Output(ctx, "config", "user.email")
	if err != nil || email == "" {
		return nil, errors.New("Git identity is required: set user.name and user.email")
	}
	return []string{"-c", "user.name=" + name, "-c", "user.email=" + email,
		"-c", "commit.gpgsign=false", "-c", "commit.template=", "-c", "merge.ff=true"}, nil
}

// Commit stages paths (additions and removals) and commits only them, with a neutral configuration.
func (w *Workspace) Commit(ctx context.Context, dir, message string, paths ...string) error {
	identity, err := w.identity(ctx)
	if err != nil {
		return err
	}
	git := Git{Dir: dir}
	if _, err := git.Output(ctx, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return err
	}
	arguments := append(identity, "commit", "--no-verify", "--no-edit", "-m", message, "--")
	_, err = git.Output(ctx, append(arguments, paths...)...)
	return err
}

// Merge merges ref into the worktree's branch without opening an editor.
// A conflict is aborted and reported as ErrConflict.
func (w *Workspace) Merge(ctx context.Context, dir, ref string) error {
	identity, err := w.identity(ctx)
	if err != nil {
		return err
	}
	git := Git{Dir: dir}
	if _, err := git.Output(ctx, append(identity, "merge", "--no-verify", "--no-edit", ref)...); err != nil {
		_, _ = git.Output(ctx, "merge", "--abort")
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return nil
}

// PushCAS pushes branch without force. A rejection because the remote moved is
// ErrRejected; otherwise the failure is ErrPermission or ErrNetwork.
func (w *Workspace) PushCAS(ctx context.Context, dir, branch string) error {
	_, err := Git{Dir: dir}.Net(ctx, "push", "origin", "refs/heads/"+branch+":refs/heads/"+branch)
	if err == nil {
		return nil
	}
	if fetchErr := w.Fetch(ctx); fetchErr != nil {
		return ErrNetwork
	}
	remote := "refs/remotes/origin/" + branch
	if w.RefExists(ctx, remote) && !(Git{Dir: dir}).Ok(ctx, "merge-base", "--is-ancestor", remote, "HEAD") {
		return ErrRejected
	}
	return ErrPermission
}

// Push pushes branch; on a race it merges origin/<branch> (never rebase) and retries.
func (w *Workspace) Push(ctx context.Context, dir, branch string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = w.PushCAS(ctx, dir, branch); !errors.Is(err, ErrRejected) {
			return err
		}
		if err = w.Merge(ctx, dir, "origin/"+branch); err != nil {
			return err
		}
	}
	return err
}

// ShowFile returns the contents of path at ref in the bare clone.
func (w *Workspace) ShowFile(ctx context.Context, ref, path string) ([]byte, error) {
	out, err := w.Repo().run(ctx, nil, "show", ref+":"+path)
	return []byte(out), err
}

// HasRemoteRefs reports whether any remote-tracking branch exists after a fetch.
func (w *Workspace) HasRemoteRefs(ctx context.Context) bool {
	out, err := w.Repo().Output(ctx, "for-each-ref", "--count=1", "refs/remotes/origin/")
	return err == nil && out != ""
}

// CreateMain makes the first commit of main from files, using plumbing so no worktree is needed, and pushes it.
func (w *Workspace) CreateMain(ctx context.Context, files map[string][]byte) error {
	identity, err := w.identity(ctx)
	if err != nil {
		return err
	}
	git := w.Repo()
	var entries strings.Builder
	for name, contents := range files {
		blob, err := git.run(ctx, contents, "hash-object", "-w", "--stdin")
		if err != nil {
			return err
		}
		fmt.Fprintf(&entries, "100644 blob %s\t%s\n", blob, name)
	}
	tree, err := git.run(ctx, []byte(entries.String()), "mktree")
	if err != nil {
		return err
	}
	commit, err := git.Output(ctx, append(identity, "commit-tree", tree, "-m", "chore: initialize memory repository")...)
	if err != nil {
		return err
	}
	if _, err := git.Output(ctx, "update-ref", "refs/heads/"+MainBranch, commit); err != nil {
		return err
	}
	if _, err := git.Net(ctx, "push", "origin", "refs/heads/"+MainBranch+":refs/heads/"+MainBranch); err != nil {
		if w.Fetch(ctx) != nil {
			return ErrNetwork
		}
		return ErrPermission
	}
	return w.Fetch(ctx)
}

// CheckIdentity fails unless Git has user.name and user.email configured.
func CheckIdentity(ctx context.Context) error {
	git := Git{Dir: os.TempDir()}
	for _, key := range []string{"user.name", "user.email"} {
		if value, err := git.Output(ctx, "config", key); err != nil || value == "" {
			return errors.New("Git identity is required: set user.name and user.email")
		}
	}
	return nil
}

// Rev resolves ref to a commit in dir.
func Rev(ctx context.Context, dir, ref string) (string, error) {
	return Git{Dir: dir}.Output(ctx, "rev-parse", "--verify", ref+"^{commit}")
}

package curation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
	"golang.org/x/sys/unix"
)

var (
	ErrDrift       = errors.New("curation preview drifted")
	ErrLocked      = errors.New("Git index is locked")
	ErrUnsupported = errors.New("proposal action is not supported by this curation phase")
)

type Summary struct {
	ID              string       `json:"id"`
	Path            string       `json:"path"`
	Title           string       `json:"title"`
	SuggestedAction inbox.Action `json:"suggested_action"`
	Target          string       `json:"target,omitempty"`
	Knowledge       string       `json:"knowledge"`
	SourcePaths     []string     `json:"source_paths"`
}

type List struct {
	Categories []string  `json:"categories"`
	Proposals  []Summary `json:"proposals"`
}

type Preview struct {
	ProposalID   string       `json:"proposal_id"`
	ProposalPath string       `json:"proposal_path"`
	Title        string       `json:"title"`
	Action       inbox.Action `json:"suggested_action"`
	Knowledge    string       `json:"knowledge"`
	SourcePaths  []string     `json:"source_paths"`
	TargetPath   string       `json:"target_path"`
	SectionID    string       `json:"section_id"`
	Diff         string       `json:"diff"`

	head          string
	postStateHash string
	postState     []byte
	proposal      inbox.Snapshot
}

type Result struct {
	ProposalID string `json:"proposal_id"`
	TargetPath string `json:"target_path"`
	SectionID  string `json:"section_id"`
	Commit     string `json:"commit"`
}

type Curator struct {
	cfg        config.Config
	root       *os.File
	rootPath   string
	inbox      *inbox.Inbox
	repository *memory.Repository
	guard      *intake.Guard
}

func New(cfg config.Config) (*Curator, error) {
	rootPath, err := filepath.EvalSymlinks(cfg.MemoryRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	root, err := os.Open(rootPath)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	proposalInbox, err := inbox.New(rootPath, cfg.StateRoot)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	repository, err := memory.Open(rootPath)
	if err != nil {
		_ = proposalInbox.Close()
		_ = root.Close()
		return nil, err
	}
	guard, err := intake.New()
	if err != nil {
		_ = repository.Close()
		_ = proposalInbox.Close()
		_ = root.Close()
		return nil, err
	}
	return &Curator{cfg: cfg, root: root, rootPath: rootPath, inbox: proposalInbox, repository: repository, guard: guard}, nil
}

func (curator *Curator) Close() error {
	repositoryErr := curator.repository.Close()
	inboxErr := curator.inbox.Close()
	rootErr := curator.root.Close()
	if repositoryErr != nil {
		return repositoryErr
	}
	if inboxErr != nil {
		return inboxErr
	}
	return rootErr
}

func (curator *Curator) List(ctx context.Context) (List, error) {
	snapshots, err := curator.inbox.List(ctx)
	if err != nil {
		return List{}, err
	}
	result := List{Categories: append([]string(nil), curator.cfg.MemoryCategories...), Proposals: make([]Summary, len(snapshots))}
	for index, snapshot := range snapshots {
		proposal := snapshot.Proposal
		if err := curator.inspect(proposal); err != nil {
			return List{}, err
		}
		result.Proposals[index] = Summary{
			ID: proposal.ID, Path: proposal.Path, Title: proposal.Title, SuggestedAction: proposal.SuggestedAction, Target: proposal.Target,
			Knowledge: proposal.Knowledge, SourcePaths: append([]string(nil), proposal.SourcePaths...),
		}
	}
	return result, nil
}

func (curator *Curator) PreviewCreate(ctx context.Context, proposalID, targetPath string) (Preview, error) {
	snapshot, err := curator.inbox.Read(ctx, proposalID)
	if err != nil {
		return Preview{}, err
	}
	return curator.previewCreate(ctx, snapshot, targetPath)
}

func (curator *Curator) previewCreate(ctx context.Context, snapshot inbox.Snapshot, targetPath string) (Preview, error) {
	if snapshot.Proposal.SuggestedAction != inbox.ActionCreate {
		return Preview{}, ErrUnsupported
	}
	if err := curator.confirmConfig(ctx, snapshot.Head); err != nil {
		return Preview{}, err
	}
	if !curator.cfg.IsMemoryPath(targetPath) || !memory.IsCreatePagePath(targetPath) {
		return Preview{}, memory.ErrInvalidPath
	}
	if err := curator.inbox.Confirm(ctx, snapshot); err != nil {
		return Preview{}, err
	}
	if err := curator.inspect(snapshot.Proposal); err != nil {
		return Preview{}, err
	}
	page, err := curator.repository.PreviewCreatePage(ctx, memory.CreatePageRequest{
		TargetPath: targetPath, Title: snapshot.Proposal.Title, Body: snapshot.Proposal.Knowledge,
	})
	if err != nil {
		return Preview{}, err
	}
	if page.Head != snapshot.Head {
		return Preview{}, ErrDrift
	}
	return Preview{
		ProposalID: snapshot.Proposal.ID, ProposalPath: snapshot.Proposal.Path, Title: snapshot.Proposal.Title,
		Action: snapshot.Proposal.SuggestedAction, Knowledge: snapshot.Proposal.Knowledge, SourcePaths: append([]string(nil), snapshot.Proposal.SourcePaths...),
		TargetPath: page.Target.Path, SectionID: page.Target.SectionID,
		Diff: page.Diff + "\n" + unifieddiff.Delete(snapshot.Proposal.Path, snapshot.Contents),
		head: page.Head, postStateHash: page.PostStateHash, postState: append([]byte(nil), page.PostState...), proposal: snapshot,
	}, nil
}

func (curator *Curator) confirmConfig(ctx context.Context, expectedHead string) error {
	current, err := config.Resolve(curator.rootPath)
	if err != nil || current.MemoryRoot != curator.cfg.MemoryRoot || !slices.Equal(current.MemoryCategories, curator.cfg.MemoryCategories) {
		return ErrDrift
	}
	for _, arguments := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "--cached", expectedHead, "--", "gpm.json"},
		{"diff", "--quiet", "--no-ext-diff", "--", "gpm.json"},
	} {
		if _, err := curator.gitInput(ctx, nil, nil, arguments...); err != nil {
			return ErrDrift
		}
	}
	head, err := curator.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != expectedHead {
		return ErrDrift
	}
	return nil
}

func (curator *Curator) ApplyCreate(ctx context.Context, expected Preview) (Result, error) {
	var result Result
	err := repolock.With(ctx, curator.cfg.StateRoot, func() error {
		fresh, err := curator.previewCreate(ctx, expected.proposal, expected.TargetPath)
		if err != nil {
			return err
		}
		if !samePreview(fresh, expected) {
			return ErrDrift
		}
		commit, err := curator.commitCreate(ctx, fresh)
		if err != nil {
			return err
		}
		result = Result{ProposalID: fresh.ProposalID, TargetPath: fresh.TargetPath, SectionID: fresh.SectionID, Commit: commit}
		return nil
	})
	return result, err
}

func samePreview(left, right Preview) bool {
	return left.ProposalID == right.ProposalID && left.ProposalPath == right.ProposalPath && left.Title == right.Title && left.Action == right.Action && left.Knowledge == right.Knowledge &&
		slices.Equal(left.SourcePaths, right.SourcePaths) && left.TargetPath == right.TargetPath && left.SectionID == right.SectionID && left.Diff == right.Diff &&
		left.head == right.head && left.postStateHash == right.postStateHash && bytes.Equal(left.postState, right.postState) &&
		left.proposal.Head == right.proposal.Head && left.proposal.Blob == right.proposal.Blob && bytes.Equal(left.proposal.Contents, right.proposal.Contents)
}

func (curator *Curator) inspect(proposal inbox.Proposal) error {
	candidate := struct {
		Title           string   `json:"title"`
		SuggestedAction string   `json:"suggested_action"`
		Target          string   `json:"target,omitempty"`
		Knowledge       string   `json:"knowledge"`
		SourcePaths     []string `json:"source_paths"`
	}{proposal.Title, string(proposal.SuggestedAction), proposal.Target, proposal.Knowledge, proposal.SourcePaths}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	_, err = curator.guard.InspectProposal(intake.NewProposalCandidate(raw))
	return err
}

func (curator *Curator) commitCreate(ctx context.Context, preview Preview) (string, error) {
	gitDirectory, err := curator.gitPath(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("resolve Git directory")
	}
	indexPath, err := curator.gitPath(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil || !within(indexPath, gitDirectory) {
		return "", fmt.Errorf("resolve Git index")
	}
	indexLockPath := indexPath + ".lock"
	indexLock, err := os.OpenFile(indexLockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return "", ErrLocked
	}
	if err != nil {
		return "", fmt.Errorf("acquire Git index lock")
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
		return "", fmt.Errorf("read Git index")
	}
	branch, err := curator.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != "main" {
		return "", inbox.ErrNotMain
	}
	head, err := curator.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != preview.head {
		return "", ErrDrift
	}
	if err := curator.inbox.Confirm(ctx, preview.proposal); err != nil {
		return "", err
	}

	blob, err := curator.gitText(ctx, preview.postState, "hash-object", "-w", "--stdin")
	if err != nil || blob == "" {
		return "", fmt.Errorf("write memory blob")
	}
	commitIndex, err := temporaryIndex(curator.cfg.StateRoot)
	if err != nil {
		return "", err
	}
	defer os.Remove(commitIndex)
	commitEnvironment := map[string]string{"GIT_INDEX_FILE": commitIndex}
	if _, err := curator.gitInput(ctx, commitEnvironment, nil, "read-tree", head); err != nil {
		return "", fmt.Errorf("initialize curation tree")
	}
	cacheInfo := "100644," + blob + "," + preview.TargetPath
	if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return "", fmt.Errorf("add memory to curation tree")
	}
	if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--force-remove", "--", preview.ProposalPath); err != nil {
		return "", fmt.Errorf("remove proposal from curation tree")
	}
	treeOutput, err := curator.gitInput(ctx, commitEnvironment, nil, "write-tree")
	tree := strings.TrimSpace(string(treeOutput))
	if err != nil || tree == "" {
		return "", fmt.Errorf("write curation tree")
	}
	message := []byte("Curate memory proposal\n\nGit-Push-Memories-Proposal: " + preview.ProposalID + "\n")
	commit, err := curator.gitText(ctx, message, "commit-tree", tree, "-p", head)
	if err != nil || commit == "" {
		return "", fmt.Errorf("write curation commit")
	}
	if err := curator.verifyCommit(ctx, commit, preview, blob); err != nil {
		return "", err
	}

	finalIndex, err := temporaryIndex(curator.cfg.StateRoot)
	if err != nil {
		return "", err
	}
	defer os.Remove(finalIndex)
	if err := os.WriteFile(finalIndex, originalIndex, indexMode.Perm()); err != nil {
		return "", fmt.Errorf("copy Git index")
	}
	finalEnvironment := map[string]string{"GIT_INDEX_FILE": finalIndex}
	if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return "", fmt.Errorf("reconcile memory index entry")
	}
	if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--force-remove", "--", preview.ProposalPath); err != nil {
		return "", fmt.Errorf("reconcile proposal index entry")
	}
	finalBytes, _, err := readRegularFile(finalIndex)
	if err != nil {
		return "", fmt.Errorf("read reconciled Git index")
	}
	if err := indexLock.Chmod(indexMode.Perm()); err != nil {
		return "", fmt.Errorf("set Git index lock mode")
	}
	if _, err := indexLock.Write(finalBytes); err != nil {
		return "", fmt.Errorf("write Git index lock")
	}
	if err := indexLock.Sync(); err != nil {
		return "", fmt.Errorf("sync Git index lock")
	}
	if err := indexLock.Close(); err != nil {
		return "", fmt.Errorf("close Git index lock")
	}

	reconciled, err := curator.repository.ReconcileCreatePage(ctx, memory.ReconcileCreatePageRequest{
		CreatePageRequest: memory.CreatePageRequest{TargetPath: preview.TargetPath, Title: preview.Title, Body: preview.proposal.Proposal.Knowledge},
		PostStateHash:     preview.postStateHash, Head: preview.head,
		Preflight: func() error {
			if err := curator.inbox.Confirm(ctx, preview.proposal); err != nil {
				return err
			}
			return curator.inspect(preview.proposal.Proposal)
		},
		Validate: func(postState []byte) error {
			if !bytes.Equal(postState, preview.postState) {
				return ErrDrift
			}
			return nil
		},
		Mode: memory.ApplyBaseline,
	})
	if err != nil || reconciled.State != memory.ReconcilePostState {
		return "", err
	}
	if _, err := curator.gitInput(ctx, nil, nil, "update-ref", "refs/heads/main", commit, head); err != nil {
		_ = curator.removeExact(preview.TargetPath, preview.postState)
		return "", ErrDrift
	}
	if err := os.Rename(indexLockPath, indexPath); err != nil {
		return "", fmt.Errorf("install Git index")
	}
	lockInstalled = true
	if err := syncDirectory(gitDirectory); err != nil {
		return "", fmt.Errorf("sync Git directory")
	}
	if err := curator.removeExact(preview.ProposalPath, preview.proposal.Contents); err != nil {
		return "", fmt.Errorf("remove consumed proposal")
	}
	return commit, nil
}

func (curator *Curator) verifyCommit(ctx context.Context, commit string, preview Preview, blob string) error {
	changed, err := curator.gitInput(ctx, nil, nil, "diff-tree", "--no-commit-id", "--name-status", "-r", commit)
	if err != nil {
		return fmt.Errorf("verify curation commit")
	}
	want := []string{"A\t" + preview.TargetPath, "D\t" + preview.ProposalPath}
	got := strings.Split(strings.TrimSpace(string(changed)), "\n")
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("verify curation paths")
	}
	stored, err := curator.gitText(ctx, nil, "rev-parse", commit+":"+preview.TargetPath)
	if err != nil || stored != blob {
		return fmt.Errorf("verify curation memory")
	}
	if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", commit+":"+preview.ProposalPath); err == nil {
		return fmt.Errorf("verify proposal consumption")
	}
	return nil
}

func (curator *Curator) removeExact(relativePath string, expected []byte) error {
	root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", curator.root.Fd()))
	if err != nil {
		return ErrDrift
	}
	defer root.Close()
	file, err := root.Open(relativePath)
	if err != nil {
		return ErrDrift
	}
	contents, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(contents, expected) {
		return ErrDrift
	}
	if err := root.Remove(relativePath); err != nil {
		return ErrDrift
	}
	return syncDirectory(filepath.Join(curator.rootPath, filepath.Dir(relativePath)))
}

func (curator *Curator) gitPath(ctx context.Context, arguments ...string) (string, error) {
	value, err := curator.gitText(ctx, nil, arguments...)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(curator.rootPath, value)
	}
	return filepath.Clean(value), nil
}

func (curator *Curator) gitText(ctx context.Context, input []byte, arguments ...string) (string, error) {
	output, err := curator.gitInput(ctx, nil, input, arguments...)
	return strings.TrimSpace(string(output)), err
}

func (curator *Curator) gitInput(ctx context.Context, extraEnvironment map[string]string, input []byte, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"--no-optional-locks", "--no-replace-objects", "-C", "/proc/self/fd/3"}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	command.Env = gitEnvironment(extraEnvironment)
	command.ExtraFiles = []*os.File{curator.root}
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

func temporaryIndex(directory string) (string, error) {
	file, err := os.CreateTemp(directory, ".git-push-memories-curation-index-*.tmp")
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

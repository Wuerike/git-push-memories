package publisher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/proposal"
	"golang.org/x/sys/unix"
)

var (
	ErrDrift  = errors.New("publication snapshot drifted")
	ErrLocked = errors.New("Git index is locked")
)

var validProposalID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var validGitObject = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type Publisher struct {
	root      *os.File
	rootPath  string
	stateRoot string
	now       func() time.Time
	fault     func(string) error
}

func New(memoryRoot, stateRoot string) (*Publisher, error) {
	absoluteRoot, err := filepath.Abs(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	rootPath, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	root, err := os.Open(rootPath)
	if err != nil {
		return nil, fmt.Errorf("memory root is invalid")
	}
	info, err := root.Stat()
	if err != nil || !info.IsDir() {
		_ = root.Close()
		return nil, fmt.Errorf("memory root is invalid")
	}
	statePath, err := canonicalFuturePath(stateRoot)
	if err != nil || within(statePath, rootPath) || within(rootPath, statePath) {
		_ = root.Close()
		return nil, fmt.Errorf("state root must be outside memory root")
	}
	publisher := &Publisher{root: root, rootPath: rootPath, stateRoot: statePath, now: time.Now}
	if err := publisher.cleanupTemporaries(); err != nil {
		_ = root.Close()
		return nil, err
	}
	return publisher, nil
}

func (publisher *Publisher) Close() error {
	return publisher.root.Close()
}

func (publisher *Publisher) Publish(ctx context.Context, apply proposal.ApplyReceipt) (proposal.PublicationReceipt, error) {
	if err := publisher.validateReceipt(apply); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := privateDirectory(publisher.stateRoot); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	var receipt proposal.PublicationReceipt
	err := publisher.withStateLock(ctx, func() error {
		var err error
		receipt, err = publisher.publishLocked(ctx, apply)
		return err
	})
	return receipt, err
}

func (publisher *Publisher) publishLocked(ctx context.Context, apply proposal.ApplyReceipt) (proposal.PublicationReceipt, error) {
	gitDirectory, err := publisher.gitPath(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("resolve Git directory")
	}
	indexPath, err := publisher.gitPath(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("resolve Git index")
	}
	if !within(indexPath, gitDirectory) {
		return proposal.PublicationReceipt{}, proposal.ErrIntegrity
	}
	indexLockPath := indexPath + ".lock"
	var marker publicationMarker
	if existing, err := publisher.readMarker(apply.ProposalID); err == nil {
		if receipt, recovered, recoveryErr := publisher.recover(ctx, apply, existing, gitDirectory, indexPath); recovered || (recoveryErr != nil && !errors.Is(recoveryErr, errResumePublication)) {
			return receipt, recoveryErr
		}
		marker = existing
	} else if !errors.Is(err, errMarkerNotFound) {
		return proposal.PublicationReceipt{}, err
	}
	if marker.ProposalID == "" {
		currentHead, headErr := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
		if headErr == nil && currentHead != apply.Baseline.Head {
			return publisher.recognizeExisting(ctx, apply, currentHead)
		}
		authorName, nameErr := publisher.gitText(ctx, nil, "config", "--get", "user.name")
		authorEmail, emailErr := publisher.gitText(ctx, nil, "config", "--get", "user.email")
		if nameErr != nil || emailErr != nil || authorName == "" || authorEmail == "" || hasControl(authorName) || hasControl(authorEmail) {
			return proposal.PublicationReceipt{}, fmt.Errorf("Git publication identity is invalid")
		}
		marker, err = newPublicationMarker(apply, indexPath, authorName, authorEmail, publisher.now())
		if err != nil {
			return proposal.PublicationReceipt{}, fmt.Errorf("create publication marker")
		}
		if err := publisher.writeMarker(marker, true); err != nil {
			return proposal.PublicationReceipt{}, err
		}
	}
	if err := acquireIndexLock(marker, gitDirectory); errors.Is(err, ErrLocked) {
		_ = publisher.removeMarker(apply.ProposalID)
		return proposal.PublicationReceipt{}, ErrLocked
	} else if err != nil {
		if _, statErr := os.Lstat(indexLockPath); errors.Is(statErr, os.ErrNotExist) {
			_ = publisher.removeMarker(apply.ProposalID)
		}
		return proposal.PublicationReceipt{}, err
	}
	ownedLock := true
	recoverable := false
	defer func() {
		if ownedLock && !recoverable {
			removeErr := os.Remove(indexLockPath)
			if (removeErr == nil || errors.Is(removeErr, os.ErrNotExist)) && syncDirectory(gitDirectory) == nil {
				_ = publisher.removeMarker(apply.ProposalID)
			}
		}
	}()
	if marker.Phase == publicationPrepared {
		recoverable = true
		receipt, recovered, err := publisher.recover(ctx, apply, marker, gitDirectory, indexPath)
		if err != nil || recovered {
			if recovered {
				ownedLock = false
			}
			return receipt, err
		}
	}

	originalIndex, mode, err := readRegularFile(indexPath)
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("read Git index")
	}
	head, err := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != apply.Baseline.Head {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	branch, err := publisher.gitText(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || branch == "" {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	entryMode := "100644"
	entryBlob := ""
	if receiptBaselineExists(apply.Baseline) {
		entryMode, entryBlob, err = publisher.targetEntry(ctx, apply.Target.Path, nil)
		if err != nil || entryBlob != apply.Baseline.Blob {
			return proposal.PublicationReceipt{}, ErrDrift
		}
		treeMode, treeBlob, err := publisher.treeEntry(ctx, head, apply.Target.Path)
		if err != nil || treeBlob != apply.Baseline.Blob || treeMode != entryMode {
			return proposal.PublicationReceipt{}, ErrDrift
		}
	} else if err := publisher.ensureTargetAbsent(ctx, head, apply.Target.Path, nil); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if _, err := publisher.readPostState(apply); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := publisher.injectFault("before_capture"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	confirmedHead, err := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || confirmedHead != head {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	if receiptBaselineExists(apply.Baseline) {
		confirmedMode, confirmedBlob, err := publisher.targetEntry(ctx, apply.Target.Path, nil)
		if err != nil || confirmedMode != entryMode || confirmedBlob != entryBlob {
			return proposal.PublicationReceipt{}, ErrDrift
		}
	} else if err := publisher.ensureTargetAbsent(ctx, head, apply.Target.Path, nil); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	captured, err := publisher.readPostState(apply)
	if err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := publisher.injectFault("after_capture"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	blobOutput, err := publisher.gitInput(ctx, nil, captured, "hash-object", "-w", "--stdin")
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("write Git blob")
	}
	blob := strings.TrimSpace(string(blobOutput))

	commitIndex, err := temporaryFile(publisher.stateRoot, "commit-index")
	if err != nil {
		return proposal.PublicationReceipt{}, err
	}
	defer os.Remove(commitIndex)
	indexEnvironment := map[string]string{"GIT_INDEX_FILE": commitIndex}
	if _, err := publisher.gitInput(ctx, indexEnvironment, nil, "read-tree", head); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("initialize commit index")
	}
	cacheInfo := entryMode + "," + blob + "," + apply.Target.Path
	if _, err := publisher.gitInput(ctx, indexEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("update commit index")
	}
	tree, err := publisher.gitText(ctx, indexEnvironment, "write-tree")
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("write Git tree")
	}
	message := fmt.Sprintf("Publish memory proposal\n\nGit-Push-Memories-Proposal: %s/%d\n", apply.ProposalID, apply.Revision)
	commitEnvironment := map[string]string{
		"GIT_AUTHOR_NAME": marker.AuthorName, "GIT_AUTHOR_EMAIL": marker.AuthorEmail, "GIT_AUTHOR_DATE": marker.PublishedAt.Format(time.RFC3339),
		"GIT_COMMITTER_NAME": marker.AuthorName, "GIT_COMMITTER_EMAIL": marker.AuthorEmail, "GIT_COMMITTER_DATE": marker.PublishedAt.Format(time.RFC3339),
	}
	commitOutput, err := publisher.gitInput(ctx, commitEnvironment, []byte(message), "commit-tree", tree, "-p", head)
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("write Git commit")
	}
	commit := strings.TrimSpace(string(commitOutput))
	recoverable = true
	if err := publisher.injectFault("after_commit_object"); err != nil {
		return proposal.PublicationReceipt{}, err
	}

	reconciledIndex := marker.FinalTemporary
	defer os.Remove(reconciledIndex)
	reconciledHandle, err := os.OpenFile(reconciledIndex, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("create reconciled Git index")
	}
	if _, err := reconciledHandle.Write(originalIndex); err != nil {
		_ = reconciledHandle.Close()
		return proposal.PublicationReceipt{}, fmt.Errorf("copy Git index")
	}
	if err := reconciledHandle.Close(); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("close reconciled Git index")
	}
	reconcileEnvironment := map[string]string{"GIT_INDEX_FILE": reconciledIndex}
	if _, err := publisher.gitInput(ctx, reconcileEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("reconcile Git index")
	}
	finalIndex, _, err := readRegularFile(reconciledIndex)
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("read reconciled index")
	}
	marker.Phase = publicationPrepared
	marker.Branch = branch
	marker.Commit = commit
	marker.Blob = blob
	marker.FinalIndexHash = bytesHash(finalIndex)
	if err := publisher.writeMarker(marker, false); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := publisher.injectFault("after_marker_prepared"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := os.Chmod(reconciledIndex, mode.Perm()); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("set Git index mode")
	}
	finalHandle, err := os.OpenFile(reconciledIndex, os.O_RDWR, 0)
	if err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("open reconciled index")
	}
	if err := finalHandle.Sync(); err != nil {
		_ = finalHandle.Close()
		return proposal.PublicationReceipt{}, fmt.Errorf("sync reconciled index")
	}
	if err := finalHandle.Close(); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("close reconciled index")
	}
	if err := os.Rename(reconciledIndex, indexLockPath); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("prepare Git index installation")
	}
	recoverable = true
	if err := syncDirectory(gitDirectory); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("sync prepared Git index")
	}
	if err := publisher.injectFault("before_ref_update"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if _, err := publisher.gitInput(ctx, nil, nil, "update-ref", branch, commit, head); err != nil {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	if err := publisher.injectFault("after_ref_update"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	if err := os.Rename(indexLockPath, indexPath); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("install Git index")
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return proposal.PublicationReceipt{}, fmt.Errorf("sync Git directory")
	}
	ownedLock = false
	if err := publisher.injectFault("after_index_install"); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	receipt := proposal.PublicationReceipt{
		SchemaVersion: 1, ProposalID: apply.ProposalID, Revision: apply.Revision, Status: "published", PublishedAt: marker.PublishedAt,
		MemoryRoot: publisher.rootPath, Target: apply.Target, Branch: branch, Parent: head, Commit: commit, Blob: blob,
	}
	if err := publisher.removeMarker(apply.ProposalID); err != nil {
		return proposal.PublicationReceipt{}, err
	}
	return receipt, nil
}

func (publisher *Publisher) injectFault(stage string) error {
	if publisher.fault == nil {
		return nil
	}
	return publisher.fault(stage)
}

func (publisher *Publisher) validateReceipt(receipt proposal.ApplyReceipt) error {
	validVersion := receipt.SchemaVersion == 1 && receipt.Baseline.Exists == nil || receipt.SchemaVersion == 2 && receipt.Baseline.Exists != nil
	validBaseline := receiptBaselineExists(receipt.Baseline) && validHash(receipt.Baseline.FileHash) && validGitObject.MatchString(receipt.Baseline.Blob) && receipt.Baseline.FileHash != receipt.PostState.FileHash ||
		!receiptBaselineExists(receipt.Baseline) && receipt.SchemaVersion == 2 && receipt.Baseline.FileHash == "" && receipt.Baseline.Blob == ""
	validCreatedTarget := receiptBaselineExists(receipt.Baseline) || memory.IsCreatePagePath(receipt.Target.Path) && receipt.Target.SectionID == memory.PageID(receipt.Target.Path)+"-overview"
	if !validVersion || receipt.Status != "applied_unpublished" || receipt.Revision != 1 || receipt.AppliedAt.IsZero() || receipt.MemoryRoot != publisher.rootPath ||
		!validProposalID.MatchString(receipt.ProposalID) || receipt.Target.Path == "" || receipt.Target.SectionID == "" || !memory.IsCanonicalPagePath(receipt.Target.Path) || hasControl(receipt.Target.Path) ||
		!validBaseline || !validCreatedTarget || !validHash(receipt.PostState.FileHash) || !validGitObject.MatchString(receipt.Baseline.Head) {
		return proposal.ErrIntegrity
	}
	return nil
}

func receiptBaselineExists(baseline proposal.Baseline) bool {
	return baseline.Exists == nil || *baseline.Exists
}

func (publisher *Publisher) ensureTargetAbsent(ctx context.Context, head, target string, environment map[string]string) error {
	index, err := publisher.gitInput(ctx, environment, nil, "ls-files", "--stage", "-z", "--", target)
	if err != nil || len(index) != 0 {
		return ErrDrift
	}
	tree, err := publisher.gitInput(ctx, environment, nil, "ls-tree", "-z", head, "--", target)
	if err != nil || len(tree) != 0 {
		return ErrDrift
	}
	return nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func (publisher *Publisher) targetEntry(ctx context.Context, target string, environment map[string]string) (string, string, error) {
	output, err := publisher.gitInput(ctx, environment, nil, "ls-files", "--stage", "-z", "--", target)
	if err != nil || bytes.Count(output, []byte{0}) != 1 {
		return "", "", ErrDrift
	}
	entry := strings.TrimSuffix(string(output), "\x00")
	tab := strings.IndexByte(entry, '\t')
	fields := strings.Fields(entry[:max(tab, 0)])
	if tab < 0 || len(fields) != 3 || fields[2] != "0" || entry[tab+1:] != target {
		return "", "", ErrDrift
	}
	return fields[0], fields[1], nil
}

func (publisher *Publisher) treeEntry(ctx context.Context, head, target string) (string, string, error) {
	output, err := publisher.gitInput(ctx, nil, nil, "ls-tree", "-z", head, "--", target)
	if err != nil || bytes.Count(output, []byte{0}) != 1 {
		return "", "", ErrDrift
	}
	entry := strings.TrimSuffix(string(output), "\x00")
	tab := strings.IndexByte(entry, '\t')
	fields := strings.Fields(entry[:max(tab, 0)])
	if tab < 0 || len(fields) != 3 || fields[1] != "blob" || entry[tab+1:] != target {
		return "", "", ErrDrift
	}
	return fields[0], fields[2], nil
}

func (publisher *Publisher) readPostState(receipt proposal.ApplyReceipt) ([]byte, error) {
	fd, err := unix.Openat2(int(publisher.root.Fd()), receipt.Target.Path, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, ErrDrift
	}
	file := os.NewFile(uintptr(fd), receipt.Target.Path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, ErrDrift
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil || hash(contents) != receipt.PostState.FileHash {
		return nil, ErrDrift
	}
	return contents, nil
}

func (publisher *Publisher) gitPath(ctx context.Context, args ...string) (string, error) {
	value, err := publisher.gitText(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(publisher.rootPath, value)
	}
	return filepath.Clean(value), nil
}

func (publisher *Publisher) gitText(ctx context.Context, environment map[string]string, args ...string) (string, error) {
	output, err := publisher.gitInput(ctx, environment, nil, args...)
	return strings.TrimSpace(string(output)), err
}

func (publisher *Publisher) gitInput(ctx context.Context, environment map[string]string, input []byte, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--no-optional-locks", "-C", "/proc/self/fd/3"}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Env = gitEnvironment(environment)
	command.ExtraFiles = []*os.File{publisher.root}
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

func privateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create publisher state")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("secure publisher state")
		}
		return syncDirectory(filepath.Dir(directory))
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("invalid publisher state")
	}
	return nil
}

func (publisher *Publisher) withStateLock(ctx context.Context, action func() error) error {
	handle, err := os.Open(publisher.stateRoot)
	if err != nil {
		return fmt.Errorf("open publisher state lock")
	}
	defer handle.Close()
	for {
		err := unix.Flock(int(handle.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock publisher state")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer unix.Flock(int(handle.Fd()), unix.LOCK_UN)
	return action()
}

var controlledPublisherTemporary = regexp.MustCompile(`^\.git-push-memories-(commit-index|real-index|marker)-[0-9]+\.tmp$`)

func (publisher *Publisher) cleanupTemporaries() error {
	if _, err := os.Lstat(publisher.stateRoot); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := privateDirectory(publisher.stateRoot); err != nil {
		return err
	}
	return publisher.withStateLock(context.Background(), func() error {
		return filepath.WalkDir(publisher.stateRoot, func(filePath string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() && controlledPublisherTemporary.MatchString(entry.Name()) {
				return os.Remove(filePath)
			}
			return nil
		})
	})
}

func temporaryFile(directory, kind string) (string, error) {
	file, err := os.CreateTemp(directory, ".git-push-memories-"+kind+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create publisher temporary")
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func readRegularFile(name string) ([]byte, os.FileMode, error) {
	fd, err := unix.Open(name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
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

func hash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validHash(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func canonicalFuturePath(requested string) (string, error) {
	if requested == "" {
		return "", errors.New("state root is required")
	}
	absolute, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	existing := absolute
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", errors.New("state root is invalid")
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
	canonical, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for index := len(suffix) - 1; index >= 0; index-- {
		canonical = filepath.Join(canonical, suffix[index])
	}
	return canonical, nil
}

func within(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

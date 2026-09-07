package publisher

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wuerike/git-push-memories/internal/proposal"
	"golang.org/x/sys/unix"
)

var errMarkerNotFound = errors.New("publication marker not found")
var errResumePublication = errors.New("resume publication intent")

type publicationPhase string

const (
	publicationIntent   publicationPhase = "intent"
	publicationPrepared publicationPhase = "prepared"
)

type publicationMarker struct {
	SchemaVersion  int              `json:"schema_version"`
	ProposalID     string           `json:"proposal_id"`
	Revision       int              `json:"revision"`
	Phase          publicationPhase `json:"phase"`
	Token          string           `json:"token"`
	MemoryRoot     string           `json:"memory_root"`
	Target         proposal.Target  `json:"target"`
	PostStateHash  string           `json:"post_state_hash"`
	BaselineExists *bool            `json:"baseline_exists,omitempty"`
	BaselineHead   string           `json:"baseline_head"`
	BaselineBlob   string           `json:"baseline_blob"`
	IndexPath      string           `json:"index_path"`
	LockTemporary  string           `json:"lock_temporary"`
	FinalTemporary string           `json:"final_temporary"`
	Branch         string           `json:"branch,omitempty"`
	Commit         string           `json:"commit,omitempty"`
	Blob           string           `json:"blob,omitempty"`
	FinalIndexHash string           `json:"final_index_hash,omitempty"`
	PublishedAt    time.Time        `json:"published_at,omitempty"`
	AuthorName     string           `json:"author_name"`
	AuthorEmail    string           `json:"author_email"`
}

func newPublicationMarker(receipt proposal.ApplyReceipt, indexPath, authorName, authorEmail string, publishedAt time.Time) (publicationMarker, error) {
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return publicationMarker{}, err
	}
	token := hex.EncodeToString(tokenBytes[:])
	return publicationMarker{
		SchemaVersion: 1, ProposalID: receipt.ProposalID, Revision: receipt.Revision, Phase: publicationIntent,
		Token: token, MemoryRoot: receipt.MemoryRoot, Target: receipt.Target,
		PostStateHash: receipt.PostState.FileHash, BaselineExists: receipt.Baseline.Exists, BaselineHead: receipt.Baseline.Head, BaselineBlob: receipt.Baseline.Blob,
		IndexPath:      indexPath,
		LockTemporary:  filepath.Join(filepath.Dir(indexPath), ".git-push-memories-index-lock-"+token+".tmp"),
		FinalTemporary: filepath.Join(filepath.Dir(indexPath), ".git-push-memories-index-final-"+token+".tmp"),
		PublishedAt:    publishedAt.UTC().Truncate(time.Second), AuthorName: authorName, AuthorEmail: authorEmail,
	}, nil
}

func (publisher *Publisher) markerPath(id string) string {
	return filepath.Join(publisher.stateRoot, "publishing", id+".json")
}

func (publisher *Publisher) writeMarker(marker publicationMarker, create bool) error {
	directory := filepath.Dir(publisher.markerPath(marker.ProposalID))
	if err := privateDirectory(directory); err != nil {
		return err
	}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".git-push-memories-marker-*.tmp")
	if err != nil {
		return fmt.Errorf("create publication marker")
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("write publication marker")
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync publication marker")
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := publisher.markerPath(marker.ProposalID)
	if create {
		err = unix.Renameat2(unix.AT_FDCWD, temporaryName, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	} else {
		err = os.Rename(temporaryName, destination)
	}
	if err != nil {
		return fmt.Errorf("publish publication marker")
	}
	committed = true
	return syncDirectory(directory)
}

func (publisher *Publisher) readMarker(id string) (publicationMarker, error) {
	var marker publicationMarker
	path := publisher.markerPath(id)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return marker, errMarkerNotFound
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return marker, proposal.ErrIntegrity
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return marker, proposal.ErrIntegrity
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return marker, proposal.ErrIntegrity
	}
	if _, err := decoder.Token(); err != io.EOF {
		return marker, proposal.ErrIntegrity
	}
	_, tokenErr := hex.DecodeString(marker.Token)
	validBaseline := markerBaselineExists(marker) && validGitObject.MatchString(marker.BaselineBlob) || !markerBaselineExists(marker) && marker.BaselineBlob == ""
	if marker.SchemaVersion != 1 || marker.ProposalID != id || marker.Revision != 1 || len(marker.Token) != 32 || tokenErr != nil || marker.MemoryRoot == "" || marker.Target.Path == "" ||
		!validHash(marker.PostStateHash) || !validGitObject.MatchString(marker.BaselineHead) || !validBaseline || !filepath.IsAbs(marker.IndexPath) ||
		marker.LockTemporary != filepath.Join(filepath.Dir(marker.IndexPath), ".git-push-memories-index-lock-"+marker.Token+".tmp") ||
		marker.FinalTemporary != filepath.Join(filepath.Dir(marker.IndexPath), ".git-push-memories-index-final-"+marker.Token+".tmp") || marker.PublishedAt.IsZero() ||
		marker.AuthorName == "" || marker.AuthorEmail == "" || hasControl(marker.AuthorName) || hasControl(marker.AuthorEmail) || (marker.Phase != publicationIntent && marker.Phase != publicationPrepared) {
		return marker, proposal.ErrIntegrity
	}
	if marker.Phase == publicationPrepared && (marker.Branch == "" || hasControl(marker.Branch) || !validGitObject.MatchString(marker.Commit) || !validGitObject.MatchString(marker.Blob) || !validHash(marker.FinalIndexHash)) {
		return marker, proposal.ErrIntegrity
	}
	return marker, nil
}

func (publisher *Publisher) removeMarker(id string) error {
	path := publisher.markerPath(id)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove publication marker")
	}
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return syncDirectory(filepath.Dir(path))
}

func markerToken(marker publicationMarker) []byte {
	return []byte("git-push-memories:" + marker.Token + "\n")
}

func acquireIndexLock(marker publicationMarker, gitDirectory string) error {
	temporary, err := os.OpenFile(marker.LockTemporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create Git index lock temporary")
	}
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(marker.LockTemporary)
		}
	}()
	if _, err := temporary.Write(markerToken(marker)); err != nil {
		return fmt.Errorf("initialize Git index lock")
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync Git index lock")
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Git index lock")
	}
	err = unix.Renameat2(unix.AT_FDCWD, marker.LockTemporary, unix.AT_FDCWD, marker.IndexPath+".lock", unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		if removeErr := os.Remove(marker.LockTemporary); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove Git index lock temporary")
		}
		committed = true
		if err := syncDirectory(gitDirectory); err != nil {
			return err
		}
		return ErrLocked
	}
	if err != nil {
		return fmt.Errorf("acquire Git index lock")
	}
	committed = true
	return syncDirectory(gitDirectory)
}

func bytesHash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (publisher *Publisher) recover(ctx context.Context, apply proposal.ApplyReceipt, marker publicationMarker, gitDirectory, indexPath string) (proposal.PublicationReceipt, bool, error) {
	if marker.ProposalID != apply.ProposalID || marker.Revision != apply.Revision || marker.MemoryRoot != publisher.rootPath || marker.Target != apply.Target || markerBaselineExists(marker) != receiptBaselineExists(apply.Baseline) ||
		marker.PostStateHash != apply.PostState.FileHash || marker.BaselineHead != apply.Baseline.Head || marker.BaselineBlob != apply.Baseline.Blob || marker.IndexPath != indexPath {
		return proposal.PublicationReceipt{}, false, proposal.ErrIntegrity
	}
	if marker.Phase == publicationPrepared {
		branch, err := publisher.gitText(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
		if err != nil || branch != marker.Branch {
			return proposal.PublicationReceipt{}, false, ErrDrift
		}
	}
	lockPath := indexPath + ".lock"
	lockContents, _, lockErr := readRegularFile(lockPath)
	if lockErr == nil {
		switch {
		case bytes.Equal(lockContents, markerToken(marker)):
			head, err := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
			if err != nil || head != marker.BaselineHead {
				return proposal.PublicationReceipt{}, false, ErrLocked
			}
			if marker.Phase == publicationPrepared {
				return publisher.resumePrepared(ctx, marker, gitDirectory, indexPath)
			}
			if err := os.Remove(lockPath); err != nil {
				return proposal.PublicationReceipt{}, false, ErrLocked
			}
			if err := removeOwnedTemporary(marker.FinalTemporary); err != nil {
				return proposal.PublicationReceipt{}, false, err
			}
			if err := syncDirectory(gitDirectory); err != nil {
				return proposal.PublicationReceipt{}, false, err
			}
			return proposal.PublicationReceipt{}, false, errResumePublication
		case marker.Phase == publicationPrepared && bytesHash(lockContents) == marker.FinalIndexHash:
			if err := publisher.validatePreparedCommit(ctx, marker); err != nil {
				return proposal.PublicationReceipt{}, false, err
			}
			head, err := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
			if err != nil {
				return proposal.PublicationReceipt{}, false, ErrDrift
			}
			if head == marker.BaselineHead {
				if _, err := publisher.gitInput(ctx, nil, nil, "update-ref", marker.Branch, marker.Commit, marker.BaselineHead); err != nil {
					return proposal.PublicationReceipt{}, false, ErrDrift
				}
			} else if head != marker.Commit {
				if err := os.Remove(lockPath); err != nil {
					return proposal.PublicationReceipt{}, false, ErrLocked
				}
				if err := syncDirectory(gitDirectory); err != nil {
					return proposal.PublicationReceipt{}, false, err
				}
				if err := publisher.removeMarker(marker.ProposalID); err != nil {
					return proposal.PublicationReceipt{}, false, err
				}
				return proposal.PublicationReceipt{}, false, ErrDrift
			}
			if err := os.Rename(lockPath, indexPath); err != nil {
				return proposal.PublicationReceipt{}, false, fmt.Errorf("recover Git index")
			}
			if err := syncDirectory(gitDirectory); err != nil {
				return proposal.PublicationReceipt{}, false, fmt.Errorf("sync recovered Git index")
			}
			receipt := marker.receipt()
			if err := publisher.removeMarker(marker.ProposalID); err != nil {
				return proposal.PublicationReceipt{}, false, err
			}
			return receipt, true, nil
		default:
			return proposal.PublicationReceipt{}, false, ErrLocked
		}
	}
	if !errors.Is(lockErr, os.ErrNotExist) {
		return proposal.PublicationReceipt{}, false, ErrLocked
	}
	if temporaryContents, _, err := readRegularFile(marker.LockTemporary); err == nil {
		if !bytes.Equal(temporaryContents, markerToken(marker)) {
			return proposal.PublicationReceipt{}, false, ErrLocked
		}
		if err := os.Remove(marker.LockTemporary); err != nil {
			return proposal.PublicationReceipt{}, false, ErrLocked
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return proposal.PublicationReceipt{}, false, ErrLocked
	}
	if err := removeOwnedTemporary(marker.FinalTemporary); err != nil {
		return proposal.PublicationReceipt{}, false, ErrLocked
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	head, err := publisher.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return proposal.PublicationReceipt{}, false, ErrDrift
	}
	if marker.Phase == publicationPrepared && head == marker.Commit {
		if err := publisher.validatePreparedCommit(ctx, marker); err != nil {
			return proposal.PublicationReceipt{}, false, err
		}
		_, blob, err := publisher.targetEntry(ctx, marker.Target.Path, nil)
		if err != nil || blob != marker.Blob {
			return proposal.PublicationReceipt{}, false, ErrDrift
		}
		receipt := marker.receipt()
		if err := publisher.removeMarker(marker.ProposalID); err != nil {
			return proposal.PublicationReceipt{}, false, err
		}
		return receipt, true, nil
	}
	if head != marker.BaselineHead {
		return proposal.PublicationReceipt{}, false, ErrDrift
	}
	return proposal.PublicationReceipt{}, false, errResumePublication
}

func (publisher *Publisher) resumePrepared(ctx context.Context, marker publicationMarker, gitDirectory, indexPath string) (proposal.PublicationReceipt, bool, error) {
	if err := publisher.validatePreparedCommit(ctx, marker); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	originalIndex, mode, err := readRegularFile(indexPath)
	if err != nil {
		return proposal.PublicationReceipt{}, false, ErrDrift
	}
	entryMode := "100644"
	if markerBaselineExists(marker) {
		var entryBlob string
		entryMode, entryBlob, err = publisher.targetEntry(ctx, marker.Target.Path, nil)
		if err != nil || entryBlob != marker.BaselineBlob {
			return proposal.PublicationReceipt{}, false, ErrDrift
		}
	} else if err := publisher.ensureTargetAbsent(ctx, marker.BaselineHead, marker.Target.Path, nil); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	if err := removeOwnedTemporary(marker.FinalTemporary); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	file, err := os.OpenFile(marker.FinalTemporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return proposal.PublicationReceipt{}, false, fmt.Errorf("recreate reconciled index")
	}
	if _, err := file.Write(originalIndex); err != nil {
		_ = file.Close()
		return proposal.PublicationReceipt{}, false, fmt.Errorf("recreate reconciled index")
	}
	if err := file.Close(); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	environment := map[string]string{"GIT_INDEX_FILE": marker.FinalTemporary}
	cacheInfo := entryMode + "," + marker.Blob + "," + marker.Target.Path
	if _, err := publisher.gitInput(ctx, environment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
		return proposal.PublicationReceipt{}, false, fmt.Errorf("reconcile interrupted Git index")
	}
	finalIndex, _, err := readRegularFile(marker.FinalTemporary)
	if err != nil || bytesHash(finalIndex) != marker.FinalIndexHash {
		return proposal.PublicationReceipt{}, false, proposal.ErrIntegrity
	}
	if err := os.Chmod(marker.FinalTemporary, mode.Perm()); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	handle, err := os.OpenFile(marker.FinalTemporary, os.O_RDWR, 0)
	if err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	if err := handle.Sync(); err != nil {
		_ = handle.Close()
		return proposal.PublicationReceipt{}, false, err
	}
	if err := handle.Close(); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	if err := os.Rename(marker.FinalTemporary, indexPath+".lock"); err != nil {
		return proposal.PublicationReceipt{}, false, fmt.Errorf("restore prepared Git index")
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return proposal.PublicationReceipt{}, false, err
	}
	return publisher.recover(ctx, proposal.ApplyReceipt{
		SchemaVersion: 2, ProposalID: marker.ProposalID, Revision: marker.Revision, Status: "applied_unpublished", AppliedAt: marker.PublishedAt,
		MemoryRoot: marker.MemoryRoot, Target: marker.Target, Baseline: proposal.Baseline{Exists: marker.BaselineExists, Head: marker.BaselineHead, Blob: marker.BaselineBlob}, PostState: proposal.PostState{FileHash: marker.PostStateHash},
	}, marker, gitDirectory, indexPath)
}

func removeOwnedTemporary(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrLocked
	}
	return os.Remove(path)
}

func (publisher *Publisher) validatePreparedCommit(ctx context.Context, marker publicationMarker) error {
	parent, err := publisher.gitText(ctx, nil, "rev-parse", marker.Commit+"^")
	if err != nil || parent != marker.BaselineHead {
		return proposal.ErrIntegrity
	}
	blob, err := publisher.gitText(ctx, nil, "rev-parse", marker.Commit+":"+marker.Target.Path)
	if err != nil || blob != marker.Blob {
		return proposal.ErrIntegrity
	}
	message, err := publisher.gitText(ctx, nil, "show", "-s", "--format=%B", marker.Commit)
	wantTrailer := fmt.Sprintf("Git-Push-Memories-Proposal: %s/%d", marker.ProposalID, marker.Revision)
	if err != nil || !strings.Contains(message, wantTrailer) {
		return proposal.ErrIntegrity
	}
	changed, err := publisher.gitText(ctx, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", marker.Commit)
	if err != nil || changed != marker.Target.Path {
		return proposal.ErrIntegrity
	}
	return nil
}

func (marker publicationMarker) receipt() proposal.PublicationReceipt {
	return proposal.PublicationReceipt{
		SchemaVersion: 1, ProposalID: marker.ProposalID, Revision: marker.Revision, Status: "published", PublishedAt: marker.PublishedAt,
		MemoryRoot: marker.MemoryRoot, Target: marker.Target, Branch: marker.Branch, Parent: marker.BaselineHead, Commit: marker.Commit, Blob: marker.Blob,
	}
}

func (publisher *Publisher) recognizeExisting(ctx context.Context, apply proposal.ApplyReceipt, commit string) (proposal.PublicationReceipt, error) {
	branch, err := publisher.gitText(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || branch == "" {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	blob, err := publisher.gitText(ctx, nil, "rev-parse", commit+":"+apply.Target.Path)
	if err != nil {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	publishedText, err := publisher.gitText(ctx, nil, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	publishedAt, err := time.Parse(time.RFC3339, publishedText)
	if err != nil {
		return proposal.PublicationReceipt{}, proposal.ErrIntegrity
	}
	marker := publicationMarker{
		SchemaVersion: 1, ProposalID: apply.ProposalID, Revision: apply.Revision, Phase: publicationPrepared, MemoryRoot: apply.MemoryRoot,
		Target: apply.Target, PostStateHash: apply.PostState.FileHash, BaselineExists: apply.Baseline.Exists, BaselineHead: apply.Baseline.Head, BaselineBlob: apply.Baseline.Blob,
		Branch: branch, Commit: commit, Blob: blob, PublishedAt: publishedAt,
	}
	if err := publisher.validatePreparedCommit(ctx, marker); err != nil {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	contents, err := publisher.gitInput(ctx, nil, nil, "cat-file", "blob", blob)
	if err != nil || hash(contents) != apply.PostState.FileHash {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	indexMode, indexBlob, err := publisher.targetEntry(ctx, apply.Target.Path, nil)
	treeMode, treeBlob, treeErr := publisher.treeEntry(ctx, commit, apply.Target.Path)
	if err != nil || treeErr != nil || indexBlob != blob || treeBlob != blob || indexMode != treeMode {
		return proposal.PublicationReceipt{}, ErrDrift
	}
	return marker.receipt(), nil
}

func markerBaselineExists(marker publicationMarker) bool {
	return marker.BaselineExists == nil || *marker.BaselineExists
}

package curation

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
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"golang.org/x/sys/unix"
)

const transactionSchemaVersion = 1

var transactionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var transactionIndexPattern = regexp.MustCompile(`^\.git-push-memories-curation-index-[0-9a-f]{32}$`)
var transactionMarkerTemporaryPattern = regexp.MustCompile(`^\.curation-marker-[0-9]+\.tmp$`)

type transactionMarker struct {
	SchemaVersion     int               `json:"schema_version"`
	ID                string            `json:"id"`
	MemoryRoot        string            `json:"memory_root"`
	Branch            string            `json:"branch"`
	Parent            string            `json:"parent"`
	Commit            string            `json:"commit"`
	Tree              string            `json:"tree"`
	IndexPath         string            `json:"index_path"`
	IndexArtifact     string            `json:"index_artifact"`
	OriginalIndexHash string            `json:"original_index_hash"`
	FinalIndexHash    string            `json:"final_index_hash"`
	PreviewHash       string            `json:"preview_hash"`
	EvidenceHash      string            `json:"evidence_hash"`
	Items             []transactionItem `json:"items"`
}

type transactionItem struct {
	ProposalID       string        `json:"proposal_id"`
	ProposalPath     string        `json:"proposal_path"`
	ProposalBlob     string        `json:"proposal_blob"`
	ProposalHash     string        `json:"proposal_hash"`
	Operation        Operation     `json:"operation"`
	TargetPath       string        `json:"target_path,omitempty"`
	SectionID        string        `json:"section_id,omitempty"`
	Decision         string        `json:"decision"`
	Reason           DiscardReason `json:"reason,omitempty"`
	BaselineExists   bool          `json:"baseline_exists"`
	BaselineBlob     string        `json:"baseline_blob,omitempty"`
	BaselineHash     string        `json:"baseline_hash,omitempty"`
	BaselineMode     uint32        `json:"baseline_mode,omitempty"`
	PostStateExists  bool          `json:"post_state_exists"`
	PostStateBlob    string        `json:"post_state_blob,omitempty"`
	PostStateHash    string        `json:"post_state_hash,omitempty"`
	PostStateGitMode string        `json:"post_state_git_mode,omitempty"`
}

func Recover(ctx context.Context, cfg config.Config) (BatchResult, error) {
	if _, err := os.Lstat(cfg.StateRoot); errors.Is(err, os.ErrNotExist) {
		return BatchResult{}, nil
	} else if err != nil {
		return BatchResult{}, ErrRecovery
	}
	curator, err := New(cfg)
	if err != nil {
		return BatchResult{}, err
	}
	defer curator.Close()
	return curator.Recover(ctx)
}

func (curator *Curator) Recover(ctx context.Context) (BatchResult, error) {
	if _, err := os.Lstat(curator.cfg.StateRoot); errors.Is(err, os.ErrNotExist) {
		return BatchResult{}, nil
	} else if err != nil {
		return BatchResult{}, ErrRecovery
	}
	var result BatchResult
	err := repolock.With(ctx, curator.cfg.StateRoot, func() error {
		var err error
		result, _, err = curator.recoverLocked(ctx)
		return err
	})
	return result, err
}

func (curator *Curator) recoverLocked(ctx context.Context) (BatchResult, transactionMarker, error) {
	marker, err := curator.readTransactionMarker()
	if errors.Is(err, os.ErrNotExist) {
		if err := curator.cleanupTransactionArtifacts(ctx); err != nil {
			return BatchResult{}, transactionMarker{}, err
		}
		return BatchResult{}, transactionMarker{}, nil
	}
	if err != nil {
		return BatchResult{}, transactionMarker{}, err
	}
	gitDirectory, err := curator.gitPath(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return BatchResult{}, marker, ErrRecovery
	}
	indexPath, err := curator.gitPath(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index")
	expectedArtifact := filepath.Join(gitDirectory, ".git-push-memories-curation-index-"+marker.ID)
	if err != nil || marker.MemoryRoot != curator.rootPath || marker.IndexPath != indexPath || marker.IndexArtifact != expectedArtifact {
		return BatchResult{}, marker, ErrRecovery
	}
	if err := curator.verifyTransactionCommit(ctx, marker); err != nil {
		return BatchResult{}, marker, err
	}
	head, err := curator.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return BatchResult{}, marker, ErrRecovery
	}
	branch, err := curator.gitText(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || branch != marker.Branch {
		return BatchResult{}, marker, ErrDrift
	}
	artifactInfo, err := regularFileInfo(marker.IndexArtifact)
	if err != nil {
		return BatchResult{}, marker, ErrRecovery
	}
	artifactBytes, _, err := readRegularFile(marker.IndexArtifact)
	if err != nil || contentHash(artifactBytes) != marker.FinalIndexHash {
		return BatchResult{}, marker, ErrRecovery
	}
	lockPath := marker.IndexPath + ".lock"
	lockInfo, lockErr := regularFileInfo(lockPath)
	lockExists := lockErr == nil
	if lockErr != nil && !errors.Is(lockErr, os.ErrNotExist) {
		return BatchResult{}, marker, ErrLocked
	}
	if lockExists && !os.SameFile(artifactInfo, lockInfo) {
		return BatchResult{}, marker, ErrLocked
	}

	switch head {
	case marker.Parent:
		if err := curator.rollbackTransaction(ctx, marker); err != nil {
			return BatchResult{}, marker, errors.Join(ErrRecovery, err)
		}
		if lockExists {
			if err := removeOwnedIndexLock(marker, lockPath); err != nil {
				return BatchResult{}, marker, ErrLocked
			}
			if err := syncDirectory(gitDirectory); err != nil {
				return BatchResult{}, marker, ErrRecovery
			}
		}
		if err := curator.removeTransaction(marker); err != nil {
			return BatchResult{}, marker, err
		}
		return BatchResult{}, marker, nil
	case marker.Commit:
		if err := curator.confirmPostState(marker); err != nil {
			return BatchResult{}, marker, err
		}
		if !lockExists {
			indexInfo, infoErr := regularFileInfo(marker.IndexPath)
			if infoErr != nil {
				return BatchResult{}, marker, ErrRecovery
			}
			if !os.SameFile(artifactInfo, indexInfo) {
				indexBytes, _, readErr := readRegularFile(marker.IndexPath)
				if readErr != nil || contentHash(indexBytes) != marker.OriginalIndexHash {
					return BatchResult{}, marker, ErrRecovery
				}
				if err := os.Link(marker.IndexArtifact, lockPath); errors.Is(err, os.ErrExist) {
					return BatchResult{}, marker, ErrLocked
				} else if err != nil {
					return BatchResult{}, marker, ErrRecovery
				}
				lockExists = true
			}
		}
		if lockExists {
			if err := installOwnedIndex(marker, lockPath); err != nil {
				return BatchResult{}, marker, ErrRecovery
			}
			if err := syncDirectory(gitDirectory); err != nil {
				return BatchResult{}, marker, ErrRecovery
			}
		}
		for _, item := range marker.Items {
			if item.Decision == "pending" {
				continue
			}
			if err := curator.removeRecoveredProposal(item); err != nil {
				return BatchResult{}, marker, err
			}
		}
		result := marker.result()
		if err := curator.removeTransaction(marker); err != nil {
			return BatchResult{}, marker, err
		}
		return result, marker, nil
	default:
		return BatchResult{}, marker, ErrDrift
	}
}

func (curator *Curator) recognizeCommittedBatch(ctx context.Context, batch BatchPreview) (BatchResult, bool, error) {
	if len(batch.Items) == 0 || len(batch.Items) > MaxBatchItems {
		return BatchResult{}, false, nil
	}
	commit, err := curator.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || commit == batch.Items[0].head {
		return BatchResult{}, false, nil
	}
	parent, err := curator.gitText(ctx, nil, "rev-parse", commit+"^")
	if err != nil {
		return BatchResult{}, false, nil
	}
	marker := transactionMarker{Parent: parent, Commit: commit, Items: make([]transactionItem, len(batch.Items))}
	marker.Tree, err = curator.gitText(ctx, nil, "rev-parse", commit+"^{tree}")
	if err != nil {
		return BatchResult{}, false, nil
	}
	for index, preview := range batch.Items {
		if preview.head != parent {
			return BatchResult{}, false, nil
		}
		item := transactionItemFromPreview(preview)
		if item.PostStateExists {
			item.PostStateBlob, err = curator.gitText(ctx, nil, "rev-parse", commit+":"+preview.TargetPath)
			if err != nil {
				return BatchResult{}, false, nil
			}
		}
		marker.Items[index] = item
	}
	if err := curator.verifyTransactionCommit(ctx, marker); err != nil {
		return BatchResult{}, false, nil
	}
	if err := curator.confirmPostState(marker); err != nil {
		return BatchResult{}, false, err
	}
	for _, item := range marker.Items {
		proposalEntry, err := curator.gitInput(ctx, nil, nil, "ls-files", "--stage", "-z", "--", item.ProposalPath)
		if err != nil {
			return BatchResult{}, false, ErrDrift
		}
		if item.Decision == "pending" {
			contents, _, err := curator.readControlledFile(item.ProposalPath)
			mode, blob, entryErr := parseIndexEntry(proposalEntry, item.ProposalPath)
			if err != nil || contentHash(contents) != item.ProposalHash || entryErr != nil || mode != "100644" || blob != item.ProposalBlob {
				return BatchResult{}, false, ErrDrift
			}
		} else {
			if _, _, err := curator.readControlledFile(item.ProposalPath); !errors.Is(err, os.ErrNotExist) || len(proposalEntry) != 0 {
				return BatchResult{}, false, ErrDrift
			}
		}
		if item.TargetPath == "" {
			continue
		}
		entry, err := curator.gitInput(ctx, nil, nil, "ls-files", "--stage", "-z", "--", item.TargetPath)
		if err != nil {
			return BatchResult{}, false, ErrDrift
		}
		if !item.PostStateExists {
			if len(entry) != 0 {
				return BatchResult{}, false, ErrDrift
			}
			continue
		}
		mode, blob, err := parseIndexEntry(entry, item.TargetPath)
		if err != nil || mode != item.PostStateGitMode || blob != item.PostStateBlob {
			return BatchResult{}, false, ErrDrift
		}
	}
	return marker.result(), true, nil
}

func (curator *Curator) commitBatch(ctx context.Context, batch BatchPreview, validateEvidence EvidenceValidator) (BatchResult, error) {
	gitDirectory, err := curator.gitPath(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return BatchResult{}, fmt.Errorf("resolve Git directory")
	}
	indexPath, err := curator.gitPath(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil || !within(indexPath, gitDirectory) {
		return BatchResult{}, fmt.Errorf("resolve Git index")
	}
	branch, err := curator.gitText(ctx, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || branch != "refs/heads/main" {
		return BatchResult{}, ErrDrift
	}
	head, err := curator.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != batch.Items[0].head {
		return BatchResult{}, ErrDrift
	}
	originalIndex, indexMode, err := readRegularFile(indexPath)
	if err != nil {
		return BatchResult{}, fmt.Errorf("read Git index")
	}

	id, err := randomTransactionID()
	if err != nil {
		return BatchResult{}, fmt.Errorf("create transaction identifier")
	}
	marker := transactionMarker{
		SchemaVersion: transactionSchemaVersion, ID: id, MemoryRoot: curator.rootPath, Branch: branch, Parent: head,
		IndexPath: indexPath, IndexArtifact: filepath.Join(gitDirectory, ".git-push-memories-curation-index-"+id),
		OriginalIndexHash: contentHash(originalIndex), PreviewHash: batchPreviewHash(batch.Items), Items: make([]transactionItem, len(batch.Items)),
	}
	marker.EvidenceHash, err = evidenceHash(validateEvidence)
	if err != nil {
		return BatchResult{}, err
	}

	commitIndex, err := temporaryIndex(curator.cfg.StateRoot)
	if err != nil {
		return BatchResult{}, err
	}
	defer os.Remove(commitIndex)
	commitEnvironment := map[string]string{"GIT_INDEX_FILE": commitIndex}
	if _, err := curator.gitInput(ctx, commitEnvironment, nil, "read-tree", head); err != nil {
		return BatchResult{}, fmt.Errorf("initialize curation tree")
	}
	for index, preview := range batch.Items {
		item := transactionItemFromPreview(preview)
		if item.PostStateExists {
			item.PostStateBlob, err = curator.gitText(ctx, preview.postState, "hash-object", "-w", "--stdin")
			if err != nil || item.PostStateBlob == "" {
				return BatchResult{}, fmt.Errorf("write memory blob")
			}
			cacheInfo := preview.gitMode + "," + item.PostStateBlob + "," + preview.TargetPath
			if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
				return BatchResult{}, fmt.Errorf("add memory to curation tree")
			}
		} else if item.TargetPath != "" {
			if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--force-remove", "--", preview.TargetPath); err != nil {
				return BatchResult{}, fmt.Errorf("remove memory from curation tree")
			}
		}
		if preview.Operation != OperationKeepPending {
			if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--force-remove", "--", preview.ProposalPath); err != nil {
				return BatchResult{}, fmt.Errorf("remove proposal from curation tree")
			}
		}
		marker.Items[index] = item
	}
	treeOutput, err := curator.gitInput(ctx, commitEnvironment, nil, "write-tree")
	if err != nil {
		return BatchResult{}, fmt.Errorf("write curation tree")
	}
	tree := strings.TrimSpace(string(treeOutput))
	if !validObjectID(tree) {
		return BatchResult{}, fmt.Errorf("write curation tree")
	}
	marker.Tree = tree
	message := batchCommitMessage(marker.Items)
	marker.Commit, err = curator.gitText(ctx, message, "commit-tree", tree, "-p", head)
	if err != nil || marker.Commit == "" {
		return BatchResult{}, fmt.Errorf("write curation commit")
	}
	if err := curator.verifyTransactionCommit(ctx, marker); err != nil {
		return BatchResult{}, err
	}

	artifact, err := os.OpenFile(marker.IndexArtifact, os.O_WRONLY|os.O_CREATE|os.O_EXCL, indexMode.Perm())
	if err != nil {
		return BatchResult{}, fmt.Errorf("create reconciled Git index")
	}
	if _, err := artifact.Write(originalIndex); err != nil {
		_ = artifact.Close()
		_ = os.Remove(marker.IndexArtifact)
		return BatchResult{}, fmt.Errorf("copy Git index")
	}
	if err := artifact.Close(); err != nil {
		_ = os.Remove(marker.IndexArtifact)
		return BatchResult{}, fmt.Errorf("close Git index")
	}
	artifactCreated := true
	markerWritten := false
	defer func() {
		if !markerWritten && artifactCreated {
			_ = os.Remove(marker.IndexArtifact)
		}
	}()
	finalEnvironment := map[string]string{"GIT_INDEX_FILE": marker.IndexArtifact}
	for _, item := range marker.Items {
		if item.TargetPath != "" {
			if item.PostStateExists {
				cacheInfo := item.PostStateGitMode + "," + item.PostStateBlob + "," + item.TargetPath
				if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
					return BatchResult{}, fmt.Errorf("reconcile memory index entry")
				}
			} else if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--force-remove", "--", item.TargetPath); err != nil {
				return BatchResult{}, fmt.Errorf("reconcile removed memory index entry")
			}
		}
		if item.Decision != "pending" {
			if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--force-remove", "--", item.ProposalPath); err != nil {
				return BatchResult{}, fmt.Errorf("reconcile proposal index entry")
			}
		}
	}
	finalIndex, _, err := readRegularFile(marker.IndexArtifact)
	if err != nil {
		return BatchResult{}, fmt.Errorf("read reconciled Git index")
	}
	marker.FinalIndexHash = contentHash(finalIndex)
	if err := syncFile(marker.IndexArtifact); err != nil {
		return BatchResult{}, fmt.Errorf("sync reconciled Git index")
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return BatchResult{}, fmt.Errorf("sync reconciled Git index directory")
	}
	if err := curator.writeTransactionMarker(marker); err != nil {
		return BatchResult{}, err
	}
	markerWritten = true
	if err := curator.injectFault("after_marker"); err != nil {
		return BatchResult{}, err
	}

	lockPath := indexPath + ".lock"
	if err := os.Link(marker.IndexArtifact, lockPath); errors.Is(err, os.ErrExist) {
		if cleanupErr := curator.removeTransaction(marker); cleanupErr == nil {
			markerWritten = false
			artifactCreated = false
		}
		return BatchResult{}, ErrLocked
	} else if err != nil {
		return BatchResult{}, fmt.Errorf("acquire Git index lock")
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return BatchResult{}, fmt.Errorf("sync Git index lock")
	}
	if err := curator.injectFault("after_index_lock"); err != nil {
		return BatchResult{}, err
	}
	currentIndex, _, err := readRegularFile(indexPath)
	if err != nil || contentHash(currentIndex) != marker.OriginalIndexHash {
		if cleanupErr := curator.abortPreparedTransaction(context.WithoutCancel(ctx), marker, gitDirectory); cleanupErr != nil {
			return BatchResult{}, errors.Join(ErrDrift, cleanupErr)
		}
		return BatchResult{}, ErrDrift
	}
	confirmed, err := curator.revalidateBatch(ctx, batch)
	if err != nil || batchPreviewHash(confirmed.Items) != marker.PreviewHash {
		if cleanupErr := curator.abortPreparedTransaction(context.WithoutCancel(ctx), marker, gitDirectory); cleanupErr != nil {
			return BatchResult{}, errors.Join(ErrDrift, cleanupErr)
		}
		return BatchResult{}, ErrDrift
	}
	confirmedEvidenceHash, err := evidenceHash(validateEvidence)
	if err != nil {
		if cleanupErr := curator.abortPreparedTransaction(context.WithoutCancel(ctx), marker, gitDirectory); cleanupErr != nil {
			return BatchResult{}, errors.Join(err, cleanupErr)
		}
		return BatchResult{}, err
	}
	if confirmedEvidenceHash != marker.EvidenceHash {
		if cleanupErr := curator.abortPreparedTransaction(context.WithoutCancel(ctx), marker, gitDirectory); cleanupErr != nil {
			return BatchResult{}, errors.Join(ErrDrift, cleanupErr)
		}
		return BatchResult{}, ErrDrift
	}

	for index, preview := range confirmed.Items {
		if err := curator.applyWorktree(ctx, preview); err != nil {
			if cleanupErr := curator.abortPreparedTransaction(context.WithoutCancel(ctx), marker, gitDirectory); cleanupErr != nil {
				return BatchResult{}, errors.Join(err, cleanupErr)
			}
			return BatchResult{}, err
		}
		if err := curator.injectFault(fmt.Sprintf("after_worktree_%d", index+1)); err != nil {
			return BatchResult{}, err
		}
	}
	if err := curator.injectFault("before_ref_update"); err != nil {
		return BatchResult{}, err
	}
	if _, err := curator.gitInput(ctx, nil, nil, "update-ref", branch, marker.Commit, head); err != nil {
		recoveryContext := context.WithoutCancel(ctx)
		current, headErr := curator.gitText(recoveryContext, nil, "rev-parse", "--verify", "HEAD")
		if headErr != nil {
			return BatchResult{}, errors.Join(ErrRecovery, err)
		}
		if current != marker.Commit {
			if current == marker.Parent {
				if cleanupErr := curator.abortPreparedTransaction(recoveryContext, marker, gitDirectory); cleanupErr != nil {
					return BatchResult{}, errors.Join(ErrDrift, cleanupErr)
				}
			}
			return BatchResult{}, ErrDrift
		}
	}
	if err := curator.injectFault("after_ref_update"); err != nil {
		return BatchResult{}, err
	}
	if err := installOwnedIndex(marker, lockPath); err != nil {
		return BatchResult{}, fmt.Errorf("install Git index: %w", err)
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return BatchResult{}, fmt.Errorf("sync Git directory")
	}
	if err := curator.injectFault("after_index_install"); err != nil {
		return BatchResult{}, err
	}
	for index, item := range marker.Items {
		if item.Decision != "pending" {
			if err := curator.removeRecoveredProposal(item); err != nil {
				return BatchResult{}, err
			}
		}
		if err := curator.injectFault(fmt.Sprintf("after_proposal_removal_%d", index+1)); err != nil {
			return BatchResult{}, err
		}
	}
	result := marker.result()
	if err := curator.injectFault("before_marker_remove"); err != nil {
		return BatchResult{}, err
	}
	if err := curator.removeTransaction(marker); err != nil {
		return BatchResult{}, err
	}
	markerWritten = false
	artifactCreated = false
	return result, nil
}

func (curator *Curator) applyWorktree(ctx context.Context, preview Preview) error {
	preflight := func() error {
		if err := curator.inbox.Confirm(ctx, preview.proposal); err != nil {
			return err
		}
		return curator.inspect(preview.proposal.Proposal)
	}
	validate := func(postState []byte) error {
		if !bytes.Equal(postState, preview.postState) {
			return ErrDrift
		}
		return nil
	}
	var result memory.ReconcileResult
	var err error
	switch preview.Operation {
	case OperationCreatePage:
		result, err = curator.repository.ReconcileCreatePage(ctx, memory.ReconcileCreatePageRequest{
			CreatePageRequest: memory.CreatePageRequest{TargetPath: preview.TargetPath, Title: preview.Title, Body: preview.proposal.Proposal.Knowledge},
			PostStateHash:     preview.postStateHash, Head: preview.head, Preflight: preflight, Validate: validate, Mode: memory.ApplyBaseline,
		})
	case OperationCreateSection:
		result, err = curator.repository.ReconcileCreateSection(ctx, memory.ReconcileCreateSectionRequest{
			CreateSectionRequest: memory.CreateSectionRequest{TargetPath: preview.TargetPath, Heading: preview.Heading, Body: preview.proposal.Proposal.Knowledge},
			ExpectedSectionID:    preview.SectionID, BaselineHash: preview.baselineHash, PostStateHash: preview.postStateHash,
			Head: preview.head, Blob: preview.baselineBlob, Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			}, Mode: memory.ApplyBaseline,
		})
	case OperationReplaceSection:
		result, err = curator.repository.ReconcileReplaceSection(ctx, memory.ReconcileRequest{
			Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, Replacement: preview.Replacement,
			BaselineHash: preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob,
			Preflight: preflight, Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			}, Mode: memory.ApplyBaseline,
		})
	case OperationRemoveSection:
		result, err = curator.repository.ReconcileRemoveSection(ctx, memory.ReconcileRemoveSectionRequest{
			RemoveSectionRequest: memory.RemoveSectionRequest{Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, ContentHash: preview.ContentHash},
			BaselineHash:         preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob,
			Preflight: preflight, Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			}, Mode: memory.ApplyBaseline,
		})
	case OperationRemovePage:
		result, err = curator.repository.ReconcileRemovePage(ctx, memory.ReconcileRemovePageRequest{
			RemovePageRequest: memory.RemovePageRequest{Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, ContentHash: preview.ContentHash},
			BaselineHash:      preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob, FileMode: preview.fileMode,
			Preflight: preflight, Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) || postState != nil {
					return ErrDrift
				}
				return nil
			}, Mode: memory.ApplyBaseline,
		})
	case OperationDiscard:
		if err = preflight(); err == nil {
			result.State = memory.ReconcilePostState
		}
	case OperationKeepPending:
		if err = preflight(); err == nil {
			result.State = memory.ReconcilePostState
		}
	default:
		return ErrUnsupported
	}
	if err != nil {
		return err
	}
	if result.State != memory.ReconcilePostState {
		return ErrDrift
	}
	return nil
}

func (curator *Curator) rollbackTransaction(ctx context.Context, marker transactionMarker) error {
	for index := len(marker.Items) - 1; index >= 0; index-- {
		item := marker.Items[index]
		if item.TargetPath == "" {
			continue
		}
		var baseline, postState []byte
		var err error
		if item.BaselineExists {
			baseline, err = curator.gitInput(ctx, nil, nil, "cat-file", "blob", item.BaselineBlob)
			if err != nil || contentHash(baseline) != item.BaselineHash {
				return ErrRecovery
			}
		}
		if item.PostStateExists {
			postState, err = curator.gitInput(ctx, nil, nil, "cat-file", "blob", item.PostStateBlob)
			if err != nil || contentHash(postState) != item.PostStateHash {
				return ErrRecovery
			}
		}
		if err := curator.repository.Rollback(ctx, memory.RollbackRequest{
			TargetPath: item.TargetPath, Baseline: baseline, PostState: postState, FileMode: os.FileMode(item.BaselineMode),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (curator *Curator) abortPreparedTransaction(ctx context.Context, marker transactionMarker, gitDirectory string) error {
	if err := curator.rollbackTransaction(ctx, marker); err != nil {
		return err
	}
	lockPath := marker.IndexPath + ".lock"
	if _, err := os.Lstat(lockPath); err == nil {
		if err := removeOwnedIndexLock(marker, lockPath); err != nil {
			return err
		}
		if err := syncDirectory(gitDirectory); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrLocked
	}
	return curator.removeTransaction(marker)
}

func (curator *Curator) confirmPostState(marker transactionMarker) error {
	for _, item := range marker.Items {
		if item.TargetPath == "" {
			continue
		}
		contents, mode, err := curator.readControlledFile(item.TargetPath)
		if !item.PostStateExists {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return ErrRecovery
		}
		if err != nil || contentHash(contents) != item.PostStateHash || gitMode(mode) != item.PostStateGitMode {
			return ErrRecovery
		}
	}
	return nil
}

func (curator *Curator) removeRecoveredProposal(item transactionItem) error {
	contents, _, err := curator.readControlledFile(item.ProposalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || contentHash(contents) != item.ProposalHash {
		return ErrRecovery
	}
	return curator.removeExact(item.ProposalPath, contents)
}

func (curator *Curator) verifyTransactionCommit(ctx context.Context, marker transactionMarker) error {
	parent, err := curator.gitText(ctx, nil, "rev-parse", marker.Commit+"^")
	if err != nil || parent != marker.Parent {
		return ErrRecovery
	}
	tree, err := curator.gitText(ctx, nil, "rev-parse", marker.Commit+"^{tree}")
	if err != nil || tree != marker.Tree {
		return ErrRecovery
	}
	changed, err := curator.gitInput(ctx, nil, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", marker.Commit)
	if err != nil {
		return ErrRecovery
	}
	got := strings.Split(strings.TrimSpace(string(changed)), "\n")
	want := make([]string, 0, len(marker.Items)*2)
	message, err := curator.gitText(ctx, nil, "show", "-s", "--format=%B", marker.Commit)
	if err != nil {
		return ErrRecovery
	}
	for _, item := range marker.Items {
		proposalBlob, err := curator.gitText(ctx, nil, "rev-parse", marker.Parent+":"+item.ProposalPath)
		if err != nil || proposalBlob != item.ProposalBlob {
			return ErrRecovery
		}
		proposalContents, err := curator.gitInput(ctx, nil, nil, "cat-file", "blob", item.ProposalBlob)
		if err != nil || contentHash(proposalContents) != item.ProposalHash {
			return ErrRecovery
		}
		if item.TargetPath != "" {
			want = append(want, item.TargetPath)
			if item.BaselineExists {
				mode, blob, err := curator.treeEntry(ctx, marker.Parent, item.TargetPath)
				if err != nil || blob != item.BaselineBlob || gitMode(os.FileMode(item.BaselineMode)) != mode {
					return ErrRecovery
				}
				baseline, err := curator.gitInput(ctx, nil, nil, "cat-file", "blob", item.BaselineBlob)
				if err != nil || contentHash(baseline) != item.BaselineHash {
					return ErrRecovery
				}
			} else if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", marker.Parent+":"+item.TargetPath); err == nil {
				return ErrRecovery
			}
			if item.PostStateExists {
				blob, err := curator.gitText(ctx, nil, "rev-parse", marker.Commit+":"+item.TargetPath)
				if err != nil || blob != item.PostStateBlob {
					return ErrRecovery
				}
				postState, err := curator.gitInput(ctx, nil, nil, "cat-file", "blob", item.PostStateBlob)
				if err != nil || contentHash(postState) != item.PostStateHash {
					return ErrRecovery
				}
			} else if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", marker.Commit+":"+item.TargetPath); err == nil {
				return ErrRecovery
			}
		}
		if item.Decision == "pending" {
			committedBlob, err := curator.gitText(ctx, nil, "rev-parse", marker.Commit+":"+item.ProposalPath)
			if err != nil || committedBlob != item.ProposalBlob {
				return ErrRecovery
			}
		} else {
			want = append(want, item.ProposalPath)
			if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", marker.Commit+":"+item.ProposalPath); err == nil {
				return ErrRecovery
			}
		}
		trailer := "Git-Push-Memories-Proposal: " + item.ProposalID + "\nGit-Push-Memories-Decision: " + item.Decision
		if item.Reason != "" {
			trailer += "\nGit-Push-Memories-Reason: " + string(item.Reason)
		}
		if !strings.Contains(message, trailer) {
			return ErrRecovery
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return ErrRecovery
	}
	return nil
}

func (curator *Curator) transactionMarkerPath() string {
	return filepath.Join(curator.cfg.StateRoot, "curation-transaction.json")
}

func (curator *Curator) writeTransactionMarker(marker transactionMarker) error {
	encoded, err := json.Marshal(marker)
	if err != nil {
		return ErrRecovery
	}
	temporary, err := os.CreateTemp(curator.cfg.StateRoot, ".curation-marker-*.tmp")
	if err != nil {
		return fmt.Errorf("create curation marker")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write curation marker")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync curation marker")
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close curation marker")
	}
	if err := unix.Renameat2(unix.AT_FDCWD, temporaryPath, unix.AT_FDCWD, curator.transactionMarkerPath(), unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("install curation marker")
	}
	return syncDirectory(curator.cfg.StateRoot)
}

func (curator *Curator) readTransactionMarker() (transactionMarker, error) {
	var marker transactionMarker
	contents, mode, err := readRegularFile(curator.transactionMarkerPath())
	if err != nil {
		return marker, err
	}
	if mode.Perm() != 0o600 {
		return marker, ErrRecovery
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return marker, ErrRecovery
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return marker, ErrRecovery
	}
	if marker.SchemaVersion != transactionSchemaVersion || !transactionIDPattern.MatchString(marker.ID) || marker.MemoryRoot == "" || marker.Branch != "refs/heads/main" ||
		!validObjectID(marker.Parent) || !validObjectID(marker.Commit) || !validObjectID(marker.Tree) || !filepath.IsAbs(marker.IndexPath) ||
		!filepath.IsAbs(marker.IndexArtifact) || filepath.Base(marker.IndexArtifact) != ".git-push-memories-curation-index-"+marker.ID || !validContentHash(marker.OriginalIndexHash) ||
		!validContentHash(marker.FinalIndexHash) || !validContentHash(marker.PreviewHash) || !validContentHash(marker.EvidenceHash) || len(marker.Items) == 0 || len(marker.Items) > MaxBatchItems {
		return marker, ErrRecovery
	}
	proposalIDs := make(map[string]struct{}, len(marker.Items))
	targetPaths := make(map[string]struct{}, len(marker.Items))
	for _, item := range marker.Items {
		proposalBase := path.Base(item.ProposalPath)
		validProposalPath := path.Clean(item.ProposalPath) == item.ProposalPath && path.Dir(item.ProposalPath) == "proposals" && strings.HasPrefix(proposalBase, item.ProposalID+"-") && path.Ext(proposalBase) == ".md"
		_, duplicateProposal := proposalIDs[item.ProposalID]
		proposalIDs[item.ProposalID] = struct{}{}
		validDiscard := item.Operation == OperationDiscard && item.Decision == "discarded" && validDiscardReason(item.Reason) && item.TargetPath == "" && !item.BaselineExists && !item.PostStateExists
		validPending := item.Operation == OperationKeepPending && item.Decision == "pending" && item.Reason == "" && item.TargetPath == "" && item.SectionID == "" && !item.BaselineExists && !item.PostStateExists
		validOperation := item.Operation == OperationCreatePage || item.Operation == OperationCreateSection || item.Operation == OperationReplaceSection || item.Operation == OperationRemoveSection || item.Operation == OperationRemovePage
		validMutation := validOperation && item.Decision == "consumed" && item.Reason == "" && item.SectionID != "" && curator.cfg.IsMemoryPath(item.TargetPath) &&
			item.BaselineExists == (item.Operation != OperationCreatePage) && item.PostStateExists == (item.Operation != OperationRemovePage)
		_, duplicateTarget := targetPaths[item.TargetPath]
		if item.TargetPath != "" {
			targetPaths[item.TargetPath] = struct{}{}
		}
		validBaseline := !item.BaselineExists && item.BaselineBlob == "" && item.BaselineHash == "" && item.BaselineMode == 0 ||
			item.BaselineExists && validObjectID(item.BaselineBlob) && validContentHash(item.BaselineHash) && item.BaselineMode <= 0o777
		validPostState := !item.PostStateExists && item.PostStateBlob == "" && item.PostStateGitMode == "" &&
			((item.Operation == OperationDiscard || item.Operation == OperationKeepPending) && item.PostStateHash == "" || item.Operation == OperationRemovePage && item.PostStateHash == contentHash(nil)) ||
			item.PostStateExists && validObjectID(item.PostStateBlob) && validContentHash(item.PostStateHash) && (item.PostStateGitMode == "100644" || item.PostStateGitMode == "100755")
		if !transactionIDPattern.MatchString(item.ProposalID) || !validProposalPath || duplicateProposal || duplicateTarget && item.TargetPath != "" ||
			!validObjectID(item.ProposalBlob) || !validContentHash(item.ProposalHash) || !(validDiscard || validPending || validMutation) || !validBaseline || !validPostState {
			return marker, ErrRecovery
		}
	}
	return marker, nil
}

func (curator *Curator) removeTransaction(marker transactionMarker) error {
	if err := os.Remove(curator.transactionMarkerPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrRecovery
	}
	if err := syncDirectory(curator.cfg.StateRoot); err != nil {
		return ErrRecovery
	}
	if err := curator.injectFault("after_marker_remove"); err != nil {
		return err
	}
	if err := os.Remove(marker.IndexArtifact); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrRecovery
	}
	return syncDirectory(filepath.Dir(marker.IndexArtifact))
}

func (curator *Curator) cleanupTransactionArtifacts(ctx context.Context) error {
	entries, err := os.ReadDir(curator.cfg.StateRoot)
	if err != nil {
		return ErrRecovery
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !transactionMarkerTemporaryPattern.MatchString(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(curator.cfg.StateRoot, entry.Name())); err != nil {
			return ErrRecovery
		}
	}
	if err := syncDirectory(curator.cfg.StateRoot); err != nil {
		return ErrRecovery
	}
	gitDirectory, err := curator.gitPath(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ErrRecovery
	}
	entries, err = os.ReadDir(gitDirectory)
	if err != nil {
		return ErrRecovery
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !transactionIndexPattern.MatchString(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(gitDirectory, entry.Name())); err != nil {
			return ErrRecovery
		}
	}
	if err := syncDirectory(gitDirectory); err != nil {
		return ErrRecovery
	}
	return nil
}

func (curator *Curator) readControlledFile(relativePath string) ([]byte, os.FileMode, error) {
	fd, err := unix.Openat2(int(curator.root.Fd()), relativePath, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_NONBLOCK | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, 0, os.ErrNotExist
		}
		return nil, 0, err
	}
	file := os.NewFile(uintptr(fd), relativePath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, 0, ErrRecovery
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, 0, ErrRecovery
	}
	contents, err := io.ReadAll(file)
	return contents, info.Mode(), err
}

func (marker transactionMarker) result() BatchResult {
	result := BatchResult{Commit: marker.Commit, Items: make([]Result, len(marker.Items))}
	for index, item := range marker.Items {
		result.Items[index] = Result{
			ProposalID: item.ProposalID, TargetPath: item.TargetPath, SectionID: item.SectionID,
			Decision: item.Decision, Reason: item.Reason, Commit: marker.Commit,
		}
	}
	return result
}

func transactionItemFromPreview(preview Preview) transactionItem {
	item := transactionItem{
		ProposalID: preview.ProposalID, ProposalPath: preview.ProposalPath, ProposalBlob: preview.proposal.Blob,
		ProposalHash: contentHash(preview.proposal.Contents), Operation: preview.Operation, TargetPath: preview.TargetPath,
		SectionID: preview.SectionID, Decision: "consumed", Reason: preview.Reason,
	}
	switch preview.Operation {
	case OperationDiscard:
		item.Decision = "discarded"
	case OperationKeepPending:
		item.Decision = "pending"
	default:
		item.BaselineExists = preview.Operation != OperationCreatePage
		item.BaselineBlob = preview.baselineBlob
		item.BaselineHash = preview.baselineHash
		item.BaselineMode = uint32(preview.fileMode.Perm())
		if item.BaselineMode == 0 && item.BaselineExists {
			item.BaselineMode = modeFromGit(preview.gitMode)
		}
		item.PostStateExists = preview.Operation != OperationRemovePage
		item.PostStateHash = preview.postStateHash
		item.PostStateGitMode = preview.gitMode
	}
	return item
}

func batchPreviewHash(previews []Preview) string {
	type digestItem struct {
		Operation, ProposalID, ProposalPath, ProposalBlob, TargetPath, SectionID, ContentHash, Heading, Replacement, Reason string
		Head, GitMode, BaselineHash, BaselineBlob, PostStateHash, ProposalHash, PostStateBytesHash, Diff                    string
	}
	items := make([]digestItem, len(previews))
	for index, preview := range previews {
		items[index] = digestItem{
			Operation: string(preview.Operation), ProposalID: preview.ProposalID, ProposalPath: preview.ProposalPath, ProposalBlob: preview.proposal.Blob,
			TargetPath: preview.TargetPath, SectionID: preview.SectionID, ContentHash: preview.ContentHash, Heading: preview.Heading,
			Replacement: preview.Replacement, Reason: string(preview.Reason), Head: preview.head, GitMode: preview.gitMode,
			BaselineHash: preview.baselineHash, BaselineBlob: preview.baselineBlob, PostStateHash: preview.postStateHash,
			ProposalHash: contentHash(preview.proposal.Contents), PostStateBytesHash: contentHash(preview.postState), Diff: preview.Diff,
		}
	}
	encoded, _ := json.Marshal(items)
	return contentHash(encoded)
}

func batchCommitMessage(items []transactionItem) []byte {
	var message strings.Builder
	title := "Curate memory proposal batch"
	if len(items) == 1 {
		title = "Curate memory proposal"
		if items[0].Decision == "discarded" {
			title = "Discard memory proposal"
		}
	}
	message.WriteString(title + "\n")
	for _, item := range items {
		message.WriteString("\nGit-Push-Memories-Proposal: ")
		message.WriteString(item.ProposalID)
		message.WriteString("\nGit-Push-Memories-Decision: ")
		message.WriteString(item.Decision)
		if item.Reason != "" {
			message.WriteString("\nGit-Push-Memories-Reason: ")
			message.WriteString(string(item.Reason))
		}
		message.WriteByte('\n')
	}
	return []byte(message.String())
}

func evidenceHash(validate EvidenceValidator) (string, error) {
	if validate == nil {
		return contentHash(nil), nil
	}
	value, err := validate()
	if err != nil {
		return "", err
	}
	if !validContentHash(value) {
		return "", ErrDrift
	}
	return value, nil
}

func (curator *Curator) injectFault(stage string) error {
	if curator.fault == nil {
		return nil
	}
	return curator.fault(stage)
}

func randomTransactionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func contentHash(contents []byte) string {
	hash := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func validContentHash(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func modeFromGit(mode string) uint32 {
	if mode == "100755" {
		return 0o755
	}
	return 0o644
}

func gitMode(mode os.FileMode) string {
	if mode.Perm()&0o111 != 0 {
		return "100755"
	}
	return "100644"
}

func syncFile(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func regularFileInfo(filePath string) (os.FileInfo, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrRecovery
	}
	return info, nil
}

func removeOwnedIndexLock(marker transactionMarker, lockPath string) error {
	artifact, err := regularFileInfo(marker.IndexArtifact)
	if err != nil {
		return ErrRecovery
	}
	lock, err := regularFileInfo(lockPath)
	if err != nil || !os.SameFile(artifact, lock) {
		return ErrLocked
	}
	return os.Remove(lockPath)
}

func installOwnedIndex(marker transactionMarker, lockPath string) error {
	artifact, err := regularFileInfo(marker.IndexArtifact)
	if err != nil {
		return ErrRecovery
	}
	lock, err := regularFileInfo(lockPath)
	if err != nil || !os.SameFile(artifact, lock) {
		return ErrLocked
	}
	return os.Rename(lockPath, marker.IndexPath)
}

func parseIndexEntry(entry []byte, relativePath string) (string, string, error) {
	if bytes.Count(entry, []byte{0}) != 1 {
		return "", "", ErrDrift
	}
	entry = bytes.TrimSuffix(entry, []byte{0})
	tab := bytes.IndexByte(entry, '\t')
	fields := strings.Fields(string(entry[:max(tab, 0)]))
	if tab < 0 || len(fields) != 3 || fields[2] != "0" || string(entry[tab+1:]) != relativePath || (fields[0] != "100644" && fields[0] != "100755") {
		return "", "", ErrDrift
	}
	return fields[0], fields[1], nil
}

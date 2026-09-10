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
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
	"golang.org/x/sys/unix"
)

var (
	ErrDrift         = errors.New("curation preview drifted")
	ErrLocked        = errors.New("Git index is locked")
	ErrUnsupported   = errors.New("proposal action is not supported by this curation phase")
	ErrInvalidReason = errors.New("invalid discard reason")
)

type Operation string

const (
	OperationCreatePage     Operation = "create_page"
	OperationCreateSection  Operation = "create_section"
	OperationReplaceSection Operation = "replace_section"
	OperationRemoveSection  Operation = "remove_section"
	OperationRemovePage     Operation = "remove_page"
	OperationDiscard        Operation = "discard_proposal"
)

type DiscardReason string

const (
	DiscardNotRelevant DiscardReason = "not_relevant"
	DiscardIncorrect   DiscardReason = "incorrect"
	DiscardSensitive   DiscardReason = "sensitive"
	DiscardWrongTarget DiscardReason = "wrong_target"
	DiscardSuperseded  DiscardReason = "superseded"
	DiscardDuplicate   DiscardReason = "duplicate"
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

type RelatedProposal struct {
	ID                 string       `json:"id"`
	Path               string       `json:"path"`
	Title              string       `json:"title"`
	SuggestedAction    inbox.Action `json:"suggested_action"`
	Target             string       `json:"target,omitempty"`
	Knowledge          string       `json:"knowledge"`
	KnowledgeTruncated bool         `json:"knowledge_truncated,omitempty"`
	Score              int          `json:"score"`
}

type RelatedProposals struct {
	Items     []RelatedProposal `json:"items"`
	Truncated bool              `json:"truncated"`
}

const (
	relatedProposalLimit  = 3
	relatedKnowledgeBytes = 256
	relatedEvidenceBudget = 4096
)

type PreviewRequest struct {
	Operation   Operation
	ProposalID  string
	TargetPath  string
	SectionID   string
	ContentHash string
	Heading     string
	Replacement string
	Reason      DiscardReason
}

type Preview struct {
	Operation    Operation     `json:"operation"`
	ProposalID   string        `json:"proposal_id"`
	ProposalPath string        `json:"proposal_path"`
	Title        string        `json:"title"`
	Action       inbox.Action  `json:"suggested_action"`
	Knowledge    string        `json:"knowledge"`
	SourcePaths  []string      `json:"source_paths"`
	TargetPath   string        `json:"target_path,omitempty"`
	SectionID    string        `json:"section_id,omitempty"`
	ContentHash  string        `json:"content_hash,omitempty"`
	Heading      string        `json:"heading,omitempty"`
	Replacement  string        `json:"replacement,omitempty"`
	Reason       DiscardReason `json:"reason,omitempty"`
	Diff         string        `json:"diff"`

	head          string
	gitMode       string
	baselineHash  string
	baselineBlob  string
	baseline      []byte
	postStateHash string
	postState     []byte
	fileMode      os.FileMode
	proposal      inbox.Snapshot
	request       PreviewRequest
}

type Result struct {
	ProposalID string        `json:"proposal_id"`
	TargetPath string        `json:"target_path,omitempty"`
	SectionID  string        `json:"section_id,omitempty"`
	Decision   string        `json:"decision"`
	Reason     DiscardReason `json:"reason,omitempty"`
	Commit     string        `json:"commit"`
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

func (curator *Curator) RelatedProposals(ctx context.Context, expected Preview, query string) (RelatedProposals, error) {
	snapshots, err := curator.inbox.List(ctx)
	if err != nil {
		return RelatedProposals{}, err
	}
	candidates := make([]RelatedProposal, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Head != expected.head {
			return RelatedProposals{}, ErrDrift
		}
		proposal := snapshot.Proposal
		if err := curator.inspect(proposal); err != nil {
			return RelatedProposals{}, err
		}
		if proposal.ID == expected.ProposalID {
			continue
		}
		score := retrieval.LexicalScore(query, proposal.Title+" "+proposal.Knowledge)
		if score == 0 {
			continue
		}
		knowledge := strings.TrimSuffix(proposal.Knowledge, "\n")
		excerpt, truncated := utf8Excerpt(knowledge, relatedKnowledgeBytes)
		candidates = append(candidates, RelatedProposal{
			ID: proposal.ID, Path: proposal.Path, Title: proposal.Title, SuggestedAction: proposal.SuggestedAction, Target: proposal.Target,
			Knowledge: excerpt, KnowledgeTruncated: truncated, Score: score,
		})
	}
	slices.SortFunc(candidates, func(left, right RelatedProposal) int {
		if left.Score != right.Score {
			return right.Score - left.Score
		}
		return strings.Compare(left.Path, right.Path)
	})
	total := len(candidates)
	if len(candidates) > relatedProposalLimit {
		candidates = candidates[:relatedProposalLimit]
	}
	for count := len(candidates); count >= 0; count-- {
		result := RelatedProposals{Items: append([]RelatedProposal(nil), candidates[:count]...), Truncated: count < total}
		for _, candidate := range result.Items {
			result.Truncated = result.Truncated || candidate.KnowledgeTruncated
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return RelatedProposals{}, err
		}
		if len(encoded) <= relatedEvidenceBudget {
			return result, nil
		}
	}
	return RelatedProposals{}, fmt.Errorf("related proposal evidence exceeds budget")
}

func (curator *Curator) RevalidatePreview(ctx context.Context, expected Preview) error {
	_, err := curator.revalidatePreview(ctx, expected)
	return err
}

func (curator *Curator) revalidatePreview(ctx context.Context, expected Preview) (Preview, error) {
	fresh, err := curator.preview(ctx, expected.proposal, expected.request)
	if err != nil {
		return Preview{}, err
	}
	if !samePreview(fresh, expected) {
		return Preview{}, ErrDrift
	}
	return fresh, nil
}

func utf8Excerpt(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], true
}

func (curator *Curator) Preview(ctx context.Context, request PreviewRequest) (Preview, error) {
	snapshot, err := curator.inbox.Read(ctx, request.ProposalID)
	if err != nil {
		return Preview{}, err
	}
	return curator.preview(ctx, snapshot, request)
}

func (curator *Curator) preview(ctx context.Context, snapshot inbox.Snapshot, request PreviewRequest) (Preview, error) {
	operation, targetPath, sectionID, heading := request.Operation, request.TargetPath, request.SectionID, request.Heading
	if !supports(operation, snapshot.Proposal.SuggestedAction) {
		return Preview{}, ErrUnsupported
	}
	if err := curator.confirmConfig(ctx, snapshot.Head); err != nil {
		return Preview{}, err
	}
	if operation != OperationDiscard && request.Reason != "" {
		return Preview{}, memory.ErrInvalidPath
	}
	if operation != OperationDiscard && !curator.cfg.IsMemoryPath(targetPath) {
		return Preview{}, memory.ErrInvalidPath
	}
	if err := curator.inbox.Confirm(ctx, snapshot); err != nil {
		return Preview{}, err
	}
	if err := curator.inspect(snapshot.Proposal); err != nil {
		return Preview{}, err
	}
	preview := Preview{
		Operation: operation, ProposalID: snapshot.Proposal.ID, ProposalPath: snapshot.Proposal.Path, Title: snapshot.Proposal.Title,
		Action: snapshot.Proposal.SuggestedAction, Knowledge: snapshot.Proposal.Knowledge, SourcePaths: append([]string(nil), snapshot.Proposal.SourcePaths...),
		TargetPath: targetPath, ContentHash: request.ContentHash, Heading: heading, Replacement: request.Replacement, Reason: request.Reason, proposal: snapshot, request: request,
	}
	var targetDiff string
	switch operation {
	case OperationCreatePage:
		if sectionID != "" || request.ContentHash != "" || heading != "" || request.Replacement != "" || !memory.IsCreatePagePath(targetPath) {
			return Preview{}, memory.ErrInvalidPath
		}
		page, err := curator.repository.PreviewCreatePage(ctx, memory.CreatePageRequest{
			TargetPath: targetPath, Title: snapshot.Proposal.Title, Body: snapshot.Proposal.Knowledge,
		})
		if err != nil {
			return Preview{}, err
		}
		preview.SectionID = page.Target.SectionID
		preview.head = page.Head
		preview.gitMode = "100644"
		preview.postStateHash = page.PostStateHash
		preview.postState = append([]byte(nil), page.PostState...)
		targetDiff = page.Diff
	case OperationCreateSection:
		if sectionID != "" || request.ContentHash != "" || heading == "" || request.Replacement != "" {
			return Preview{}, memory.ErrInvalidPath
		}
		section, err := curator.repository.PreviewCreateSection(ctx, memory.CreateSectionRequest{
			TargetPath: targetPath, Heading: heading, Body: snapshot.Proposal.Knowledge,
		})
		if err != nil {
			return Preview{}, err
		}
		preview.SectionID = section.Target.SectionID
		preview.head = section.Head
		mode, blob, err := curator.treeEntry(ctx, section.Head, targetPath)
		if err != nil || blob != section.Blob {
			return Preview{}, ErrDrift
		}
		preview.gitMode = mode
		preview.baselineHash = section.BaselineHash
		preview.baselineBlob = section.Blob
		preview.baseline = append([]byte(nil), section.Baseline...)
		preview.postStateHash = section.PostStateHash
		preview.postState = append([]byte(nil), section.PostState...)
		targetDiff = section.Diff
	case OperationReplaceSection:
		if sectionID == "" || request.ContentHash == "" || heading != "" || request.Replacement == "" {
			return Preview{}, memory.ErrInvalidPath
		}
		preview.SectionID = sectionID
		if err := curator.inspectKnowledge(snapshot.Proposal, request.Replacement); err != nil {
			return Preview{}, err
		}
		replacement, err := curator.repository.PreviewReplaceSection(ctx, memory.PreviewRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, Replacement: request.Replacement,
		})
		if err != nil {
			return Preview{}, err
		}
		if replacement.SectionHash != request.ContentHash {
			return Preview{}, ErrDrift
		}
		mode, blob, err := curator.treeEntry(ctx, replacement.Head, targetPath)
		if err != nil || blob != replacement.Blob {
			return Preview{}, ErrDrift
		}
		preview.gitMode = mode
		preview.head = replacement.Head
		preview.baselineHash = replacement.BaselineHash
		preview.baselineBlob = replacement.Blob
		preview.baseline = append([]byte(nil), replacement.Baseline...)
		preview.postStateHash = replacement.PostStateHash
		preview.postState = append([]byte(nil), replacement.PostState...)
		targetDiff = replacement.Diff
	case OperationRemoveSection:
		if sectionID == "" || request.ContentHash == "" || heading != "" || request.Replacement != "" {
			return Preview{}, memory.ErrInvalidPath
		}
		preview.SectionID = sectionID
		removal, err := curator.repository.PreviewRemoveSection(ctx, memory.RemoveSectionRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, ContentHash: request.ContentHash,
		})
		if err != nil {
			return Preview{}, err
		}
		mode, blob, err := curator.treeEntry(ctx, removal.Head, targetPath)
		if err != nil || blob != removal.Blob {
			return Preview{}, ErrDrift
		}
		preview.gitMode = mode
		preview.head = removal.Head
		preview.baselineHash = removal.BaselineHash
		preview.baselineBlob = removal.Blob
		preview.baseline = append([]byte(nil), removal.Baseline...)
		preview.postStateHash = removal.PostStateHash
		preview.postState = append([]byte(nil), removal.PostState...)
		targetDiff = removal.Diff
	case OperationRemovePage:
		if sectionID == "" || request.ContentHash == "" || heading != "" || request.Replacement != "" {
			return Preview{}, memory.ErrInvalidPath
		}
		preview.SectionID = sectionID
		removal, err := curator.repository.PreviewRemovePage(ctx, memory.RemovePageRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, ContentHash: request.ContentHash,
		})
		if err != nil {
			return Preview{}, err
		}
		mode, blob, err := curator.treeEntry(ctx, removal.Head, targetPath)
		if err != nil || blob != removal.Blob {
			return Preview{}, ErrDrift
		}
		preview.gitMode = mode
		preview.head = removal.Head
		preview.baselineHash = removal.BaselineHash
		preview.baselineBlob = removal.Blob
		preview.baseline = append([]byte(nil), removal.Baseline...)
		preview.postStateHash = removal.PostStateHash
		preview.fileMode = removal.FileMode
		targetDiff = removal.Diff
	case OperationDiscard:
		if !validDiscardReason(request.Reason) {
			return Preview{}, ErrInvalidReason
		}
		if targetPath != "" || sectionID != "" || request.ContentHash != "" || heading != "" || request.Replacement != "" {
			return Preview{}, memory.ErrInvalidPath
		}
		preview.head = snapshot.Head
	default:
		return Preview{}, ErrUnsupported
	}
	if preview.head != snapshot.Head {
		return Preview{}, ErrDrift
	}
	if targetDiff == "" {
		preview.Diff = unifieddiff.Delete(snapshot.Proposal.Path, snapshot.Contents)
	} else {
		preview.Diff = targetDiff + "\n" + unifieddiff.Delete(snapshot.Proposal.Path, snapshot.Contents)
	}
	return preview, nil
}

func supports(operation Operation, action inbox.Action) bool {
	switch operation {
	case OperationCreatePage, OperationCreateSection:
		return action == inbox.ActionCreate
	case OperationReplaceSection:
		return action == inbox.ActionUpdate
	case OperationRemoveSection, OperationRemovePage:
		return action == inbox.ActionRemove
	case OperationDiscard:
		return true
	default:
		return false
	}
}

func validDiscardReason(reason DiscardReason) bool {
	switch reason {
	case DiscardNotRelevant, DiscardIncorrect, DiscardSensitive, DiscardWrongTarget, DiscardSuperseded, DiscardDuplicate:
		return true
	default:
		return false
	}
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

func (curator *Curator) Apply(ctx context.Context, expected Preview) (Result, error) {
	var result Result
	err := repolock.With(ctx, curator.cfg.StateRoot, func() error {
		fresh, err := curator.revalidatePreview(ctx, expected)
		if err != nil {
			return err
		}
		commit, err := curator.commit(ctx, fresh)
		if err != nil {
			return err
		}
		decision := "consumed"
		if fresh.Operation == OperationDiscard {
			decision = "discarded"
		}
		result = Result{ProposalID: fresh.ProposalID, TargetPath: fresh.TargetPath, SectionID: fresh.SectionID, Decision: decision, Reason: fresh.Reason, Commit: commit}
		return nil
	})
	return result, err
}

func samePreview(left, right Preview) bool {
	return left.Operation == right.Operation && left.ProposalID == right.ProposalID && left.ProposalPath == right.ProposalPath && left.Title == right.Title && left.Action == right.Action && left.Knowledge == right.Knowledge &&
		slices.Equal(left.SourcePaths, right.SourcePaths) && left.TargetPath == right.TargetPath && left.SectionID == right.SectionID && left.ContentHash == right.ContentHash && left.Replacement == right.Replacement && left.Reason == right.Reason && left.Diff == right.Diff &&
		left.Heading == right.Heading && left.head == right.head && left.gitMode == right.gitMode && left.fileMode == right.fileMode && left.baselineHash == right.baselineHash && left.baselineBlob == right.baselineBlob && bytes.Equal(left.baseline, right.baseline) &&
		left.postStateHash == right.postStateHash && bytes.Equal(left.postState, right.postState) &&
		left.proposal.Head == right.proposal.Head && left.proposal.Blob == right.proposal.Blob && bytes.Equal(left.proposal.Contents, right.proposal.Contents) && left.request == right.request
}

func (curator *Curator) inspect(proposal inbox.Proposal) error {
	return curator.inspectKnowledge(proposal, proposal.Knowledge)
}

func (curator *Curator) inspectKnowledge(proposal inbox.Proposal, knowledge string) error {
	candidate := struct {
		Title           string   `json:"title"`
		SuggestedAction string   `json:"suggested_action"`
		Target          string   `json:"target,omitempty"`
		Knowledge       string   `json:"knowledge"`
		SourcePaths     []string `json:"source_paths"`
	}{proposal.Title, string(proposal.SuggestedAction), proposal.Target, knowledge, proposal.SourcePaths}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	_, err = curator.guard.InspectProposal(intake.NewProposalCandidate(raw))
	return err
}

func (curator *Curator) commit(ctx context.Context, preview Preview) (string, error) {
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

	blob := ""
	if preview.Operation != OperationRemovePage && preview.Operation != OperationDiscard {
		blob, err = curator.gitText(ctx, preview.postState, "hash-object", "-w", "--stdin")
		if err != nil || blob == "" {
			return "", fmt.Errorf("write memory blob")
		}
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
	cacheInfo := preview.gitMode + "," + blob + "," + preview.TargetPath
	if preview.Operation != OperationDiscard {
		if preview.Operation == OperationRemovePage {
			if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--force-remove", "--", preview.TargetPath); err != nil {
				return "", fmt.Errorf("remove memory from curation tree")
			}
		} else if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
			return "", fmt.Errorf("add memory to curation tree")
		}
	}
	if _, err := curator.gitInput(ctx, commitEnvironment, nil, "update-index", "--force-remove", "--", preview.ProposalPath); err != nil {
		return "", fmt.Errorf("remove proposal from curation tree")
	}
	treeOutput, err := curator.gitInput(ctx, commitEnvironment, nil, "write-tree")
	tree := strings.TrimSpace(string(treeOutput))
	if err != nil || tree == "" {
		return "", fmt.Errorf("write curation tree")
	}
	message := []byte("Curate memory proposal\n\nGit-Push-Memories-Proposal: " + preview.ProposalID + "\nGit-Push-Memories-Decision: consumed\n")
	if preview.Operation == OperationDiscard {
		message = []byte("Discard memory proposal\n\nGit-Push-Memories-Proposal: " + preview.ProposalID + "\nGit-Push-Memories-Decision: discarded\nGit-Push-Memories-Reason: " + string(preview.Reason) + "\n")
	}
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
	if preview.Operation != OperationDiscard {
		if preview.Operation == OperationRemovePage {
			if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--force-remove", "--", preview.TargetPath); err != nil {
				return "", fmt.Errorf("reconcile removed memory index entry")
			}
		} else if _, err := curator.gitInput(ctx, finalEnvironment, nil, "update-index", "--add", "--cacheinfo", cacheInfo); err != nil {
			return "", fmt.Errorf("reconcile memory index entry")
		}
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
	var reconciled memory.ReconcileResult
	switch preview.Operation {
	case OperationCreatePage:
		reconciled, err = curator.repository.ReconcileCreatePage(ctx, memory.ReconcileCreatePageRequest{
			CreatePageRequest: memory.CreatePageRequest{TargetPath: preview.TargetPath, Title: preview.Title, Body: preview.proposal.Proposal.Knowledge},
			PostStateHash:     preview.postStateHash, Head: preview.head, Preflight: preflight, Validate: validate, Mode: memory.ApplyBaseline,
		})
	case OperationCreateSection:
		reconciled, err = curator.repository.ReconcileCreateSection(ctx, memory.ReconcileCreateSectionRequest{
			CreateSectionRequest: memory.CreateSectionRequest{TargetPath: preview.TargetPath, Heading: preview.Heading, Body: preview.proposal.Proposal.Knowledge},
			ExpectedSectionID:    preview.SectionID, BaselineHash: preview.baselineHash, PostStateHash: preview.postStateHash,
			Head: preview.head, Blob: preview.baselineBlob, Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			},
			Mode: memory.ApplyBaseline,
		})
	case OperationReplaceSection:
		reconciled, err = curator.repository.ReconcileReplaceSection(ctx, memory.ReconcileRequest{
			Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, Replacement: preview.Replacement,
			BaselineHash: preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob,
			Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			},
			Mode: memory.ApplyBaseline,
		})
	case OperationRemoveSection:
		reconciled, err = curator.repository.ReconcileRemoveSection(ctx, memory.ReconcileRemoveSectionRequest{
			RemoveSectionRequest: memory.RemoveSectionRequest{Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, ContentHash: preview.ContentHash},
			BaselineHash:         preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob,
			Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) {
					return ErrDrift
				}
				return validate(postState)
			},
			Mode: memory.ApplyBaseline,
		})
	case OperationRemovePage:
		reconciled, err = curator.repository.ReconcileRemovePage(ctx, memory.ReconcileRemovePageRequest{
			RemovePageRequest: memory.RemovePageRequest{Target: memory.Target{Path: preview.TargetPath, SectionID: preview.SectionID}, ContentHash: preview.ContentHash},
			BaselineHash:      preview.baselineHash, PostStateHash: preview.postStateHash, Head: preview.head, Blob: preview.baselineBlob, FileMode: preview.fileMode,
			Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if !bytes.Equal(baseline, preview.baseline) || postState != nil {
					return ErrDrift
				}
				return nil
			},
			Mode: memory.ApplyBaseline,
		})
	case OperationDiscard:
		if err = preflight(); err == nil {
			reconciled.State = memory.ReconcilePostState
		}
	default:
		return "", ErrUnsupported
	}
	if err != nil {
		if rollbackErr := curator.rollback(ctx, preview); rollbackErr != nil {
			return "", errors.Join(err, fmt.Errorf("rollback curation: %w", rollbackErr))
		}
		return "", err
	}
	if reconciled.State != memory.ReconcilePostState {
		return "", ErrDrift
	}
	if _, err := curator.gitInput(ctx, nil, nil, "update-ref", "refs/heads/main", commit, head); err != nil {
		if rollbackErr := curator.rollback(ctx, preview); rollbackErr != nil {
			return "", errors.Join(ErrDrift, fmt.Errorf("rollback curation: %w", rollbackErr))
		}
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
	status := "A"
	if preview.Operation == OperationCreateSection || preview.Operation == OperationReplaceSection || preview.Operation == OperationRemoveSection {
		status = "M"
	} else if preview.Operation == OperationRemovePage {
		status = "D"
	}
	want := []string{"D\t" + preview.ProposalPath}
	if preview.Operation != OperationDiscard {
		want = append(want, status+"\t"+preview.TargetPath)
	}
	got := strings.Split(strings.TrimSpace(string(changed)), "\n")
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("verify curation paths")
	}
	if preview.Operation != OperationDiscard {
		if preview.Operation == OperationRemovePage {
			if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", commit+":"+preview.TargetPath); err == nil {
				return fmt.Errorf("verify curation memory removal")
			}
		} else {
			stored, err := curator.gitText(ctx, nil, "rev-parse", commit+":"+preview.TargetPath)
			if err != nil || stored != blob {
				return fmt.Errorf("verify curation memory")
			}
			mode, storedBlob, err := curator.treeEntry(ctx, commit, preview.TargetPath)
			if err != nil || mode != preview.gitMode || storedBlob != blob {
				return fmt.Errorf("verify curation memory mode")
			}
		}
	}
	if _, err := curator.gitInput(ctx, nil, nil, "cat-file", "-e", commit+":"+preview.ProposalPath); err == nil {
		return fmt.Errorf("verify proposal consumption")
	}
	return nil
}

func (curator *Curator) treeEntry(ctx context.Context, revision, relativePath string) (string, string, error) {
	entry, err := curator.gitInput(ctx, nil, nil, "ls-tree", "-z", revision, "--", relativePath)
	if err != nil || bytes.Count(entry, []byte{0}) != 1 {
		return "", "", ErrDrift
	}
	entry = bytes.TrimSuffix(entry, []byte{0})
	tab := bytes.IndexByte(entry, '\t')
	fields := strings.Fields(string(entry[:max(tab, 0)]))
	if tab < 0 || len(fields) != 3 || fields[1] != "blob" || string(entry[tab+1:]) != relativePath || (fields[0] != "100644" && fields[0] != "100755") {
		return "", "", ErrDrift
	}
	return fields[0], fields[2], nil
}

func (curator *Curator) rollback(ctx context.Context, preview Preview) error {
	switch preview.Operation {
	case OperationCreatePage:
		return curator.repository.Rollback(ctx, memory.RollbackRequest{TargetPath: preview.TargetPath, PostState: preview.postState})
	case OperationCreateSection, OperationReplaceSection, OperationRemoveSection:
		return curator.repository.Rollback(ctx, memory.RollbackRequest{TargetPath: preview.TargetPath, Baseline: preview.baseline, PostState: preview.postState})
	case OperationRemovePage:
		return curator.repository.Rollback(ctx, memory.RollbackRequest{TargetPath: preview.TargetPath, Baseline: preview.baseline, FileMode: preview.fileMode})
	case OperationDiscard:
		return nil
	default:
		return ErrUnsupported
	}
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

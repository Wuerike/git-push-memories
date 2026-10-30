package promotion

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
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
	"golang.org/x/sys/unix"
)

var (
	ErrDrift         = errors.New("promotion preview drifted")
	ErrLocked        = errors.New("Git index is locked")
	ErrUnsupported   = errors.New("operation is not supported")
	ErrInvalidReason = errors.New("invalid discard reason")
	ErrInvalidBatch  = errors.New("invalid promotion batch")
	ErrRecovery      = errors.New("promotion transaction recovery failed")
)

type Operation string

const (
	OperationCreatePage     Operation = "create_page"
	OperationCreateSection  Operation = "create_section"
	OperationReplaceSection Operation = "replace_section"
	OperationRemoveSection  Operation = "remove_section"
	OperationRemovePage     Operation = "remove_page"
	OperationDiscard        Operation = "discard_proposal"
	OperationRewritePage    Operation = "rewrite_page"
	OperationFoldProposal   Operation = "fold_proposal"
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
	ID      string `json:"id"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type List struct {
	Categories []string  `json:"categories"`
	Proposals  []Summary `json:"proposals"`
}

type RelatedProposal struct {
	ID               string `json:"id"`
	Path             string `json:"path"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	ContentTruncated bool   `json:"content_truncated,omitempty"`
	Score            int    `json:"score"`
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
	FoldInto    string
}

type Preview struct {
	Operation    Operation     `json:"operation"`
	ProposalID   string        `json:"proposal_id"`
	ProposalPath string        `json:"proposal_path"`
	Title        string        `json:"title"`
	Content      string        `json:"content"`
	TargetPath   string        `json:"target_path,omitempty"`
	SectionID    string        `json:"section_id,omitempty"`
	ContentHash  string        `json:"content_hash,omitempty"`
	Heading      string        `json:"heading,omitempty"`
	Replacement  string        `json:"replacement,omitempty"`
	Reason       DiscardReason `json:"reason,omitempty"`
	FoldInto     string        `json:"fold_into,omitempty"`
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

const MaxBatchItems = 16

type BatchPreview struct {
	Items []Preview `json:"items"`
	Diff  string    `json:"diff"`
}

type BatchResult struct {
	Items  []Result `json:"items"`
	Commit string   `json:"commit"`
}

type EvidenceValidator func() (string, error)

type Promoter struct {
	cfg        config.Config
	root       *os.File
	rootPath   string
	inbox      *inbox.Inbox
	repository *memory.Repository
	branch     string // short name of the branch the worktree must be on
	fault      func(string) error
}

// New opens a promoter over the worktree at root, which must be on branch.
func New(cfg config.Config, rootDir, branch string) (*Promoter, error) {
	rootPath, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	root, err := os.Open(rootPath)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	proposalInbox, err := inbox.New(rootPath, cfg.StateRoot, branch)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	repository, err := memory.Open(rootPath)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Promoter{cfg: cfg, root: root, rootPath: rootPath, inbox: proposalInbox, repository: repository, branch: branch}, nil
}

func (promoter *Promoter) Close() error {
	repositoryErr := promoter.repository.Close()
	rootErr := promoter.root.Close()
	if repositoryErr != nil {
		return repositoryErr
	}
	return rootErr
}

func (promoter *Promoter) List(ctx context.Context) (List, error) {
	snapshots, err := promoter.inbox.List(ctx)
	if err != nil {
		return List{}, err
	}
	result := List{Categories: append([]string(nil), promoter.cfg.MemoryCategories...), Proposals: make([]Summary, len(snapshots))}
	for index, snapshot := range snapshots {
		proposal := snapshot.Proposal
		result.Proposals[index] = Summary{ID: proposal.ID, Path: proposal.Path, Title: proposal.Title, Content: proposal.Content}
	}
	return result, nil
}

func (promoter *Promoter) RelatedProposals(ctx context.Context, expected Preview, query string) (RelatedProposals, error) {
	snapshots, err := promoter.inbox.List(ctx)
	if err != nil {
		return RelatedProposals{}, err
	}
	candidates := make([]RelatedProposal, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Head != expected.head {
			return RelatedProposals{}, ErrDrift
		}
		proposal := snapshot.Proposal
		if proposal.ID == expected.ProposalID {
			continue
		}
		score := retrieval.LexicalScore(query, proposal.Title+" "+proposal.Content)
		if score == 0 {
			continue
		}
		content := strings.TrimSuffix(proposal.Content, "\n")
		excerpt, truncated := utf8Excerpt(content, relatedKnowledgeBytes)
		candidates = append(candidates, RelatedProposal{
			ID: proposal.ID, Path: proposal.Path, Title: proposal.Title, Content: excerpt, ContentTruncated: truncated, Score: score,
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
			result.Truncated = result.Truncated || candidate.ContentTruncated
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

func (promoter *Promoter) RevalidatePreview(ctx context.Context, expected Preview) error {
	_, err := promoter.revalidatePreview(ctx, expected)
	return err
}

func (promoter *Promoter) revalidatePreview(ctx context.Context, expected Preview) (Preview, error) {
	fresh, err := promoter.preview(ctx, expected.proposal, expected.request)
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

// Preview previews a single decision. A fold_proposal only exists inside a batch,
// next to the rewrite_page or create_page with replacement it folds into, so it
// is rejected here.
func (promoter *Promoter) Preview(ctx context.Context, request PreviewRequest) (Preview, error) {
	if request.Operation == OperationFoldProposal {
		return Preview{}, ErrInvalidBatch
	}
	return promoter.previewOne(ctx, request)
}

func (promoter *Promoter) previewOne(ctx context.Context, request PreviewRequest) (Preview, error) {
	snapshot, err := promoter.inbox.Read(ctx, request.ProposalID)
	if err != nil {
		return Preview{}, err
	}
	return promoter.preview(ctx, snapshot, request)
}

// validFolds reports whether every fold_proposal in the batch folds into another
// item of the same batch that writes a complete page: a rewrite_page, or a
// create_page carrying a replacement.
func validFolds(requests []PreviewRequest) bool {
	targets := make(map[string]bool, len(requests))
	for _, request := range requests {
		targets[request.ProposalID] = request.Operation == OperationRewritePage || request.Operation == OperationCreatePage && request.Replacement != ""
	}
	for _, request := range requests {
		if request.Operation != OperationFoldProposal {
			continue
		}
		if request.FoldInto == request.ProposalID || !targets[request.FoldInto] {
			return false
		}
	}
	return true
}

func (promoter *Promoter) PreviewBatch(ctx context.Context, requests []PreviewRequest) (BatchPreview, error) {
	if len(requests) == 0 || len(requests) > MaxBatchItems {
		return BatchPreview{}, ErrInvalidBatch
	}
	batch := BatchPreview{Items: make([]Preview, 0, len(requests))}
	proposalIDs := make(map[string]struct{}, len(requests))
	targets := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		if _, exists := proposalIDs[request.ProposalID]; exists {
			return BatchPreview{}, ErrInvalidBatch
		}
		proposalIDs[request.ProposalID] = struct{}{}
		if request.TargetPath != "" {
			if _, exists := targets[request.TargetPath]; exists {
				return BatchPreview{}, ErrInvalidBatch
			}
			targets[request.TargetPath] = struct{}{}
		}
	}
	if !validFolds(requests) {
		return BatchPreview{}, ErrInvalidBatch
	}
	for _, request := range requests {
		preview, err := promoter.previewOne(ctx, request)
		if err != nil {
			return BatchPreview{}, err
		}
		if len(batch.Items) > 0 && preview.head != batch.Items[0].head {
			return BatchPreview{}, ErrDrift
		}
		batch.Items = append(batch.Items, preview)
	}
	diffs := make([]string, len(batch.Items))
	for index, preview := range batch.Items {
		diffs[index] = preview.Diff
	}
	batch.Diff = strings.Join(diffs, "\n")
	return batch, nil
}

func (promoter *Promoter) preview(ctx context.Context, snapshot inbox.Snapshot, request PreviewRequest) (Preview, error) {
	operation, targetPath, sectionID, heading := request.Operation, request.TargetPath, request.SectionID, request.Heading
	if err := promoter.confirmConfig(ctx, snapshot.Head); err != nil {
		return Preview{}, err
	}
	if operation != OperationDiscard && request.Reason != "" {
		return Preview{}, memory.ErrInvalidPath
	}
	if operation != OperationFoldProposal && request.FoldInto != "" {
		return Preview{}, memory.ErrInvalidPath
	}
	if operation != OperationDiscard && operation != OperationFoldProposal && !promoter.cfg.IsMemoryPath(targetPath) {
		return Preview{}, memory.ErrInvalidPath
	}
	if err := promoter.inbox.Confirm(ctx, snapshot); err != nil {
		return Preview{}, err
	}
	preview := Preview{
		Operation: operation, ProposalID: snapshot.Proposal.ID, ProposalPath: snapshot.Proposal.Path, Title: snapshot.Proposal.Title,
		Content:    snapshot.Proposal.Content,
		TargetPath: targetPath, ContentHash: request.ContentHash, Heading: heading, Replacement: request.Replacement, Reason: request.Reason, FoldInto: request.FoldInto, proposal: snapshot, request: request,
	}
	var targetDiff string
	switch operation {
	case OperationCreatePage:
		if sectionID != "" || request.ContentHash != "" || heading != "" || !memory.IsCreatePagePath(targetPath) {
			return Preview{}, memory.ErrInvalidPath
		}
		page, err := promoter.repository.PreviewCreatePage(ctx, memory.CreatePageRequest{
			TargetPath: targetPath, Title: snapshot.Proposal.Title, Body: snapshot.Proposal.Content, Replacement: request.Replacement,
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
		section, err := promoter.repository.PreviewCreateSection(ctx, memory.CreateSectionRequest{
			TargetPath: targetPath, Heading: heading, Body: snapshot.Proposal.Content,
		})
		if err != nil {
			return Preview{}, err
		}
		preview.SectionID = section.Target.SectionID
		preview.head = section.Head
		mode, blob, err := promoter.treeEntry(ctx, section.Head, targetPath)
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
		replacement, err := promoter.repository.PreviewReplaceSection(ctx, memory.PreviewRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, Replacement: request.Replacement,
		})
		if err != nil {
			return Preview{}, err
		}
		if replacement.SectionHash != request.ContentHash {
			return Preview{}, ErrDrift
		}
		mode, blob, err := promoter.treeEntry(ctx, replacement.Head, targetPath)
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
	case OperationRewritePage:
		if sectionID != "" || request.ContentHash == "" || heading != "" || request.Replacement == "" {
			return Preview{}, memory.ErrInvalidPath
		}
		rewrite, err := promoter.repository.PreviewRewritePage(ctx, memory.RewritePageRequest{
			TargetPath: targetPath, ContentHash: request.ContentHash, Replacement: request.Replacement,
		})
		if err != nil {
			return Preview{}, err
		}
		if rewrite.SectionHash != request.ContentHash {
			return Preview{}, ErrDrift
		}
		preview.SectionID = rewrite.Target.SectionID
		mode, blob, err := promoter.treeEntry(ctx, rewrite.Head, targetPath)
		if err != nil || blob != rewrite.Blob {
			return Preview{}, ErrDrift
		}
		preview.gitMode = mode
		preview.head = rewrite.Head
		preview.baselineHash = rewrite.BaselineHash
		preview.baselineBlob = rewrite.Blob
		preview.baseline = append([]byte(nil), rewrite.Baseline...)
		preview.postStateHash = rewrite.PostStateHash
		preview.postState = append([]byte(nil), rewrite.PostState...)
		targetDiff = rewrite.Diff
	case OperationRemoveSection:
		if sectionID == "" || request.ContentHash == "" || heading != "" || request.Replacement != "" {
			return Preview{}, memory.ErrInvalidPath
		}
		preview.SectionID = sectionID
		removal, err := promoter.repository.PreviewRemoveSection(ctx, memory.RemoveSectionRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, ContentHash: request.ContentHash,
		})
		if err != nil {
			return Preview{}, err
		}
		mode, blob, err := promoter.treeEntry(ctx, removal.Head, targetPath)
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
		removal, err := promoter.repository.PreviewRemovePage(ctx, memory.RemovePageRequest{
			Target: memory.Target{Path: targetPath, SectionID: sectionID}, ContentHash: request.ContentHash,
		})
		if err != nil {
			return Preview{}, err
		}
		mode, blob, err := promoter.treeEntry(ctx, removal.Head, targetPath)
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
	case OperationFoldProposal:
		if request.FoldInto == "" || targetPath != "" || sectionID != "" || request.ContentHash != "" || heading != "" || request.Replacement != "" || request.Reason != "" {
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

func validDiscardReason(reason DiscardReason) bool {
	switch reason {
	case DiscardNotRelevant, DiscardIncorrect, DiscardSensitive, DiscardWrongTarget, DiscardSuperseded, DiscardDuplicate:
		return true
	default:
		return false
	}
}

func (promoter *Promoter) ref() string { return "refs/heads/" + promoter.branch }

func (promoter *Promoter) confirmConfig(ctx context.Context, expectedHead string) error {
	categories, err := config.LoadRepositoryConfig(promoter.rootPath)
	if err != nil || !slices.Equal(categories, promoter.cfg.MemoryCategories) {
		return ErrDrift
	}
	for _, arguments := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "--cached", expectedHead, "--", "gpm.json"},
		{"diff", "--quiet", "--no-ext-diff", "--", "gpm.json"},
	} {
		if _, err := promoter.gitInput(ctx, nil, nil, arguments...); err != nil {
			return ErrDrift
		}
	}
	head, err := promoter.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != expectedHead {
		return ErrDrift
	}
	return nil
}

func (promoter *Promoter) Apply(ctx context.Context, expected Preview, validateEvidence EvidenceValidator) (Result, error) {
	result, err := promoter.ApplyBatch(ctx, BatchPreview{Items: []Preview{expected}, Diff: expected.Diff}, validateEvidence)
	if err != nil {
		return Result{}, err
	}
	return result.Items[0], nil
}

func (promoter *Promoter) ApplyBatch(ctx context.Context, expected BatchPreview, validateEvidence EvidenceValidator) (BatchResult, error) {
	var result BatchResult
	err := repolock.With(ctx, promoter.cfg.StateRoot, func() error {
		recovered, marker, err := promoter.recoverLocked(ctx)
		if err != nil {
			return err
		}
		if recovered.Commit != "" {
			if marker.PreviewHash != batchPreviewHash(expected.Items) {
				return ErrDrift
			}
			result = recovered
			return nil
		}
		existing, found, err := promoter.recognizeCommittedBatch(ctx, expected)
		if err != nil {
			return err
		}
		if found {
			result = existing
			return nil
		}
		fresh, err := promoter.revalidateBatch(ctx, expected)
		if err != nil {
			return err
		}
		result, err = promoter.commitBatch(ctx, fresh, validateEvidence)
		return err
	})
	return result, err
}

func (promoter *Promoter) revalidateBatch(ctx context.Context, expected BatchPreview) (BatchPreview, error) {
	if len(expected.Items) == 0 || len(expected.Items) > MaxBatchItems {
		return BatchPreview{}, ErrInvalidBatch
	}
	requests := make([]PreviewRequest, len(expected.Items))
	for index, preview := range expected.Items {
		requests[index] = preview.request
	}
	fresh, err := promoter.PreviewBatch(ctx, requests)
	if err != nil {
		return BatchPreview{}, err
	}
	for index := range fresh.Items {
		if !samePreview(fresh.Items[index], expected.Items[index]) {
			return BatchPreview{}, ErrDrift
		}
	}
	if fresh.Diff != expected.Diff {
		return BatchPreview{}, ErrDrift
	}
	return fresh, nil
}

func samePreview(left, right Preview) bool {
	return left.Operation == right.Operation && left.ProposalID == right.ProposalID && left.ProposalPath == right.ProposalPath && left.Title == right.Title && left.Content == right.Content && left.TargetPath == right.TargetPath && left.SectionID == right.SectionID && left.ContentHash == right.ContentHash && left.Replacement == right.Replacement && left.Reason == right.Reason && left.FoldInto == right.FoldInto && left.Diff == right.Diff &&
		left.Heading == right.Heading && left.head == right.head && left.gitMode == right.gitMode && left.fileMode == right.fileMode && left.baselineHash == right.baselineHash && left.baselineBlob == right.baselineBlob && bytes.Equal(left.baseline, right.baseline) &&
		left.postStateHash == right.postStateHash && bytes.Equal(left.postState, right.postState) &&
		left.proposal.Head == right.proposal.Head && left.proposal.Blob == right.proposal.Blob && bytes.Equal(left.proposal.Contents, right.proposal.Contents) && left.request == right.request
}

func (promoter *Promoter) treeEntry(ctx context.Context, revision, relativePath string) (string, string, error) {
	entry, err := promoter.gitInput(ctx, nil, nil, "ls-tree", "-z", revision, "--", relativePath)
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

func (promoter *Promoter) removeExact(relativePath string, expected []byte) error {
	fd, err := unix.Openat2(int(promoter.root.Fd()), relativePath, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_NONBLOCK | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return ErrDrift
	}
	file := os.NewFile(uintptr(fd), relativePath)
	if file == nil {
		_ = unix.Close(fd)
		return ErrDrift
	}
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil || opened.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = file.Close()
		return ErrDrift
	}
	contents, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(contents, expected) {
		return ErrDrift
	}
	var current unix.Stat_t
	if err := unix.Fstatat(int(promoter.root.Fd()), relativePath, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil || current.Dev != opened.Dev || current.Ino != opened.Ino || current.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrDrift
	}
	if err := unix.Unlinkat(int(promoter.root.Fd()), relativePath, 0); err != nil {
		return ErrDrift
	}
	return syncDirectory(filepath.Join(promoter.rootPath, filepath.Dir(relativePath)))
}

func (promoter *Promoter) gitPath(ctx context.Context, arguments ...string) (string, error) {
	value, err := promoter.gitText(ctx, nil, arguments...)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(promoter.rootPath, value)
	}
	return filepath.Clean(value), nil
}

func (promoter *Promoter) gitText(ctx context.Context, input []byte, arguments ...string) (string, error) {
	output, err := promoter.gitInput(ctx, nil, input, arguments...)
	return strings.TrimSpace(string(output)), err
}

func (promoter *Promoter) gitInput(ctx context.Context, extraEnvironment map[string]string, input []byte, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"--no-optional-locks", "--no-replace-objects", "-C", "/proc/self/fd/3"}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	command.Env = gitEnvironment(extraEnvironment)
	command.ExtraFiles = []*os.File{promoter.root}
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
	file, err := os.CreateTemp(directory, ".git-push-memories-promotion-index-*.tmp")
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

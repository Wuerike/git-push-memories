package proposal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
)

type Workflow struct {
	repository *memory.Repository
	store      *store
	memoryRoot string
	now        func() time.Time
}

func New(memoryRoot, stateRoot string) (*Workflow, error) {
	return openWorkflow(memoryRoot, stateRoot, true)
}

func OpenReadOnly(memoryRoot, stateRoot string) (*Workflow, error) {
	return openWorkflow(memoryRoot, stateRoot, false)
}

func openWorkflow(memoryRoot, stateRoot string, cleanup bool) (*Workflow, error) {
	if stateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	memoryAbsolute, err := filepath.EvalSymlinks(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	stateAbsolute, err := canonicalFuturePath(stateRoot)
	if err != nil || pathsOverlap(stateAbsolute, memoryAbsolute) {
		return nil, fmt.Errorf("state root must be outside the memory repository")
	}
	var repository *memory.Repository
	if cleanup {
		repository, err = memory.New(memoryRoot)
	} else {
		repository, err = memory.NewReadOnly(memoryRoot)
	}
	if err != nil {
		return nil, err
	}
	store, err := newStore(memoryAbsolute, stateAbsolute, cleanup)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	workflow := &Workflow{repository: repository, store: store, memoryRoot: memoryAbsolute, now: time.Now}
	if cleanup {
		if err := workflow.reconcileStartup(context.Background()); err != nil {
			_ = repository.Close()
			_ = store.close()
			return nil, err
		}
	}
	return workflow, nil
}

func canonicalFuturePath(requested string) (string, error) {
	absolute, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	existing := absolute
	var suffix []string
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
	canonical, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		canonical = filepath.Join(canonical, suffix[i])
	}
	return canonical, nil
}

func pathsOverlap(left, right string) bool {
	return within(left, right) || within(right, left)
}

func within(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

func (workflow *Workflow) Close() error {
	repositoryErr := workflow.repository.Close()
	storeErr := workflow.store.close()
	if repositoryErr != nil {
		return repositoryErr
	}
	return storeErr
}

func (workflow *Workflow) Prepare(ctx context.Context, candidate intake.AllowedCandidate) (View, error) {
	if candidate.Operation() == intake.OperationCreatePage {
		return workflow.prepareCreatePage(ctx, candidate)
	}
	if candidate.Operation() == intake.OperationCreateSection {
		return workflow.prepareCreateSection(ctx, candidate)
	}
	preview, err := workflow.repository.Preview(ctx, memory.PreviewRequest{
		Target:      memory.Target{Path: candidate.TargetPath(), SectionID: candidate.SectionID()},
		Replacement: candidate.Replacement(),
	})
	if err != nil {
		return View{}, err
	}
	nonce, err := proposalNonce()
	if err != nil {
		return View{}, fmt.Errorf("create proposal nonce")
	}
	view := View{
		SchemaVersion: 2, Nonce: nonce, Revision: 1, Status: "pending", CreatedAt: workflow.now().UTC(),
		Operation:   Operation{Type: OperationReplaceSection, Target: Target{Path: preview.Target.Path, SectionID: preview.Target.SectionID}, Replacement: candidate.Replacement()},
		Baseline:    Baseline{Exists: boolPointer(true), FileHash: preview.BaselineHash, Head: preview.Head, Blob: preview.Blob},
		PostState:   PostState{FileHash: preview.PostStateHash},
		Diff:        unifiedDiff(preview.Target.Path, preview.Baseline, preview.PostState),
		SourcePaths: candidate.SourcePaths(),
	}
	if err := sealProposal(&view); err != nil {
		return View{}, fmt.Errorf("seal proposal")
	}
	if err := workflow.store.writeProposal(view); err != nil {
		return View{}, err
	}
	return view, nil
}

func (workflow *Workflow) prepareCreateSection(ctx context.Context, candidate intake.AllowedCandidate) (View, error) {
	preview, err := workflow.repository.PreviewCreateSection(ctx, memory.CreateSectionRequest{
		TargetPath: candidate.TargetPath(), Heading: candidate.Heading(), Body: candidate.Body(),
	})
	if err != nil {
		return View{}, err
	}
	nonce, err := proposalNonce()
	if err != nil {
		return View{}, fmt.Errorf("create proposal nonce")
	}
	view := View{
		SchemaVersion: 2, Nonce: nonce, Revision: 1, Status: "pending", CreatedAt: workflow.now().UTC(),
		Operation: Operation{Type: OperationCreateSection, Target: Target{Path: preview.Target.Path, SectionID: preview.Target.SectionID}, Heading: candidate.Heading(), Body: candidate.Body()},
		Baseline:  Baseline{Exists: boolPointer(true), FileHash: preview.BaselineHash, Head: preview.Head, Blob: preview.Blob}, PostState: PostState{FileHash: preview.PostStateHash},
		Diff: preview.Diff, SourcePaths: candidate.SourcePaths(),
	}
	if err := sealProposal(&view); err != nil {
		return View{}, fmt.Errorf("seal proposal")
	}
	if err := workflow.store.writeProposal(view); err != nil {
		return View{}, err
	}
	return view, nil
}

func (workflow *Workflow) prepareCreatePage(ctx context.Context, candidate intake.AllowedCandidate) (View, error) {
	preview, err := workflow.repository.PreviewCreatePage(ctx, memory.CreatePageRequest{
		TargetPath: candidate.TargetPath(), Title: candidate.Title(), Body: candidate.Body(),
	})
	if err != nil {
		return View{}, err
	}
	nonce, err := proposalNonce()
	if err != nil {
		return View{}, fmt.Errorf("create proposal nonce")
	}
	absent := false
	view := View{
		SchemaVersion: 2, Nonce: nonce, Revision: 1, Status: "pending", CreatedAt: workflow.now().UTC(),
		Operation: Operation{Type: OperationCreatePage, Target: Target{Path: preview.Target.Path, SectionID: preview.Target.SectionID}, Title: candidate.Title(), Body: candidate.Body()},
		Baseline:  Baseline{Exists: &absent, Head: preview.Head}, PostState: PostState{FileHash: preview.PostStateHash},
		Diff: preview.Diff, SourcePaths: candidate.SourcePaths(),
	}
	if err := sealProposal(&view); err != nil {
		return View{}, fmt.Errorf("seal proposal")
	}
	if err := workflow.store.writeProposal(view); err != nil {
		return View{}, err
	}
	return view, nil
}

func (workflow *Workflow) Review(id string) (Review, error) {
	published, err := workflow.store.readPublication(id)
	if err == nil {
		applied, applyErr := workflow.store.readApply(id)
		if applyErr != nil || !publicationMatchesApply(published, applied) || published.MemoryRoot != workflow.memoryRoot {
			return Review{}, ErrIntegrity
		}
		return Review{Published: &published}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Review{}, err
	}
	applied, err := workflow.store.readApply(id)
	if err == nil {
		if applied.MemoryRoot != workflow.memoryRoot {
			return Review{}, ErrIntegrity
		}
		return Review{Applied: &applied}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Review{}, err
	}
	decision, err := workflow.store.readDecision(id)
	if err == nil {
		return Review{Decision: &decision}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Review{}, err
	}
	view, err := workflow.store.readProposal(id)
	if err != nil {
		return Review{}, err
	}
	return Review{Proposal: &view}, nil
}

func (workflow *Workflow) RecordPublication(ctx context.Context, receipt PublicationReceipt) (PublicationReceipt, error) {
	var stored PublicationReceipt
	err := workflow.store.withProposalLock(ctx, receipt.ProposalID, func() error {
		existing, err := workflow.store.readPublication(receipt.ProposalID)
		if err == nil {
			if existing != receipt {
				return ErrIntegrity
			}
			if err := workflow.store.confirmDurable("publications"); err != nil {
				return err
			}
			stored = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		applied, err := workflow.Applied(receipt.ProposalID)
		if err != nil || !publicationMatchesApply(receipt, applied) {
			return ErrIntegrity
		}
		if err := workflow.store.writePublication(receipt); err != nil {
			return err
		}
		stored = receipt
		return nil
	})
	return stored, err
}

func (workflow *Workflow) Published(id string) (PublicationReceipt, error) {
	receipt, err := workflow.store.readPublication(id)
	if err != nil {
		return PublicationReceipt{}, err
	}
	applied, err := workflow.store.readApply(id)
	if err != nil || !publicationMatchesApply(receipt, applied) || receipt.MemoryRoot != workflow.memoryRoot {
		return PublicationReceipt{}, ErrIntegrity
	}
	if err := workflow.store.confirmDurable("publications"); err != nil {
		return PublicationReceipt{}, err
	}
	return receipt, nil
}

func publicationMatchesApply(publication PublicationReceipt, apply ApplyReceipt) bool {
	return publication.SchemaVersion == 1 && publication.ProposalID == apply.ProposalID && publication.Revision == apply.Revision && publication.Status == "published" && !publication.PublishedAt.IsZero() &&
		publication.MemoryRoot == apply.MemoryRoot && publication.Target == apply.Target && publication.Parent == apply.Baseline.Head && publication.Branch != "" && publication.Commit != "" && publication.Blob != ""
}

var rejectionReasons = map[string]struct{}{
	"not_relevant": {}, "incorrect": {}, "sensitive": {}, "wrong_target": {}, "user_cancelled": {},
}

func (workflow *Workflow) Reject(id, reason string) (DecisionReceipt, error) {
	if _, valid := rejectionReasons[reason]; !valid {
		return DecisionReceipt{}, ErrInvalidReason
	}
	var receipt DecisionReceipt
	err := workflow.store.withProposalLock(context.Background(), id, func() error {
		var err error
		receipt, err = workflow.rejectLocked(id, reason)
		return err
	})
	return receipt, err
}

func (workflow *Workflow) rejectLocked(id, reason string) (DecisionReceipt, error) {
	if _, err := workflow.store.readApply(id); err == nil {
		return DecisionReceipt{}, ErrTerminal
	} else if !errors.Is(err, ErrNotFound) {
		return DecisionReceipt{}, err
	}
	decision, err := workflow.store.readDecision(id)
	if err == nil {
		if err := workflow.store.confirmDurable("decisions"); err != nil {
			return DecisionReceipt{}, err
		}
		if err := workflow.store.finishRejection(id); err != nil {
			return DecisionReceipt{}, err
		}
		return decision, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return DecisionReceipt{}, err
	}
	if _, err := workflow.applyLocked(context.Background(), id, 1, false); err == nil {
		return DecisionReceipt{}, ErrTerminal
	} else if !errors.Is(err, ErrNotApplied) {
		return DecisionReceipt{}, err
	}
	view, err := workflow.store.readProposal(id)
	if err != nil {
		return DecisionReceipt{}, err
	}
	receipt := DecisionReceipt{
		SchemaVersion: view.SchemaVersion, ProposalID: view.ID, Revision: view.Revision,
		Status: "rejected", Reason: reason, DecidedAt: workflow.now().UTC(),
	}
	if err := workflow.store.reject(receipt); err != nil {
		return DecisionReceipt{}, err
	}
	return receipt, nil
}

func (workflow *Workflow) Apply(ctx context.Context, id string, revision int) (ApplyReceipt, error) {
	var receipt ApplyReceipt
	err := workflow.store.withProposalLock(ctx, id, func() error {
		var err error
		receipt, err = workflow.applyLocked(ctx, id, revision, true)
		return err
	})
	return receipt, err
}

func (workflow *Workflow) applyLocked(ctx context.Context, id string, revision int, write bool) (ApplyReceipt, error) {
	receipt, err := workflow.store.readApply(id)
	if err == nil {
		if receipt.Revision != revision {
			return ApplyReceipt{}, ErrWrongRevision
		}
		if receipt.MemoryRoot != workflow.memoryRoot {
			return ApplyReceipt{}, ErrIntegrity
		}
		if err := workflow.store.confirmDurable("applies"); err != nil {
			return ApplyReceipt{}, err
		}
		if err := workflow.store.finishApply(id); err != nil {
			return ApplyReceipt{}, err
		}
		return receipt, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ApplyReceipt{}, err
	}
	if decision, err := workflow.store.readDecision(id); err == nil {
		if decision.Revision != revision {
			return ApplyReceipt{}, ErrWrongRevision
		}
		return ApplyReceipt{}, ErrTerminal
	} else if !errors.Is(err, ErrNotFound) {
		return ApplyReceipt{}, err
	}
	view, err := workflow.store.readProposal(id)
	if err != nil {
		return ApplyReceipt{}, err
	}
	if view.Revision != revision {
		return ApplyReceipt{}, ErrWrongRevision
	}
	mode := memory.ReconcileOnly
	if write {
		mode = memory.ApplyBaseline
	}
	result, err := workflow.reconcileView(ctx, view, mode)
	if err != nil {
		return ApplyReceipt{}, err
	}
	if result.Target.Path != view.Operation.Target.Path || result.Target.SectionID != view.Operation.Target.SectionID {
		return ApplyReceipt{}, ErrIntegrity
	}
	if result.State == memory.ReconcileBaseline {
		return ApplyReceipt{}, ErrNotApplied
	}
	receipt = ApplyReceipt{
		SchemaVersion: view.SchemaVersion, ProposalID: view.ID, Revision: view.Revision, Status: "applied_unpublished", AppliedAt: workflow.now().UTC(),
		MemoryRoot: workflow.memoryRoot, Target: view.Operation.Target, Baseline: view.Baseline, PostState: view.PostState,
	}
	if err := workflow.store.writeApply(receipt); err != nil {
		return ApplyReceipt{}, err
	}
	if err := workflow.store.finishApply(id); err != nil {
		return ApplyReceipt{}, err
	}
	return receipt, nil
}

func boolPointer(value bool) *bool { return &value }

func (workflow *Workflow) reconcileView(ctx context.Context, view View, mode memory.ReconcileMode) (memory.ReconcileResult, error) {
	preflight := func() error {
		allowed, err := inspectStoredCandidate(view)
		if err != nil || allowed.Operation() != view.Operation.Type || allowed.TargetPath() != view.Operation.Target.Path {
			return ErrIntegrity
		}
		if view.Operation.Type == OperationCreatePage {
			expectedSectionID := memory.PageID(view.Operation.Target.Path) + "-overview"
			if view.Operation.Target.SectionID != expectedSectionID || allowed.Title() != view.Operation.Title || allowed.Body() != view.Operation.Body {
				return ErrIntegrity
			}
			return nil
		}
		if view.Operation.Type == OperationCreateSection {
			if allowed.Heading() != view.Operation.Heading || allowed.Body() != view.Operation.Body {
				return ErrIntegrity
			}
			return nil
		}
		if allowed.SectionID() != view.Operation.Target.SectionID || allowed.Replacement() != view.Operation.Replacement {
			return ErrIntegrity
		}
		return nil
	}
	if view.Operation.Type == OperationCreatePage {
		return workflow.repository.ReconcileCreatePage(ctx, memory.ReconcileCreatePageRequest{
			CreatePageRequest: memory.CreatePageRequest{TargetPath: view.Operation.Target.Path, Title: view.Operation.Title, Body: view.Operation.Body},
			PostStateHash:     view.PostState.FileHash, Head: view.Baseline.Head, Preflight: preflight,
			Validate: func(postState []byte) error {
				if createPageDiff(view.Operation.Target.Path, postState) != view.Diff {
					return ErrIntegrity
				}
				return nil
			}, Mode: mode,
		})
	}
	if view.Operation.Type == OperationCreateSection {
		return workflow.repository.ReconcileCreateSection(ctx, memory.ReconcileCreateSectionRequest{
			CreateSectionRequest: memory.CreateSectionRequest{TargetPath: view.Operation.Target.Path, Heading: view.Operation.Heading, Body: view.Operation.Body},
			ExpectedSectionID:    view.Operation.Target.SectionID, BaselineHash: view.Baseline.FileHash, PostStateHash: view.PostState.FileHash, Head: view.Baseline.Head, Blob: view.Baseline.Blob,
			Preflight: preflight,
			Validate: func(baseline, postState []byte) error {
				if unifiedDiff(view.Operation.Target.Path, baseline, postState) != view.Diff {
					return ErrIntegrity
				}
				return nil
			}, Mode: mode,
		})
	}
	return workflow.repository.ReconcileApply(ctx, memory.ReconcileRequest{
		Target: memory.Target{Path: view.Operation.Target.Path, SectionID: view.Operation.Target.SectionID}, Replacement: view.Operation.Replacement,
		BaselineHash: view.Baseline.FileHash, PostStateHash: view.PostState.FileHash, Head: view.Baseline.Head, Blob: view.Baseline.Blob,
		Preflight: preflight,
		Validate: func(baseline, postState []byte) error {
			if unifiedDiff(view.Operation.Target.Path, baseline, postState) != view.Diff {
				return ErrIntegrity
			}
			return nil
		}, Mode: mode,
	})
}

func (workflow *Workflow) reconcileStartup(ctx context.Context) error {
	ids, err := workflow.store.proposalIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := workflow.store.withProposalLock(ctx, id, func() error {
			if _, err := workflow.store.readDecision(id); err == nil {
				if err := workflow.store.confirmDurable("decisions"); err != nil {
					return err
				}
				return workflow.store.finishRejection(id)
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			if receipt, err := workflow.store.readApply(id); err == nil {
				if receipt.MemoryRoot != workflow.memoryRoot {
					return ErrIntegrity
				}
				if err := workflow.store.confirmDurable("applies"); err != nil {
					return err
				}
				return workflow.store.finishApply(id)
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			_, err := workflow.applyLocked(ctx, id, 1, false)
			if errors.Is(err, ErrNotApplied) || errors.Is(err, memory.ErrDrift) {
				return nil
			}
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func inspectStoredCandidate(view View) (intake.AllowedCandidate, error) {
	var candidate any
	if view.Operation.Type == OperationCreatePage {
		candidate = struct {
			Operation   string   `json:"operation"`
			TargetPath  string   `json:"target_path"`
			Title       string   `json:"title"`
			Body        string   `json:"body"`
			SourcePaths []string `json:"source_paths"`
		}{view.Operation.Type, view.Operation.Target.Path, view.Operation.Title, view.Operation.Body, view.SourcePaths}
	} else if view.Operation.Type == OperationCreateSection {
		candidate = struct {
			Operation   string   `json:"operation"`
			TargetPath  string   `json:"target_path"`
			Heading     string   `json:"heading"`
			Body        string   `json:"body"`
			SourcePaths []string `json:"source_paths"`
		}{view.Operation.Type, view.Operation.Target.Path, view.Operation.Heading, view.Operation.Body, view.SourcePaths}
	} else {
		candidate = struct {
			TargetPath  string   `json:"target_path"`
			SectionID   string   `json:"section_id"`
			Replacement string   `json:"replacement"`
			SourcePaths []string `json:"source_paths"`
		}{view.Operation.Target.Path, view.Operation.Target.SectionID, view.Operation.Replacement, view.SourcePaths}
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return intake.AllowedCandidate{}, err
	}
	guard, err := intake.New()
	if err != nil {
		return intake.AllowedCandidate{}, err
	}
	return guard.Inspect(intake.NewCandidate(raw))
}

func (workflow *Workflow) Applied(id string) (ApplyReceipt, error) {
	if receipt, err := workflow.store.readApply(id); err == nil {
		if receipt.MemoryRoot != workflow.memoryRoot {
			return ApplyReceipt{}, ErrIntegrity
		}
		if err := workflow.store.confirmDurable("applies"); err != nil {
			return ApplyReceipt{}, err
		}
		return receipt, nil
	} else if !errors.Is(err, ErrNotFound) {
		return ApplyReceipt{}, err
	}
	if _, err := workflow.store.readProposal(id); err == nil {
		return ApplyReceipt{}, ErrNotApplied
	} else if !errors.Is(err, ErrNotFound) {
		return ApplyReceipt{}, err
	}
	if _, err := workflow.store.readDecision(id); err == nil {
		return ApplyReceipt{}, ErrNotApplied
	} else if !errors.Is(err, ErrNotFound) {
		return ApplyReceipt{}, err
	}
	return ApplyReceipt{}, ErrNotFound
}

func proposalNonce() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

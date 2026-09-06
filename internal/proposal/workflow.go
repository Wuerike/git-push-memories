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

func New(memoryRoot, codeRoot, stateRoot string) (*Workflow, error) {
	return openWorkflow(memoryRoot, codeRoot, stateRoot, true)
}

func OpenReadOnly(memoryRoot, codeRoot, stateRoot string) (*Workflow, error) {
	return openWorkflow(memoryRoot, codeRoot, stateRoot, false)
}

func openWorkflow(memoryRoot, codeRoot, stateRoot string, cleanup bool) (*Workflow, error) {
	if stateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	memoryAbsolute, err := filepath.EvalSymlinks(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid memory root")
	}
	codeAbsolute, codeErr := filepath.EvalSymlinks(codeRoot)
	stateAbsolute, err := canonicalFuturePath(stateRoot)
	if err != nil || codeErr != nil || pathsOverlap(stateAbsolute, memoryAbsolute) || pathsOverlap(stateAbsolute, codeAbsolute) {
		return nil, fmt.Errorf("state root must be outside configured repositories")
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
	store, err := newStore(stateAbsolute, cleanup)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	workflow := &Workflow{repository: repository, store: store, memoryRoot: memoryAbsolute, now: time.Now}
	if cleanup {
		if err := workflow.reconcileStartup(context.Background()); err != nil {
			_ = repository.Close()
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
	return workflow.repository.Close()
}

func (workflow *Workflow) Prepare(ctx context.Context, candidate intake.AllowedCandidate) (View, error) {
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
		SchemaVersion: 1, Nonce: nonce, Revision: 1, Status: "pending", CreatedAt: workflow.now().UTC(),
		Operation:   Operation{Type: "replace_section_content", Target: Target{Path: preview.Target.Path, SectionID: preview.Target.SectionID}, Replacement: candidate.Replacement()},
		Baseline:    Baseline{FileHash: preview.BaselineHash, Head: preview.Head, Blob: preview.Blob},
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
		SchemaVersion: 1, ProposalID: view.ID, Revision: view.Revision,
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
	result, err := workflow.repository.ReconcileApply(ctx, memory.ReconcileRequest{
		Target: memory.Target{Path: view.Operation.Target.Path, SectionID: view.Operation.Target.SectionID}, Replacement: view.Operation.Replacement,
		BaselineHash: view.Baseline.FileHash, PostStateHash: view.PostState.FileHash, Head: view.Baseline.Head, Blob: view.Baseline.Blob,
		Preflight: func() error {
			allowed, err := inspectStoredCandidate(view)
			if err != nil || allowed.TargetPath() != view.Operation.Target.Path || allowed.SectionID() != view.Operation.Target.SectionID || allowed.Replacement() != view.Operation.Replacement {
				return ErrIntegrity
			}
			return nil
		},
		Validate: func(baseline, postState []byte) error {
			if unifiedDiff(view.Operation.Target.Path, baseline, postState) != view.Diff {
				return ErrIntegrity
			}
			return nil
		},
		Mode: mode,
	})
	if err != nil {
		return ApplyReceipt{}, err
	}
	if result.Target.Path != view.Operation.Target.Path {
		return ApplyReceipt{}, ErrIntegrity
	}
	if result.State == memory.ReconcileBaseline {
		return ApplyReceipt{}, ErrNotApplied
	}
	receipt = ApplyReceipt{
		SchemaVersion: 1, ProposalID: view.ID, Revision: view.Revision, Status: "applied_unpublished", AppliedAt: workflow.now().UTC(),
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
	raw, err := json.Marshal(struct {
		TargetPath  string   `json:"target_path"`
		SectionID   string   `json:"section_id"`
		Replacement string   `json:"replacement"`
		SourcePaths []string `json:"source_paths"`
	}{view.Operation.Target.Path, view.Operation.Target.SectionID, view.Operation.Replacement, view.SourcePaths})
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

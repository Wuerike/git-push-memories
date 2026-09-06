package proposal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	repository, err := memory.NewReadOnly(memoryRoot)
	if err != nil {
		return nil, err
	}
	store, err := newStore(stateAbsolute, cleanup)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &Workflow{repository: repository, store: store, now: time.Now}, nil
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
	id, err := proposalID()
	if err != nil {
		return View{}, fmt.Errorf("create proposal ID")
	}
	view := View{
		SchemaVersion: 1, ID: id, Revision: 1, Status: "pending", CreatedAt: workflow.now().UTC(),
		Operation:   Operation{Type: "replace_section_content", Target: Target{Path: preview.Target.Path, SectionID: preview.Target.SectionID}, Replacement: candidate.Replacement()},
		Baseline:    Baseline{FileHash: preview.BaselineHash, Head: preview.Head, Blob: preview.Blob},
		PostState:   PostState{FileHash: preview.PostStateHash},
		Diff:        unifiedDiff(preview.Target.Path, preview.Baseline, preview.PostState),
		SourcePaths: candidate.SourcePaths(),
	}
	if err := workflow.store.writeProposal(view); err != nil {
		return View{}, err
	}
	return view, nil
}

func (workflow *Workflow) Review(id string) (Review, error) {
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

var rejectionReasons = map[string]struct{}{
	"not_relevant": {}, "incorrect": {}, "sensitive": {}, "wrong_target": {}, "user_cancelled": {},
}

func (workflow *Workflow) Reject(id, reason string) (DecisionReceipt, error) {
	if _, valid := rejectionReasons[reason]; !valid {
		return DecisionReceipt{}, ErrInvalidReason
	}
	decision, err := workflow.store.readDecision(id)
	if err == nil {
		if err := workflow.store.finishRejection(id); err != nil {
			return DecisionReceipt{}, err
		}
		return decision, nil
	}
	if !errors.Is(err, ErrNotFound) {
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
		decision, readErr := workflow.store.readDecision(id)
		if readErr != nil {
			return DecisionReceipt{}, err
		}
		if cleanupErr := workflow.store.finishRejection(id); cleanupErr != nil {
			return DecisionReceipt{}, cleanupErr
		}
		return decision, nil
	}
	return receipt, nil
}

func (workflow *Workflow) Applied(id string) (ApplyReceipt, error) {
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

func proposalID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// Package setup prepares the managed clone and worktrees for a memory repository URL.
// Every step derives what is missing from the current state, so it is safe to repeat.
package setup

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

type Options struct {
	URL string
	// Categories is asked only when the remote is empty and gpm.json must be created.
	Categories func() ([]string, error)
}

type Result struct {
	Created bool
	Config  config.Config
}

var errProtected = errors.New("the initial push was refused; if main is protected, enable the protection only after setup")

func Prepare(ctx context.Context, options Options) (Result, error) {
	if options.URL == "" {
		return Result{}, errors.New("repository URL is required")
	}
	if err := workspace.CheckIdentity(ctx); err != nil {
		return Result{}, err
	}
	stored, err := config.StoredURL()
	if err != nil {
		return Result{}, err
	}
	if stored != "" && stored != options.URL {
		return Result{}, fmt.Errorf("already configured for %s; remove the state directory to switch repositories", stored)
	}
	w, err := workspace.Open(options.URL)
	if err != nil {
		return Result{}, err
	}
	if err := w.Clone(ctx); err != nil {
		return Result{}, fmt.Errorf("repository could not be cloned: %w", err)
	}
	if err := w.Fetch(ctx); err != nil {
		return Result{}, err
	}

	created := false
	switch {
	case w.RefExists(ctx, "refs/remotes/origin/main"):
		contents, err := w.ShowFile(ctx, "refs/remotes/origin/main", "gpm.json")
		if err != nil {
			return Result{}, errors.New("the repository has content but main has no gpm.json")
		}
		if _, err := config.ParseRepositoryConfig(contents); err != nil {
			return Result{}, err
		}
	case w.HasRemoteRefs(ctx):
		return Result{}, errors.New("the repository has content but main has no gpm.json")
	default:
		if options.Categories == nil {
			return Result{}, errors.New("memory categories are required to initialize an empty repository")
		}
		categories, err := options.Categories()
		if err != nil {
			return Result{}, err
		}
		contents, err := config.RepositoryConfigJSON(categories)
		if err != nil {
			return Result{}, err
		}
		if err := w.CreateMain(ctx, map[string][]byte{"gpm.json": contents}); err != nil {
			if errors.Is(err, workspace.ErrPermission) {
				return Result{}, errProtected
			}
			return Result{}, err
		}
		created = true
	}

	if err := ensureProposals(ctx, w); err != nil {
		return Result{}, err
	}
	if err := w.EnsureRead(ctx); err != nil {
		return Result{}, err
	}
	cfg, err := config.Resolve(options.URL)
	if err != nil {
		return Result{}, err
	}
	return Result{Created: created, Config: cfg}, nil
}

func ensureProposals(ctx context.Context, w *workspace.Workspace) error {
	start, remote := "origin/main", w.RefExists(ctx, "refs/remotes/origin/proposals")
	if remote {
		start = "origin/proposals"
	}
	if err := w.EnsureWorktree(ctx, w.Paths.Proposals, workspace.ProposalsBranch, start); err != nil {
		return err
	}
	if remote {
		return nil
	}
	if err := w.Push(ctx, w.Paths.Proposals, workspace.ProposalsBranch); err != nil {
		if errors.Is(err, workspace.ErrPermission) {
			return errProtected
		}
		return err
	}
	return nil
}

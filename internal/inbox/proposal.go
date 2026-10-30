package inbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var proposalPathPattern = regexp.MustCompile(`^proposals/([0-9a-f]{32})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)

type Snapshot struct {
	Proposal Proposal
	Head     string
	Blob     string
	Contents []byte
}

// List returns the committed proposals; files it cannot read or whose name is not a proposal are skipped.
func (inbox *Inbox) List(ctx context.Context) ([]Snapshot, error) {
	if err := inbox.checkBranch(ctx); err != nil {
		return nil, err
	}
	git := inbox.git()
	head, err := git.Output(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read inbox HEAD")
	}
	entries, err := git.Bytes(ctx, "ls-tree", "-r", "-z", "--full-tree", head, "--", "proposals")
	if err != nil {
		return nil, fmt.Errorf("list proposals")
	}
	var snapshots []Snapshot
	for _, encoded := range bytes.Split(entries, []byte{0}) {
		meta, name, ok := strings.Cut(string(encoded), "\t")
		fields := strings.Fields(meta)
		matches := proposalPathPattern.FindStringSubmatch(name)
		if !ok || len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" || matches == nil {
			continue
		}
		contents, err := git.Bytes(ctx, "cat-file", "blob", fields[2])
		if err != nil {
			continue
		}
		snapshots = append(snapshots, Snapshot{
			Proposal: Proposal{ID: matches[1], Path: name, Title: title(matches[2], contents), Content: string(contents)},
			Head:     head, Blob: fields[2], Contents: contents,
		})
	}
	return snapshots, nil
}

// title is the first "# " heading of the text, or the file-name slug.
func title(slug string, contents []byte) string {
	for _, line := range strings.Split(string(contents), "\n") {
		if heading, ok := strings.CutPrefix(line, "# "); ok && strings.TrimSpace(heading) != "" {
			return strings.TrimSpace(heading)
		}
	}
	return slug
}

func (inbox *Inbox) Read(ctx context.Context, id string) (Snapshot, error) {
	if raw, err := hex.DecodeString(id); err != nil || len(raw) != 16 || strings.ToLower(id) != id {
		return Snapshot{}, ErrInvalidProposal
	}
	snapshots, err := inbox.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Proposal.ID == id {
			return snapshot, nil
		}
	}
	return Snapshot{}, os.ErrNotExist
}

// Confirm checks the proposal is still exactly what was listed: same HEAD, clean index and worktree, same bytes.
func (inbox *Inbox) Confirm(ctx context.Context, expected Snapshot) error {
	if err := inbox.checkBranch(ctx); err != nil {
		return err
	}
	git := inbox.git()
	if head, err := git.Output(ctx, "rev-parse", "--verify", "HEAD"); err != nil || head != expected.Head {
		return ErrProposalDrift
	}
	for _, arguments := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "--cached", expected.Head, "--", expected.Proposal.Path},
		{"diff", "--quiet", "--no-ext-diff", "--", expected.Proposal.Path},
	} {
		if !git.Ok(ctx, arguments...) {
			return ErrProposalDrift
		}
	}
	contents, err := os.ReadFile(filepath.Join(inbox.rootPath, expected.Proposal.Path))
	if err != nil || !bytes.Equal(contents, expected.Contents) {
		return ErrProposalDrift
	}
	return nil
}

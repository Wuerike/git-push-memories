package inbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Wuerike/git-push-memories/internal/reference"
)

var proposalPathPattern = regexp.MustCompile(`^proposals/([0-9a-f]{32})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)

type Snapshot struct {
	Proposal Proposal
	Head     string
	Blob     string
	Contents []byte
}

func (inbox *Inbox) List(ctx context.Context) ([]Snapshot, error) {
	branch, err := inbox.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != "main" {
		return nil, ErrNotMain
	}
	head, err := inbox.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read inbox HEAD")
	}
	entries, err := inbox.gitInput(ctx, nil, "ls-tree", "-r", "-z", "--full-tree", head, "--", "proposals")
	if err != nil {
		return nil, fmt.Errorf("list proposals")
	}
	var snapshots []Snapshot
	seen := make(map[string]struct{})
	for _, encoded := range bytes.Split(entries, []byte{0}) {
		if len(encoded) == 0 {
			continue
		}
		tab := bytes.IndexByte(encoded, '\t')
		if tab < 0 {
			return nil, ErrInvalidProposal
		}
		fields := strings.Fields(string(encoded[:tab]))
		proposalPath := string(encoded[tab+1:])
		if len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" || path.Dir(proposalPath) != "proposals" || path.Ext(proposalPath) != ".md" {
			return nil, ErrInvalidProposal
		}
		contents, err := inbox.gitInput(ctx, nil, "cat-file", "blob", fields[2])
		if err != nil {
			return nil, ErrInvalidProposal
		}
		proposal, err := parse(proposalPath, contents)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[proposal.ID]; duplicate {
			return nil, ErrInvalidProposal
		}
		seen[proposal.ID] = struct{}{}
		snapshots = append(snapshots, Snapshot{Proposal: proposal, Head: head, Blob: fields[2], Contents: append([]byte(nil), contents...)})
	}
	return snapshots, nil
}

func (inbox *Inbox) Read(ctx context.Context, id string) (Snapshot, error) {
	if len(id) != 32 {
		return Snapshot{}, ErrInvalidProposal
	}
	if _, err := hex.DecodeString(id); err != nil || strings.ToLower(id) != id {
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

func (inbox *Inbox) Confirm(ctx context.Context, expected Snapshot) error {
	branch, err := inbox.gitText(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != "main" {
		return ErrNotMain
	}
	head, err := inbox.gitText(ctx, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head != expected.Head {
		return ErrProposalDrift
	}
	for _, arguments := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "--cached", head, "--", expected.Proposal.Path},
		{"diff", "--quiet", "--no-ext-diff", "--", expected.Proposal.Path},
	} {
		if _, err := inbox.gitInput(ctx, nil, arguments...); err != nil {
			return ErrProposalDrift
		}
	}
	root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", inbox.memoryRoot.Fd()))
	if err != nil {
		return ErrProposalDrift
	}
	defer root.Close()
	file, err := root.Open(expected.Proposal.Path)
	if err != nil {
		return ErrProposalDrift
	}
	contents, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(contents, expected.Contents) {
		return ErrProposalDrift
	}
	entry, err := inbox.gitInput(ctx, nil, "ls-files", "--stage", "-z", "--", expected.Proposal.Path)
	if err != nil || bytes.Count(entry, []byte{0}) != 1 {
		return ErrProposalDrift
	}
	entry = bytes.TrimSuffix(entry, []byte{0})
	tab := bytes.IndexByte(entry, '\t')
	fields := strings.Fields(string(entry[:max(tab, 0)]))
	if tab < 0 || len(fields) != 3 || fields[0] != "100644" || fields[1] != expected.Blob || fields[2] != "0" || string(entry[tab+1:]) != expected.Proposal.Path {
		return ErrProposalDrift
	}
	return nil
}

func parse(proposalPath string, contents []byte) (Proposal, error) {
	if !utf8.Valid(contents) || len(contents) == 0 || contents[len(contents)-1] != '\n' || bytes.Contains(contents, []byte{'\r'}) {
		return Proposal{}, ErrInvalidProposal
	}
	matches := proposalPathPattern.FindStringSubmatch(proposalPath)
	if matches == nil {
		return Proposal{}, ErrInvalidProposal
	}
	lines := strings.Split(string(contents), "\n")
	if len(lines) < 8 || lines[0] != "---" || !strings.HasPrefix(lines[1], "schema_version: ") || !strings.HasPrefix(lines[2], "suggested_action: ") {
		return Proposal{}, ErrInvalidProposal
	}
	schemaVersion, err := strconv.Atoi(strings.TrimPrefix(lines[1], "schema_version: "))
	if err != nil || schemaVersion != 1 && schemaVersion != 2 || lines[1] != "schema_version: "+strconv.Itoa(schemaVersion) {
		return Proposal{}, ErrInvalidProposal
	}
	action, err := quotedValue(lines[2], "suggested_action: ")
	if err != nil {
		return Proposal{}, ErrInvalidProposal
	}
	line := 3
	target := ""
	if line < len(lines) && strings.HasPrefix(lines[line], "target: ") {
		target, err = quotedValue(lines[line], "target: ")
		if err != nil {
			return Proposal{}, ErrInvalidProposal
		}
		line++
	}
	sourcePaths := make([]string, 0)
	sourceReferences := make([]reference.Reference, 0)
	if line >= len(lines) {
		return Proposal{}, ErrInvalidProposal
	}
	if schemaVersion == 1 {
		switch lines[line] {
		case "source_paths: []":
			line++
		case "source_paths:":
			line++
			for line < len(lines) && strings.HasPrefix(lines[line], "  - ") {
				value, err := quotedValue(lines[line], "  - ")
				if err != nil {
					return Proposal{}, ErrInvalidProposal
				}
				sourcePaths = append(sourcePaths, value)
				line++
			}
			if len(sourcePaths) == 0 {
				return Proposal{}, ErrInvalidProposal
			}
		default:
			return Proposal{}, ErrInvalidProposal
		}
	} else {
		if lines[line] != "source_references:" {
			return Proposal{}, ErrInvalidProposal
		}
		line++
		for line < len(lines) && strings.HasPrefix(lines[line], "  - repository: ") {
			if line+3 >= len(lines) {
				return Proposal{}, ErrInvalidProposal
			}
			repository, repositoryErr := quotedValue(lines[line], "  - repository: ")
			revision, revisionErr := quotedValue(lines[line+1], "    revision: ")
			sourcePath, pathErr := quotedValue(lines[line+2], "    path: ")
			contentHash, hashErr := quotedValue(lines[line+3], "    content_hash: ")
			if repositoryErr != nil || revisionErr != nil || pathErr != nil || hashErr != nil {
				return Proposal{}, ErrInvalidProposal
			}
			sourceReferences = append(sourceReferences, reference.Reference{Repository: repository, Revision: revision, Path: sourcePath, ContentHash: contentHash})
			line += 4
		}
		if len(sourceReferences) == 0 {
			return Proposal{}, ErrInvalidProposal
		}
	}
	if line+4 > len(lines) || lines[line] != "---" || lines[line+1] != "" || !strings.HasPrefix(lines[line+2], "# ") || lines[line+3] != "" {
		return Proposal{}, ErrInvalidProposal
	}
	title := strings.TrimPrefix(lines[line+2], "# ")
	knowledge := strings.Join(lines[line+4:], "\n")
	proposal := Proposal{
		SchemaVersion: schemaVersion, ID: matches[1], Path: proposalPath, Title: title,
		SuggestedAction: Action(action), Target: target, Knowledge: knowledge, SourcePaths: sourcePaths, SourceReferences: sourceReferences,
	}
	if matches[2] != slug(title) {
		return Proposal{}, ErrInvalidProposal
	}
	if _, err := validate(Request{Title: title, SuggestedAction: Action(action), Target: target, Knowledge: knowledge, SourcePaths: sourcePaths, SourceReferences: sourceReferences}); err != nil {
		return Proposal{}, ErrInvalidProposal
	}
	if !bytes.Equal(render(proposal), contents) {
		return Proposal{}, ErrInvalidProposal
	}
	return proposal, nil
}

func quotedValue(line, prefix string) (string, error) {
	encoded := strings.TrimPrefix(line, prefix)
	if encoded == line {
		return "", errors.New("missing prefix")
	}
	value, err := strconv.Unquote(encoded)
	if err != nil || strconv.Quote(value) != encoded {
		return "", errors.New("invalid quoted value")
	}
	return value, nil
}

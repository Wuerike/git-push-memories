// Package reference snapshots and verifies immutable source references using local Git objects.
package reference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidReference     = errors.New("invalid source reference")
	ErrInvalidRoot          = errors.New("invalid source repository root")
	ErrBlobTooLarge         = errors.New("source blob exceeds verification limit")
	ErrIntegrityUnavailable = errors.New("source reference is unavailable")
	ErrIntegrityMismatch    = errors.New("source reference does not match")
	ErrInvalidRange         = errors.New("invalid source excerpt range")
	ErrBudgetTooSmall       = errors.New("source excerpt budget too small")
	ErrNonTextBlob          = errors.New("source blob is not UTF-8 text")
	objectIDPattern         = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	repositoryPattern       = regexp.MustCompile(`^git-lineage-sha256:[0-9a-f]{64}$`)
	hashPattern             = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	errRevisionUnavailable  = errors.New("source revision is unavailable")
)

const (
	MaxReferences = 16
	maxBlobBytes  = 8 * 1024 * 1024
)

type Reference struct {
	Repository  string `json:"repository"`
	Revision    string `json:"revision"`
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
}

type Snapshot struct {
	Repository string      `json:"repository"`
	Revision   string      `json:"revision"`
	References []Reference `json:"references"`
}

type Excerpt struct {
	Reference Reference `json:"reference"`
	StartLine int       `json:"start_line"`
	EndLine   int       `json:"end_line"`
	Content   string    `json:"content"`
	Truncated bool      `json:"truncated"`
}

type IntegrityStatus string

const (
	IntegrityVerified         IntegrityStatus = "verified"
	IntegrityUnavailable      IntegrityStatus = "unavailable"
	IntegrityMismatch         IntegrityStatus = "mismatch"
	IntegrityLegacyUnverified IntegrityStatus = "legacy_unverified"
)

type IntegrityReason string

const (
	ReasonRepositoryUnavailable IntegrityReason = "repository_unavailable"
	ReasonRevisionUnavailable   IntegrityReason = "revision_unavailable"
	ReasonInvalidReference      IntegrityReason = "invalid_reference"
	ReasonRepositoryMismatch    IntegrityReason = "repository_mismatch"
	ReasonPathOrBlobMismatch    IntegrityReason = "path_or_blob_mismatch"
	ReasonContentHashMismatch   IntegrityReason = "content_hash_mismatch"
	ReasonBlobTooLarge          IntegrityReason = "blob_too_large"
	ReasonLegacyIdentityMissing IntegrityReason = "missing_repository_revision_and_hash"
)

type SemanticSupport string

const (
	SemanticNotAssessed SemanticSupport = "not_assessed"
	SemanticSupports    SemanticSupport = "supports"
	SemanticContradicts SemanticSupport = "contradicts"
	SemanticUnclear     SemanticSupport = "unclear"
)

type Integrity struct {
	Status IntegrityStatus `json:"status"`
	Reason IntegrityReason `json:"reason,omitempty"`
}

type Evidence struct {
	Reference       *Reference      `json:"reference,omitempty"`
	LegacyPath      string          `json:"legacy_path,omitempty"`
	Integrity       Integrity       `json:"integrity"`
	SemanticSupport SemanticSupport `json:"semantic_support"`
}

type Verifier struct {
	mutex    sync.RWMutex
	bindings map[string]string
}

func New() *Verifier {
	return &Verifier{bindings: make(map[string]string)}
}

func Validate(reference Reference) error {
	if !repositoryPattern.MatchString(reference.Repository) || !objectIDPattern.MatchString(reference.Revision) || !canonicalPath(reference.Path) || !hashPattern.MatchString(reference.ContentHash) {
		return ErrInvalidReference
	}
	return nil
}

func ValidateCollection(references []Reference) error {
	if len(references) == 0 || len(references) > MaxReferences {
		return ErrInvalidReference
	}
	seen := make(map[Reference]struct{}, len(references))
	for _, sourceReference := range references {
		if Validate(sourceReference) != nil {
			return ErrInvalidReference
		}
		if _, duplicate := seen[sourceReference]; duplicate {
			return ErrInvalidReference
		}
		seen[sourceReference] = struct{}{}
	}
	return nil
}

func (verifier *Verifier) Snapshot(ctx context.Context, repositoryRoot, revision string, paths []string) (Snapshot, error) {
	if len(paths) == 0 || len(paths) > MaxReferences {
		return Snapshot{}, ErrInvalidReference
	}
	root, err := validateRoot(ctx, repositoryRoot)
	if err != nil {
		return Snapshot{}, err
	}
	if revision == "" {
		revision, err = gitText(ctx, root, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return Snapshot{}, err
		}
	}
	resolved, err := resolveCommit(ctx, root, revision)
	if err != nil {
		if errors.Is(err, errRevisionUnavailable) {
			return Snapshot{}, ErrInvalidReference
		}
		return Snapshot{}, err
	}
	if resolved != revision {
		return Snapshot{}, ErrInvalidReference
	}
	repositoryID, err := repositoryIdentity(ctx, root, revision)
	if err != nil {
		return Snapshot{}, err
	}
	seen := make(map[string]struct{}, len(paths))
	references := make([]Reference, len(paths))
	for index, sourcePath := range paths {
		if !canonicalPath(sourcePath) {
			return Snapshot{}, ErrInvalidReference
		}
		if _, duplicate := seen[sourcePath]; duplicate {
			return Snapshot{}, ErrInvalidReference
		}
		seen[sourcePath] = struct{}{}
		blob, err := readBlob(ctx, root, revision, sourcePath)
		if err != nil {
			return Snapshot{}, err
		}
		references[index] = Reference{Repository: repositoryID, Revision: revision, Path: sourcePath, ContentHash: contentHash(blob)}
	}
	verifier.mutex.Lock()
	verifier.bindings[repositoryID] = root
	verifier.mutex.Unlock()
	return Snapshot{Repository: repositoryID, Revision: revision, References: references}, nil
}

func (verifier *Verifier) Verify(ctx context.Context, references []Reference, legacyPaths []string) ([]Evidence, error) {
	evidence := make([]Evidence, 0, len(references)+len(legacyPaths))
	for index := range references {
		reference := references[index]
		integrity, err := verifier.verify(ctx, reference)
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, Evidence{Reference: &reference, Integrity: integrity, SemanticSupport: SemanticNotAssessed})
	}
	for _, legacyPath := range legacyPaths {
		evidence = append(evidence, Evidence{
			LegacyPath: legacyPath, Integrity: Integrity{Status: IntegrityLegacyUnverified, Reason: ReasonLegacyIdentityMissing}, SemanticSupport: SemanticNotAssessed,
		})
	}
	return evidence, nil
}

func (verifier *Verifier) Expand(ctx context.Context, sourceReference Reference, startLine, budget int) (Excerpt, error) {
	if startLine < 1 || budget <= 0 || budget > 64*1024 {
		return Excerpt{}, ErrInvalidRange
	}
	evidence, err := verifier.Verify(ctx, []Reference{sourceReference}, nil)
	if err != nil {
		return Excerpt{}, err
	}
	switch evidence[0].Integrity.Status {
	case IntegrityUnavailable:
		return Excerpt{}, ErrIntegrityUnavailable
	case IntegrityMismatch:
		return Excerpt{}, ErrIntegrityMismatch
	case IntegrityVerified:
	default:
		return Excerpt{}, ErrIntegrityMismatch
	}
	verifier.mutex.RLock()
	root := verifier.bindings[sourceReference.Repository]
	verifier.mutex.RUnlock()
	blob, err := readBlob(ctx, root, sourceReference.Revision, sourceReference.Path)
	if err != nil {
		return Excerpt{}, err
	}
	if !utf8.Valid(blob) {
		return Excerpt{}, ErrNonTextBlob
	}
	lines := strings.SplitAfter(string(blob), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if startLine > len(lines) {
		return Excerpt{}, ErrInvalidRange
	}
	start := startLine - 1
	best := Excerpt{}
	for low, high := start+1, len(lines); low <= high; {
		end := low + (high-low)/2
		excerpt := Excerpt{
			Reference: sourceReference, StartLine: startLine, EndLine: end,
			Content: strings.Join(lines[start:end], ""), Truncated: start > 0 || end < len(lines),
		}
		encoded, err := json.Marshal(excerpt)
		if err != nil {
			return Excerpt{}, err
		}
		if len(encoded) <= budget {
			best = excerpt
			low = end + 1
		} else {
			high = end - 1
		}
	}
	if best.EndLine != 0 {
		return best, nil
	}
	return Excerpt{}, ErrBudgetTooSmall
}

func (verifier *Verifier) verify(ctx context.Context, reference Reference) (Integrity, error) {
	if Validate(reference) != nil {
		return Integrity{Status: IntegrityMismatch, Reason: ReasonInvalidReference}, nil
	}
	verifier.mutex.RLock()
	root, available := verifier.bindings[reference.Repository]
	verifier.mutex.RUnlock()
	if !available {
		return Integrity{Status: IntegrityUnavailable, Reason: ReasonRepositoryUnavailable}, nil
	}
	resolved, err := resolveCommit(ctx, root, reference.Revision)
	if err != nil {
		if errors.Is(err, errRevisionUnavailable) {
			return Integrity{Status: IntegrityUnavailable, Reason: ReasonRevisionUnavailable}, nil
		}
		return Integrity{}, err
	}
	if resolved != reference.Revision {
		return Integrity{Status: IntegrityUnavailable, Reason: ReasonRevisionUnavailable}, nil
	}
	repositoryID, err := repositoryIdentity(ctx, root, reference.Revision)
	if err != nil {
		return Integrity{}, err
	}
	if repositoryID != reference.Repository {
		return Integrity{Status: IntegrityMismatch, Reason: ReasonRepositoryMismatch}, nil
	}
	blob, err := readBlob(ctx, root, reference.Revision, reference.Path)
	if err != nil {
		if errors.Is(err, ErrInvalidReference) {
			return Integrity{Status: IntegrityMismatch, Reason: ReasonPathOrBlobMismatch}, nil
		}
		if errors.Is(err, ErrBlobTooLarge) {
			return Integrity{Status: IntegrityMismatch, Reason: ReasonBlobTooLarge}, nil
		}
		return Integrity{}, err
	}
	if contentHash(blob) != reference.ContentHash {
		return Integrity{Status: IntegrityMismatch, Reason: ReasonContentHashMismatch}, nil
	}
	return Integrity{Status: IntegrityVerified}, nil
}

func validateRoot(ctx context.Context, requested string) (string, error) {
	if requested == "" {
		return "", ErrInvalidRoot
	}
	absolute, err := filepath.Abs(requested)
	if err != nil {
		return "", ErrInvalidRoot
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", ErrInvalidRoot
	}
	topLevel, err := gitText(ctx, canonical, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrInvalidRoot
	}
	topLevel, err = filepath.EvalSymlinks(topLevel)
	if err != nil || topLevel != canonical {
		return "", ErrInvalidRoot
	}
	shallow, err := gitText(ctx, canonical, "rev-parse", "--is-shallow-repository")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrInvalidRoot
	}
	if shallow != "false" {
		return "", ErrInvalidRoot
	}
	return canonical, nil
}

func resolveCommit(ctx context.Context, root, revision string) (string, error) {
	if !objectIDPattern.MatchString(revision) {
		return "", ErrInvalidReference
	}
	resolved, err := gitText(ctx, root, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return "", errRevisionUnavailable
		}
		return "", err
	}
	return resolved, nil
}

func repositoryIdentity(ctx context.Context, root, revision string) (string, error) {
	output, err := gitOutput(ctx, root, "rev-list", "--max-parents=0", revision)
	if err != nil {
		return "", err
	}
	roots := strings.Fields(string(output))
	if len(roots) == 0 {
		return "", ErrInvalidReference
	}
	sort.Strings(roots)
	digest := sha256.Sum256([]byte(strings.Join(roots, "\n")))
	return fmt.Sprintf("git-lineage-sha256:%x", digest), nil
}

func readBlob(ctx context.Context, root, revision, sourcePath string) ([]byte, error) {
	entry, err := gitOutput(ctx, root, "ls-tree", "-z", revision, "--", sourcePath)
	if err != nil {
		return nil, err
	}
	if bytes.Count(entry, []byte{0}) != 1 {
		return nil, ErrInvalidReference
	}
	entry = bytes.TrimSuffix(entry, []byte{0})
	tab := bytes.IndexByte(entry, '\t')
	fields := strings.Fields(string(entry[:max(tab, 0)]))
	if tab < 0 || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || string(entry[tab+1:]) != sourcePath {
		return nil, ErrInvalidReference
	}
	sizeText, err := gitText(ctx, root, "cat-file", "-s", fields[2])
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || size < 0 {
		return nil, ErrInvalidReference
	}
	if size > maxBlobBytes {
		return nil, ErrBlobTooLarge
	}
	return gitOutput(ctx, root, "cat-file", "blob", fields[2])
}

func canonicalPath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || path.IsAbs(value) || strings.Contains(value, "\\") || path.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return false
		}
	}
	return value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func contentHash(content []byte) string {
	digest := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", digest)
}

func gitText(ctx context.Context, root string, arguments ...string) (string, error) {
	output, err := gitOutput(ctx, root, arguments...)
	return strings.TrimSpace(string(output)), err
}

func gitOutput(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"--no-optional-locks", "--no-replace-objects", "-C", root}, arguments...)
	command := exec.CommandContext(ctx, "git", commandArguments...)
	command.Env = append(withoutGitEnvironment(os.Environ()), "GIT_NO_LAZY_FETCH=1")
	return command.Output()
}

func withoutGitEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "GIT_") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

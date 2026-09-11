package reference_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/reference"
)

func TestSnapshotAndVerifyCommittedBlobWithoutReadingWorktreeContent(t *testing.T) {
	root := newSourceRoot(t, map[string]string{"services/checkout/config.yaml": "timeout: 5s\n"})
	verifier := reference.New()
	snapshot, err := verifier.Snapshot(context.Background(), root, "", []string{"services/checkout/config.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(snapshot.Repository, "git-lineage-sha256:") || snapshot.Revision != gitOutput(t, root, "rev-parse", "HEAD") || len(snapshot.References) != 1 {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	stored := snapshot.References[0]
	if stored.Repository != snapshot.Repository || stored.Revision != snapshot.Revision || stored.Path != "services/checkout/config.yaml" || !strings.HasPrefix(stored.ContentHash, "sha256:") {
		t.Fatalf("reference = %+v", stored)
	}
	if err := os.WriteFile(filepath.Join(root, "services", "checkout", "config.yaml"), []byte("dirty worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence, err := verifier.Verify(context.Background(), []reference.Reference{stored}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].Integrity.Status != reference.IntegrityVerified || evidence[0].SemanticSupport != reference.SemanticNotAssessed {
		t.Fatalf("Verify() = %+v", evidence)
	}
}

func TestVerifyDistinguishesUnavailableMismatchAndLegacyReference(t *testing.T) {
	root := newSourceRoot(t, map[string]string{"service.go": "package service\n"})
	if err := os.Symlink("service.go", filepath.Join(root, "service-link")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "service-link")
	runGit(t, root, "commit", "--quiet", "-m", "add symlink")
	bound := reference.New()
	snapshot, err := bound.Snapshot(context.Background(), root, "", []string{"service.go"})
	if err != nil {
		t.Fatal(err)
	}
	stored := snapshot.References[0]

	unavailable, err := reference.New().Verify(context.Background(), []reference.Reference{stored}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(unavailable) != 1 || unavailable[0].Integrity.Status != reference.IntegrityUnavailable || unavailable[0].Integrity.Reason != "repository_unavailable" {
		t.Fatalf("unavailable evidence = %+v", unavailable)
	}
	mismatched := stored
	mismatched.ContentHash = "sha256:" + strings.Repeat("0", 64)
	mismatch, err := bound.Verify(context.Background(), []reference.Reference{mismatched}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatch) != 1 || mismatch[0].Integrity.Status != reference.IntegrityMismatch || mismatch[0].Integrity.Reason != "content_hash_mismatch" {
		t.Fatalf("mismatch evidence = %+v", mismatch)
	}
	wrongPath := stored
	wrongPath.Path = "missing.go"
	pathMismatch, err := bound.Verify(context.Background(), []reference.Reference{wrongPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pathMismatch) != 1 || pathMismatch[0].Integrity.Status != reference.IntegrityMismatch || pathMismatch[0].Integrity.Reason != reference.ReasonPathOrBlobMismatch {
		t.Fatalf("path mismatch evidence = %+v", pathMismatch)
	}
	symlink := stored
	symlink.Path = "service-link"
	symlinkEvidence, err := bound.Verify(context.Background(), []reference.Reference{symlink}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(symlinkEvidence) != 1 || symlinkEvidence[0].Integrity.Status != reference.IntegrityMismatch || symlinkEvidence[0].Integrity.Reason != reference.ReasonPathOrBlobMismatch {
		t.Fatalf("symlink evidence = %+v", symlinkEvidence)
	}
	missingRevision := stored
	missingRevision.Revision = strings.Repeat("f", len(stored.Revision))
	missing, err := bound.Verify(context.Background(), []reference.Reference{missingRevision}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].Integrity.Status != reference.IntegrityUnavailable || missing[0].Integrity.Reason != "revision_unavailable" {
		t.Fatalf("missing revision evidence = %+v", missing)
	}
	legacy, err := bound.Verify(context.Background(), nil, []string{"service.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 1 || legacy[0].LegacyPath != "service.go" || legacy[0].Integrity.Status != reference.IntegrityLegacyUnverified || legacy[0].SemanticSupport != reference.SemanticNotAssessed {
		t.Fatalf("legacy evidence = %+v", legacy)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := bound.Verify(cancelled, []reference.Reference{stored}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify() cancelled error = %v, want context.Canceled", err)
	}
}

func TestSnapshotRejectsSymbolicRevisionDuplicatePathSymlinkAndShallowRepository(t *testing.T) {
	root := newSourceRoot(t, map[string]string{"service.go": "package service\n"})
	if err := os.Symlink("service.go", filepath.Join(root, "service-link")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "service-link")
	runGit(t, root, "commit", "--quiet", "-m", "add symlink")
	commit := gitOutput(t, root, "rev-parse", "HEAD")
	runGit(t, root, "update-index", "--add", "--cacheinfo", "160000,"+commit+",module")
	runGit(t, root, "commit", "--quiet", "-m", "add gitlink")
	verifier := reference.New()
	for _, test := range []struct {
		name     string
		revision string
		paths    []string
	}{
		{name: "symbolic revision", revision: "HEAD", paths: []string{"service.go"}},
		{name: "duplicate path", paths: []string{"service.go", "service.go"}},
		{name: "symlink", paths: []string{"service-link"}},
		{name: "submodule", paths: []string{"module"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifier.Snapshot(context.Background(), root, test.revision, test.paths); !errors.Is(err, reference.ErrInvalidReference) {
				t.Fatalf("Snapshot() error = %v, want ErrInvalidReference", err)
			}
		})
	}
	shallow := filepath.Join(t.TempDir(), "shallow")
	command := exec.Command("git", "clone", "--quiet", "--depth=1", "file://"+root, shallow)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git clone shallow: %v: %s", err, output)
	}
	if _, err := verifier.Snapshot(context.Background(), shallow, "", []string{"service.go"}); !errors.Is(err, reference.ErrInvalidRoot) {
		t.Fatalf("Snapshot() shallow error = %v, want ErrInvalidRoot", err)
	}
}

func TestExpandReturnsBoundedExactCommittedText(t *testing.T) {
	root := newSourceRoot(t, map[string]string{"service.go": "line one\nline two\nline three\n"})
	verifier := reference.New()
	snapshot, err := verifier.Snapshot(context.Background(), root, "", []string{"service.go"})
	if err != nil {
		t.Fatal(err)
	}
	excerpt, err := verifier.Expand(context.Background(), snapshot.References[0], 2, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if excerpt.StartLine != 2 || excerpt.EndLine != 3 || excerpt.Content != "line two\nline three\n" || !excerpt.Truncated {
		t.Fatalf("Expand() = %+v", excerpt)
	}
	if _, err := verifier.Expand(context.Background(), snapshot.References[0], 1, 1); !errors.Is(err, reference.ErrBudgetTooSmall) {
		t.Fatalf("Expand() tiny budget error = %v", err)
	}
}

func newSourceRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	for name, content := range files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "source")
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

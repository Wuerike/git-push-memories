package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/consolidation"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHelpAndVersionDoNotRequireRoots(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"help"}, want: "gpm mcp"},
		{name: "long help", args: []string{"--help"}, want: "gpm mcp"},
		{name: "version", args: []string{"version"}, want: "gpm test-version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := Run(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr, "test-version")
			if exitCode != 0 {
				t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q, want containing %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// fakeClaude puts a claude executable that always succeeds first on PATH and isolates its configuration.
func fakeClaude(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "claude"))
}

func run(args ...string) (int, string, string) {
	return runWithInput("", args...)
}

func runWithInput(input string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(input), &stdout, &stderr, "test")
	return code, stdout.String(), stderr.String()
}

func remoteRef(t *testing.T, remote, ref string) string {
	t.Helper()
	command := exec.Command("git", "-C", remote, "rev-parse", "--verify", "--quiet", ref)
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

func TestSetupConfiguresGlobalCommands(t *testing.T) {
	wt.Env(t)
	fakeClaude(t)
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\",\"decision\"]}\n"})

	code, stdout, stderr := run("setup", remote, "--yes")
	if code != 0 {
		t.Fatalf("setup exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Use /remember <fact> and /promote in any Claude Code session") {
		t.Fatalf("setup output = %q", stdout)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("global configuration is not usable: %v", err)
	}
	if cfg.RepoURL != remote || !slices.Equal(cfg.MemoryCategories, []string{"domains", "decision"}) {
		t.Fatalf("configuration = %+v", cfg)
	}
	for _, worktree := range []string{cfg.Paths.Read, cfg.Paths.Proposals} {
		if _, err := os.Stat(filepath.Join(worktree, ".git")); err != nil {
			t.Fatalf("setup did not create worktree %s: %v", worktree, err)
		}
	}
	if remoteRef(t, remote, "refs/heads/proposals") == "" {
		t.Fatal("setup did not publish the proposals branch")
	}
}

func TestSetupFlagsAndErrors(t *testing.T) {
	t.Run("repository URL is required first", func(t *testing.T) {
		wt.Env(t)
		fakeClaude(t)
		for _, args := range [][]string{{"setup"}, {"setup", "--yes"}} {
			if code, _, stderr := run(args...); code != 2 || !strings.Contains(stderr, "repository URL") {
				t.Fatalf("%v: exit = %d, stderr = %q", args, code, stderr)
			}
		}
	})

	t.Run("without a terminal --yes is required", func(t *testing.T) {
		wt.Env(t)
		fakeClaude(t)
		remote := wt.Remote(t)
		wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\"]}\n"})
		code, _, stderr := run("setup", remote)
		if code != 2 || !strings.Contains(stderr, "--yes") {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if url, _ := config.StoredURL(); url != "" {
			t.Fatalf("refused setup saved configuration for %q", url)
		}
	})

	t.Run("an empty repository needs --categories", func(t *testing.T) {
		wt.Env(t)
		fakeClaude(t)
		remote := wt.Remote(t)
		code, _, stderr := run("setup", remote, "--yes")
		if code != 1 || !strings.Contains(stderr, "--categories") {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if remoteRef(t, remote, "refs/heads/main") != "" {
			t.Fatal("failed setup initialized the remote")
		}
		if url, _ := config.StoredURL(); url != "" {
			t.Fatalf("failed setup saved configuration for %q", url)
		}
	})

	t.Run("--categories initializes an empty repository", func(t *testing.T) {
		wt.Env(t)
		fakeClaude(t)
		remote := wt.Remote(t)
		if code, _, stderr := run("setup", remote, "--yes", "--categories", "domains, decision"); code != 0 {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		contents := wt.Git(t, remote, "show", "main:gpm.json")
		categories, err := config.ParseRepositoryConfig([]byte(contents))
		if err != nil || !slices.Equal(categories, []string{"domains", "decision"}) {
			t.Fatalf("initialized gpm.json = %q, %v", contents, err)
		}
		cfg, err := config.Load()
		if err != nil || !slices.Equal(cfg.MemoryCategories, []string{"domains", "decision"}) {
			t.Fatalf("Load() = %+v, %v", cfg, err)
		}
	})

	t.Run("invalid categories are rejected", func(t *testing.T) {
		wt.Env(t)
		fakeClaude(t)
		remote := wt.Remote(t)
		if code, _, _ := run("setup", remote, "--yes", "--categories", "Not Valid"); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if remoteRef(t, remote, "refs/heads/main") != "" {
			t.Fatal("invalid categories initialized the remote")
		}
	})

	t.Run("a different URL is refused once configured", func(t *testing.T) {
		managed := wt.Setup(t, nil)
		fakeClaude(t)
		other := wt.Remote(t)
		wt.Seed(t, other, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\"]}\n"})
		code, _, stderr := run("setup", other, "--yes")
		if code != 1 || !strings.Contains(stderr, "already configured for "+wt.GitHubURL) {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if url, _ := config.StoredURL(); url != managed.Config.RepoURL {
			t.Fatalf("configuration changed to %q", url)
		}
	})

	t.Run("repeating setup for the same URL is safe", func(t *testing.T) {
		managed := wt.Setup(t, nil)
		fakeClaude(t)
		if code, _, stderr := run("setup", wt.GitHubURL, "--yes"); code != 0 {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if url, _ := config.StoredURL(); url != managed.Config.RepoURL {
			t.Fatalf("configuration = %q", url)
		}
	})

	t.Run("Claude Code must be installed", func(t *testing.T) {
		wt.Env(t)
		empty := t.TempDir()
		t.Setenv("PATH", empty)
		code, _, stderr := run("setup", "git@github.com:o/r.git", "--yes")
		if code != 1 || !strings.Contains(stderr, "Claude Code") {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
	})
}

func TestMCPRequiresSetup(t *testing.T) {
	wt.Env(t)
	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"mcp"}, strings.NewReader(""), &stdout, &stderr, "test-version")
	if exitCode == 0 {
		t.Fatal("Run() exit code = 0, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want protocol-only empty output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "run setup first") {
		t.Fatalf("stderr = %q, want setup instruction", stderr.String())
	}
}

func TestMCPDoesNotExposeConfiguredURL(t *testing.T) {
	wt.Env(t)
	secretURL := "git@example.invalid:private-user-path/private-repo.git"
	if err := config.Save(secretURL); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"mcp"}, {"retrieve", "query", "--budget-bytes", "1024"}, {"promote", "--finish"}} {
		code, stdout, stderr := run(args...)
		if code == 0 {
			t.Fatalf("%v: exit code = 0, want failure", args)
		}
		for _, output := range []string{stdout, stderr} {
			if strings.Contains(output, "private-user-path") || strings.Contains(output, "private-repo") {
				t.Fatalf("%v exposed the repository URL: %q", args, output)
			}
		}
	}
}

const checkoutPage = "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n\ntimeout team-b\n"

func TestRetrieveReturnsVersionedReadOnlyContextFromReadWorktree(t *testing.T) {
	managed := wt.Setup(t, map[string]string{"memories/domains/checkout-api.md": checkoutPage})
	cfg := managed.Config
	beforeRead, beforeProposals := treeFingerprint(t, cfg.Paths.Read), treeFingerprint(t, cfg.Paths.Proposals)
	beforeRemote := remoteRef(t, managed.Remote, "refs/heads/proposals")

	code, stdout, stderr := run("retrieve", "checkout timeout team", "--budget-bytes", "2048")
	if code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, stderr)
	}
	var response struct {
		SchemaVersion int                     `json:"schema_version"`
		Data          retrieval.ContextBundle `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion != 1 || len(response.Data.Items) != 1 {
		t.Fatalf("retrieve response = %+v", response)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if treeFingerprint(t, cfg.Paths.Read) != beforeRead {
		t.Fatal("retrieve changed the read worktree")
	}
	if treeFingerprint(t, cfg.Paths.Proposals) != beforeProposals || remoteRef(t, managed.Remote, "refs/heads/proposals") != beforeRemote {
		t.Fatal("retrieve touched proposals")
	}
}

func TestRetrieveSeesProposalsOnlyAfterTheyAreMerged(t *testing.T) {
	managed := wt.Setup(t, nil)
	managed.Remember(t, "Checkout timeout team-b")
	retrieve := func() int {
		code, stdout, stderr := run("retrieve", "checkout timeout team", "--budget-bytes", "2048")
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		var response struct {
			Data retrieval.ContextBundle `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &response); err != nil {
			t.Fatal(err)
		}
		return len(response.Data.Items)
	}
	if got := retrieve(); got != 0 {
		t.Fatalf("retrieve returned %d items for an unmerged proposal", got)
	}
	// Publishing to main and letting the refresh interval pass makes retrieve fetch the new main.
	managed.PublishMain(t, map[string]string{"memories/domains/checkout-api.md": checkoutPage})
	if err := os.Remove(filepath.Join(managed.Config.Paths.State, "fetch-attempt")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if got := retrieve(); got != 1 {
		t.Fatalf("retrieve returned %d items after main was published, want 1", got)
	}
}

func TestMCPRunsOnStdioReadingFromTheReadWorktree(t *testing.T) {
	managed := wt.Setup(t, map[string]string{"memories/domains/checkout-api.md": checkoutPage})
	cfg := managed.Config
	// A fresh fetch stamp keeps the server's detached background refresh from racing with the checks below.
	if err := os.WriteFile(filepath.Join(cfg.Paths.State, "fetch-attempt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeRead, beforeProposals := treeFingerprint(t, cfg.Paths.Read), treeFingerprint(t, cfg.Paths.Proposals)
	beforeRemote := wt.Git(t, managed.Remote, "for-each-ref")

	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run(context.Background(), []string{"mcp"}, serverInput, serverOutput, &stderr, "test-version")
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.IOTransport{Reader: clientInput, Writer: clientOutput}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []string{"memory_expand", "memory_page_read", "memory_promote_check", "memory_promote_finish", "memory_promote_prepare", "memory_promote_submit", "memory_proposal_list", "memory_propose", "memory_search"}
	if len(result.Tools) != len(wantTools) {
		t.Fatalf("ListTools() = %+v, want %v", result.Tools, wantTools)
	}
	for index, want := range wantTools {
		if result.Tools[index].Name != want {
			t.Fatalf("ListTools()[%d] = %q, want %q", index, result.Tools[index].Name, want)
		}
	}
	toolResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": "checkout timeout", "budget_bytes": 2048},
	})
	if err != nil {
		t.Fatal(err)
	}
	if toolResult.IsError {
		t.Fatalf("memory_search returned tool error: %+v", toolResult.Content)
	}
	encoded, err := json.Marshal(toolResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		SchemaVersion int                     `json:"schema_version"`
		Data          retrieval.ContextBundle `json:"data"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion != 1 || len(response.Data.Items) != 1 {
		t.Fatalf("memory_search response = %+v", response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case exitCode := <-done:
		if exitCode != 0 {
			t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP server did not stop after the client disconnected")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if got := treeFingerprint(t, cfg.Paths.Read); got != beforeRead {
		t.Fatalf("read worktree changed: before %s, after %s", beforeRead, got)
	}
	if got := treeFingerprint(t, cfg.Paths.Proposals); got != beforeProposals {
		t.Fatalf("proposals worktree changed: before %s, after %s", beforeProposals, got)
	}
	if got := wt.Git(t, managed.Remote, "for-each-ref"); got != beforeRemote {
		t.Fatalf("remote changed: before %q, after %q", beforeRemote, got)
	}
	if _, err := os.Stat(cfg.Paths.Release); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading created a release worktree: %v", err)
	}
}

func treeFingerprint(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", relative, info.Mode())
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(contents)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func TestPromoteFlagValidation(t *testing.T) {
	wt.Setup(t, nil)
	for _, args := range [][]string{{"promote"}, {"promote", "--yes"}, {"promote", "--resume"}, {"promote", "--abort"}, {"promote", "--finish", "extra"}, {"promote", "--finish", "--yes"}, {"promote", "--bogus"}} {
		code, stdout, _ := run(args...)
		if code != 2 || strings.Contains(stdout, "finished") {
			t.Fatalf("%v: exit=%d stdout=%q", args, code, stdout)
		}
	}
	code, _, stderr := run("promote")
	if code != 2 || !strings.Contains(stderr, "needs --finish") {
		t.Fatalf("missing finish: %d %q", code, stderr)
	}
}

func TestPromoteRequiresSetup(t *testing.T) {
	wt.Env(t)
	code, _, stderr := run("promote", "--finish")
	if code != 2 || !strings.Contains(stderr, "run setup first") {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
}

func publishedCLIPromotion(t *testing.T) (*wt.Managed, consolidation.Prepared, string) {
	t.Helper()
	managed := wt.Setup(t, nil)
	proposal := managed.Remember(t, "Checkout timeout")
	manager := consolidation.New(managed.Config)
	prepared, err := manager.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(prepared.Worktree, "memories/domains/checkout-timeout.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Checkout timeout\nThe timeout is nine seconds.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(context.Background(), proposal.ID+" creates checkout timeout memory."); err != nil {
		t.Fatal(err)
	}
	return managed, prepared, proposal.Path
}

func TestPromoteFinishConfirmsMergeAndCleansApprovedInputs(t *testing.T) {
	managed, prepared, proposalPath := publishedCLIPromotion(t)
	clone := filepath.Join(t.TempDir(), "review")
	wt.Git(t, filepath.Dir(clone), "clone", "-q", "-b", "main", managed.Remote, clone)
	wt.Git(t, clone, "merge", "--squash", "origin/"+prepared.Branch)
	wt.Git(t, clone, "commit", "-q", "-m", "Reviewed consolidation")
	wt.Git(t, clone, "push", "-q", "origin", "main")
	merged := wt.Git(t, clone, "rev-parse", "HEAD")
	wt.FakeGH(t).SetPR(t, "MERGED", merged)
	code, stdout, stderr := run("promote", "--finish")
	if code != 0 || !strings.Contains(stdout, "Promotion finished: 1 proposal(s) removed") || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if paths := wt.Git(t, managed.Remote, "ls-tree", "-r", "--name-only", "proposals", "--", proposalPath); paths != "" {
		t.Fatalf("concluded proposal remains: %s", paths)
	}
	if _, err := os.Stat(managed.Config.Paths.Published); !os.IsNotExist(err) {
		t.Fatalf("publication record remains: %v", err)
	}
	if wt.Git(t, managed.Config.Paths.Read, "rev-parse", "HEAD") != merged {
		t.Fatal("CLI did not refresh published reads")
	}
}

func TestPromoteFinishWaitsForConfirmedMerge(t *testing.T) {
	managed, _, proposalPath := publishedCLIPromotion(t)
	before := wt.Git(t, managed.Remote, "show-ref")
	code, stdout, stderr := run("promote", "--finish")
	if code != 1 || !strings.Contains(stdout, "not merged yet") || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if wt.Git(t, managed.Remote, "show-ref") != before || wt.Git(t, managed.Remote, "ls-tree", "-r", "--name-only", "proposals", "--", proposalPath) == "" {
		t.Fatal("open PR cleanup changed the inbox")
	}
}

func TestPromoteFinishReportsMissingGitHubWithoutManualFallback(t *testing.T) {
	managed, _, proposalPath := publishedCLIPromotion(t)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	code, stdout, stderr := runWithInput("y\n", "promote", "--finish")
	if code != 1 || !strings.Contains(stderr, "gh is not installed or not authenticated") || strings.Contains(stdout, "[y/N]") || strings.Contains(stdout, "finished") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if wt.Git(t, managed.Remote, "ls-tree", "-r", "--name-only", "proposals", "--", proposalPath) == "" {
		t.Fatal("unconfirmed merge removed a proposal")
	}
	if _, err := os.Stat(managed.Config.Paths.Published); err != nil {
		t.Fatalf("unconfirmed publication lost its record: %v", err)
	}
}

func TestPromoteFinishWithoutPublication(t *testing.T) {
	wt.Setup(t, nil)
	code, stdout, stderr := run("promote", "--finish")
	if code != 1 || !strings.Contains(stdout, "No published promotion") || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestPromoteFinishReportsClosedPR(t *testing.T) {
	publishedCLIPromotion(t)
	wt.FakeGH(t).SetPR(t, "CLOSED", "")
	code, _, stderr := run("promote", "--finish")
	if code != 1 || !strings.Contains(stderr, "closed without merging") {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
}

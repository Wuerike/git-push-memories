package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHelpAndVersionDoNotRequireRoots(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"help"}, want: "git-push-memories mcp"},
		{name: "long help", args: []string{"--help"}, want: "git-push-memories mcp"},
		{name: "version", args: []string{"version"}, want: "git-push-memories test-version"},
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

func TestMCPRequiresExplicitRoots(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"mcp"}, strings.NewReader(""), &stdout, &stderr, "test-version")
	if exitCode == 0 {
		t.Fatal("Run() exit code = 0, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want protocol-only empty output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "memory root is required") {
		t.Fatalf("stderr = %q, want missing root error", stderr.String())
	}
}

func TestMCPDoesNotExposeInvalidRootPath(t *testing.T) {
	invalidRoot := filepath.Join(t.TempDir(), "private-user-path")
	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"mcp",
		"--memory-root", invalidRoot,
		"--code-root", invalidRoot,
	}, strings.NewReader(""), &stdout, &stderr, "test-version")
	if exitCode == 0 {
		t.Fatal("Run() exit code = 0, want failure")
	}
	if strings.Contains(stderr.String(), invalidRoot) || strings.Contains(stderr.String(), "private-user-path") {
		t.Fatalf("stderr exposed invalid root: %q", stderr.String())
	}
}

func TestRetrieveReturnsVersionedReadOnlyContext(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	codeRoot := newGitRoot(t, "code")
	pagePath := filepath.Join(memoryRoot, "services", "checkout-api.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	page := "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n\ntimeout team-b\n"
	if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "config", "user.name", "Test")
	runGit(t, memoryRoot, "config", "user.email", "test@example.com")
	runGit(t, memoryRoot, "add", ".")
	runGit(t, memoryRoot, "commit", "--quiet", "-m", "initial")
	beforeMemory := treeFingerprint(t, memoryRoot)
	beforeCode := treeFingerprint(t, codeRoot)

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"retrieve", "checkout timeout team",
		"--memory-root", memoryRoot,
		"--code-root", codeRoot,
		"--budget-bytes", "2048",
	}, strings.NewReader(""), &stdout, &stderr, "test-version")
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var response struct {
		SchemaVersion int                     `json:"schema_version"`
		Data          retrieval.ContextBundle `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SchemaVersion != 1 || len(response.Data.Items) != 1 {
		t.Fatalf("retrieve response = %+v", response)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if treeFingerprint(t, memoryRoot) != beforeMemory || treeFingerprint(t, codeRoot) != beforeCode {
		t.Fatal("retrieve changed a configured worktree")
	}
}

func TestProposalShowAndRejectCommands(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	codeRoot := newGitRoot(t, "code")
	pagePath := filepath.Join(memoryRoot, "services", "checkout-api.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	page := "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n\nold\n"
	if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "config", "user.name", "Test")
	runGit(t, memoryRoot, "config", "user.email", "test@example.com")
	runGit(t, memoryRoot, "add", ".")
	runGit(t, memoryRoot, "commit", "--quiet", "-m", "initial")
	stateRoot := filepath.Join(t.TempDir(), "state")
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.Inspect(intake.NewCandidate([]byte(`{"target_path":"services/checkout-api.md","section_id":"checkout-api-operation","replacement":"\nnew\n","source_paths":[]}`)))
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	common := []string{"--memory-root", memoryRoot, "--code-root", codeRoot, "--state-root", stateRoot}

	var stdout, stderr bytes.Buffer
	showArgs := append([]string{"proposal", "show", view.ID}, common...)
	if exitCode := Run(context.Background(), showArgs, strings.NewReader(""), &stdout, &stderr, "test"); exitCode != 0 || !strings.Contains(stdout.String(), view.ID) {
		t.Fatalf("proposal show exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	rejectArgs := append([]string{"proposal", "reject", view.ID, "--reason", "incorrect"}, common...)
	if exitCode := Run(context.Background(), rejectArgs, strings.NewReader(""), &stdout, &stderr, "test"); exitCode != 0 || !strings.Contains(stdout.String(), `"status":"rejected"`) {
		t.Fatalf("proposal reject exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestMCPRunsOnStdioWithoutCreatingState(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	codeRoot := newGitRoot(t, "code")
	beforeMemory := treeFingerprint(t, memoryRoot)
	beforeCode := treeFingerprint(t, codeRoot)

	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run(context.Background(), []string{
			"mcp",
			"--memory-root", memoryRoot,
			"--code-root", codeRoot,
		}, serverInput, serverOutput, &stderr, "test-version")
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.IOTransport{
		Reader: clientInput,
		Writer: clientOutput,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 2 || result.Tools[0].Name != "memory_expand" || result.Tools[1].Name != "memory_search" {
		t.Fatalf("ListTools() = %+v, want memory_expand and memory_search", result.Tools)
	}
	toolResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search",
		Arguments: map[string]any{
			"query":        "checkout timeout",
			"budget_bytes": 2048,
		},
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
	if response.SchemaVersion != 1 || len(response.Data.Items) != 0 {
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
	if got := treeFingerprint(t, memoryRoot); got != beforeMemory {
		t.Fatalf("memory root changed: before %s, after %s", beforeMemory, got)
	}
	if got := treeFingerprint(t, codeRoot); got != beforeCode {
		t.Fatalf("code root changed: before %s, after %s", beforeCode, got)
	}
}

func newGitRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
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

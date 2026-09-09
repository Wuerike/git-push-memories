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
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
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

func TestSetupConfiguresGlobalCommands(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	memoryRoot := newGitRoot(t, "memory")

	var stdout, stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"setup", memoryRoot}, strings.NewReader(""), &stdout, &stderr, "test"); exitCode != 0 {
		t.Fatalf("setup exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Use /remember <fact> in any Claude Code session") {
		t.Fatalf("setup output = %q", stdout.String())
	}
	if _, err := config.Load(); err != nil {
		t.Fatalf("global configuration is not usable: %v", err)
	}
}

func TestMCPRequiresSetup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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

func TestMCPDoesNotExposeInvalidRootPath(t *testing.T) {
	invalidRoot := filepath.Join(t.TempDir(), "private-user-path")
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := os.MkdirAll(filepath.Join(configHome, "git-push-memories"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "git-push-memories", "config.json"), []byte(`{"schema_version":1,"memory_root":"`+invalidRoot+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"mcp"}, strings.NewReader(""), &stdout, &stderr, "test-version")
	if exitCode == 0 {
		t.Fatal("Run() exit code = 0, want failure")
	}
	if strings.Contains(stderr.String(), invalidRoot) || strings.Contains(stderr.String(), "private-user-path") {
		t.Fatalf("stderr exposed invalid root: %q", stderr.String())
	}
}

func TestRetrieveReturnsVersionedReadOnlyContext(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	configure(t, memoryRoot)
	pagePath := filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md")
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

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"retrieve", "checkout timeout team",
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
	if treeFingerprint(t, memoryRoot) != beforeMemory {
		t.Fatal("retrieve changed the memory worktree")
	}
}

func TestMCPRunsOnStdioWithoutCreatingState(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	cfg := configure(t, memoryRoot)
	stateRoot := cfg.StateRoot
	beforeMemory := treeFingerprint(t, memoryRoot)

	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run(context.Background(), []string{
			"mcp",
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
	if len(result.Tools) != 3 || result.Tools[0].Name != "memory_expand" || result.Tools[1].Name != "memory_propose" || result.Tools[2].Name != "memory_search" {
		t.Fatalf("ListTools() = %+v, want only read and proposal tools", result.Tools)
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
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state root exists without an accepted proposal: %v", err)
	}
}

func newGitRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "init", "--quiet", "--initial-branch=main", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "gpm.json"), []byte("{\n  \"schema_version\": 1,\n  \"memory_categories\": [\"domains\", \"decision\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "gpm.json")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func configure(t *testing.T, memoryRoot string) config.Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg, err := config.Save(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
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

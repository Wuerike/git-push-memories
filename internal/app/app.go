package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Wuerike/git-push-memories/internal/claudecode"
	"github.com/Wuerike/git-push-memories/internal/codex"
	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/consolidation"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/setup"
)

const usage = `Usage:
  gpm setup <repository-url> [--agent claude-code|codex] [--categories a,b] [--yes]
  gpm promote --finish
  gpm mcp
  gpm version
  gpm help
`

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "gpm %s\n", version)
		return 0
	case "setup":
		return runSetup(ctx, args[1:], stdin, stdout, stderr)
	case "promote":
		return runPromote(ctx, args[1:], stdin, stdout, stderr)
	case "mcp":
		return runMCP(ctx, args[1:], stdin, stdout, stderr, version)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func runSetup(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "setup requires the memory repository URL first")
		return 2
	}
	url := args[0]
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	categories := flags.String("categories", "", "comma-separated memory categories, used only when the repository is created")
	yes := flags.Bool("yes", false, "run without prompts")
	agent := flags.String("agent", "claude-code", "agent integration: claude-code or codex")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return 2
	}
	preflight, install := claudecode.Preflight, claudecode.Install
	switch *agent {
	case "claude-code":
	case "codex":
		preflight, install = codex.Preflight, codex.Install
	default:
		fmt.Fprintf(stderr, "unknown agent %q; use claude-code or codex\n", *agent)
		return 2
	}
	interactive := isTerminal(stdin)
	if !interactive && !*yes {
		fmt.Fprintln(stderr, "setup needs a terminal for prompts; pass --yes (and --categories for a new repository)")
		return 2
	}
	if err := preflight(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	prepared, err := setup.Prepare(ctx, setup.Options{URL: url, Categories: func() ([]string, error) {
		if *categories != "" {
			return splitCategories(*categories), nil
		}
		if *yes {
			return nil, errors.New("--categories is required with --yes to initialize an empty repository")
		}
		fmt.Fprint(stdout, "Memory categories (comma-separated, e.g. domains,decision): ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return nil, errors.New("memory categories are required")
		}
		return splitCategories(line), nil
	}})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := install(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := config.Save(url); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *agent == "codex" {
		fmt.Fprintf(stdout, "Configured memory for %s. Use $remember <fact> and $promote in any Codex session.\n", prepared.RepoURL)
		fmt.Fprintf(stdout, "Memory directory for Codex --add-dir: %s\n", prepared.StateRoot)
	} else {
		fmt.Fprintf(stdout, "Configured memory for %s. Use /remember <fact> and /promote in any Claude Code session.\n", prepared.RepoURL)
	}
	return 0
}

func splitCategories(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	return err == nil
}

func runMCP(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "mcp does not accept positional arguments")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	server, err := mcpserver.New(cfg, version)
	if err != nil {
		fmt.Fprintln(stderr, "MCP server could not initialize")
		return 1
	}
	if err := server.Run(ctx, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "MCP server stopped with a protocol or transport error")
		return 1
	}
	return 0
}

func runPromote(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("promote", flag.ContinueOnError)
	flags.SetOutput(stderr)
	finish := flags.Bool("finish", false, "confirm the published PR merge and remove concluded proposals")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if !*finish {
		fmt.Fprintln(stderr, "promote needs --finish; start consolidation with /promote in Claude Code or $promote in Codex")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, err := consolidation.New(cfg).Finish(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !result.Found {
		fmt.Fprintln(stdout, "No published promotion to finish.")
		return 1
	}
	if !result.Merged {
		fmt.Fprintln(stdout, "The promotion pull request is not merged yet.")
		return 1
	}
	fmt.Fprintf(stdout, "Promotion finished: %d proposal(s) removed; %d changed input(s) preserved.\n", len(result.Removed), len(result.Preserved))
	return 0
}

package app

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
)

const usage = `Usage:
  git-push-memories mcp --memory-root <path> --code-root <path>
  git-push-memories version
  git-push-memories help
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
		fmt.Fprintf(stdout, "git-push-memories %s\n", version)
		return 0
	case "mcp":
		return runMCP(ctx, args[1:], stdin, stdout, stderr, version)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func runMCP(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	memoryRoot := flags.String("memory-root", "", "path to the memory Git worktree root")
	codeRoot := flags.String("code-root", "", "path to the code Git worktree root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "mcp does not accept positional arguments")
		return 2
	}

	cfg, err := config.Load(*memoryRoot, *codeRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := mcpserver.New(cfg, version).Run(ctx, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "MCP server stopped with a protocol or transport error")
		return 1
	}
	return 0
}

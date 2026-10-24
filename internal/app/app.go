package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Wuerike/git-push-memories/internal/claudecode"
	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/protocol"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/setup"
)

const usage = `Usage:
  gpm setup <repository-url> [--categories a,b] [--yes]
  gpm mcp
  gpm retrieve <query> --budget-bytes <n>
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
	case "mcp":
		return runMCP(ctx, args[1:], stdin, stdout, stderr, version)
	case "retrieve":
		return runRetrieve(ctx, args[1:], stdout, stderr)
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
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return 2
	}
	interactive := isTerminal(stdin)
	if !interactive && !*yes {
		fmt.Fprintln(stderr, "setup needs a terminal for prompts; pass --yes (and --categories for a new repository)")
		return 2
	}
	if err := claudecode.Preflight(); err != nil {
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
	if _, err := claudecode.Install(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := config.Save(url); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "Configured memory for %s. Use /remember <fact> and /promote in any Claude Code session.\n", prepared.Config.RepoURL)
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

func runRetrieve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, "retrieve query is required")
		return 2
	}
	query := args[0]
	flags := flag.NewFlagSet("retrieve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	budget := flags.Int("budget-bytes", 0, "maximum UTF-8 bytes for the ContextBundle JSON")
	pathPrefix := flags.String("path-prefix", "", "optional canonical relative path scope")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "retrieve accepts exactly one query")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	retriever, err := retrieval.New(cfg.MemoryRoot, cfg.MemoryCategories...)
	if err != nil {
		fmt.Fprintln(stderr, "retrieval could not initialize")
		return 1
	}
	defer retriever.Close()
	bundle, err := retriever.Retrieve(ctx, query, retrieval.Scope{PathPrefix: *pathPrefix}, *budget)
	if err != nil {
		fmt.Fprintln(stderr, retrieval.ErrorCode(err))
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(protocol.NewEnvelope(bundle)); err != nil {
		fmt.Fprintln(stderr, "write retrieval response failed")
		return 1
	}
	return 0
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

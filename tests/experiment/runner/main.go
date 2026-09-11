package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	experiment "github.com/Wuerike/git-push-memories/tests/experiment"
)

func main() {
	approved := flag.Bool("approved", false, "confirm the pre-registered authenticated run and USD 25 maximum")
	evidence := flag.String("evidence-root", "", "new directory for sanitized evidence")
	verification := flag.String("verification-file", "", "sanitized outer verification TSV")
	runID := flag.String("run-id", "", "unique run identifier")
	credential := flag.String("credentials", filepath.Join(os.Getenv("HOME"), ".claude", ".credentials.json"), "Claude credential source copied only to tmpfs")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Hour)
	defer cancel()
	if err := experiment.Run(ctx, experiment.RunOptions{
		EvidenceRoot: *evidence, VerificationFile: *verification, RunID: *runID, CredentialPath: *credential,
		Approved: *approved, Input: os.Stdin, Output: os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

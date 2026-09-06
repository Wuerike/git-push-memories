package main

import (
	"context"
	"os"

	"github.com/Wuerike/git-push-memories/internal/app"
	"github.com/Wuerike/git-push-memories/internal/buildinfo"
)

func main() {
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, buildinfo.Version))
}

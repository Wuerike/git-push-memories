package mcpserver

import (
	"context"
	"io"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	config config.Config
	inner  *mcp.Server
}

func New(cfg config.Config, version string) *Server {
	return &Server{
		config: cfg,
		inner: mcp.NewServer(&mcp.Implementation{
			Name:    "git-push-memories",
			Version: version,
		}, nil),
	}
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	transport := &mcp.IOTransport{
		Reader: io.NopCloser(input),
		Writer: nopWriteCloser{Writer: output},
	}
	return s.inner.Run(ctx, transport)
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

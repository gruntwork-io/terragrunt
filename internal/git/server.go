package git

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	gogit "github.com/go-git/go-git/v6"
	backendhttp "github.com/go-git/go-git/v6/backend/http"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
)

// Server is a pure-Go HTTP Git server backed by in-memory storage.
// It is intended for use in tests.
type Server struct {
	store storage.Storer
	repo  *gogit.Repository
	ln    net.Listener
	srv   *http.Server
}

// NewServer creates a Server with an empty in-memory repository.
func NewServer() (*Server, error) {
	store := memory.NewStorage()
	wt := memfs.New()

	repo, err := gogit.Init(
		store,
		gogit.WithWorkTree(wt),
		gogit.WithDefaultBranch(plumbing.NewBranchReferenceName("main")),
	)
	if err != nil {
		return nil, fmt.Errorf("init repo: %w", err)
	}

	return &Server{
		store: store,
		repo:  repo,
	}, nil
}

// Repo returns the underlying go-git repository so callers can create
// commits, branches, etc. before starting the server.
func (s *Server) Repo() *gogit.Repository {
	return s.repo
}

// CommitFile is a convenience that writes a single file to the worktree and
// commits it. It returns the commit hash.
func (s *Server) CommitFile(path string, data []byte, msg string) error {
	w, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	f, err := w.Filesystem.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close file %s: %w", path, err)
	}

	if _, err := w.Add(path); err != nil {
		return fmt.Errorf("add %s: %w", path, err)
	}

	sig := &object.Signature{
		Name:  "Test",
		Email: "test@test.com",
		When:  time.Now(),
	}

	_, err = w.Commit(msg, &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// Start begins serving Git HTTP on a random local port.
// Returns the base URL (e.g. "http://127.0.0.1:12345").
func (s *Server) Start(ctx context.Context) (string, error) {
	loader := &singleRepoLoader{store: s.store}
	backend := backendhttp.NewBackend(loader)

	var lc net.ListenConfig

	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	s.ln = ln
	s.srv = &http.Server{
		Handler: backend,
	}

	go func() { _ = s.srv.Serve(ln) }()

	return "http://" + ln.Addr().String(), nil
}

// Close shuts down the server.
func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}

	return nil
}

// singleRepoLoader implements transport.Loader by always returning the same
// storer, regardless of the endpoint path.
type singleRepoLoader struct {
	store storage.Storer
}

func (l *singleRepoLoader) Load(_ *transport.Endpoint) (storage.Storer, error) {
	return l.store, nil
}

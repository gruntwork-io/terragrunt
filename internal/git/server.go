package git

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	gogit "github.com/go-git/go-git/v6"
	backendhttp "github.com/go-git/go-git/v6/backend/http"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
)

// Server is a pure-Go HTTP Git server backed by in-memory storage.
// It is intended for use in tests.
type Server struct {
	store  storage.Storer
	repo   *gogit.Repository
	ln     net.Listener
	srv    *http.Server
	mounts map[string]storage.Storer
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
		store:  store,
		repo:   repo,
		mounts: map[string]storage.Storer{},
	}, nil
}

// Mount serves other's repository at the given URL path (e.g.
// "/child.git") on this server's listener, so tests can exercise
// same-host relative submodule URLs. Paths without a mount still
// resolve to this server's own repository. Call before [Server.Start];
// the mounted server itself does not need to be started.
func (s *Server) Mount(path string, other *Server) {
	// The HTTP backend strips the leading slash before it builds the
	// endpoint handed to the loader, so mount keys are stored bare.
	s.mounts[strings.TrimPrefix(path, "/")] = other.store
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

// CommitSymlink creates a symlink in the worktree pointing at target and
// commits it. The recorded tree entry is mode 120000 (git's symlink type),
// matching the on-disk representation produced by `git add` on a symlink.
func (s *Server) CommitSymlink(link, target, msg string) error {
	w, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	if err := w.Filesystem.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	if _, err := w.Add(link); err != nil {
		return fmt.Errorf("add %s: %w", link, err)
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

// CommitSubmodule records a gitlink at path pinned to commitHash and,
// when url is non-empty, registers the submodule in .gitmodules before
// committing both. An empty url leaves the gitlink unregistered,
// mirroring the orphaned entry left behind by accidentally committing
// a nested repository. The gitlink lands as a tree entry with mode
// 160000 and type commit, matching `git submodule add`.
func (s *Server) CommitSubmodule(path, url, commitHash, msg string) error {
	w, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	if url != "" {
		if err := appendGitmodules(w, path, url); err != nil {
			return err
		}
	}

	idx, err := s.repo.Storer.Index()
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}

	idx.Entries = append(idx.Entries, &index.Entry{
		Name: path,
		Hash: plumbing.NewHash(commitHash),
		Mode: filemode.Submodule,
	})

	if err := s.repo.Storer.SetIndex(idx); err != nil {
		return fmt.Errorf("write index: %w", err)
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

// Head returns the canonical hash of the current HEAD commit. Useful
// in tests that need to capture a non-tip commit hash before adding
// further commits.
func (s *Server) Head() (string, error) {
	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolve HEAD: %w", err)
	}

	return ref.Hash().String(), nil
}

// Tag creates an annotated tag at the current HEAD with the given name.
func (s *Server) Tag(name string) error {
	ref, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}

	sig := &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}

	if _, err := s.repo.CreateTag(name, ref.Hash(), &gogit.CreateTagOptions{
		Tagger:  sig,
		Message: name,
	}); err != nil {
		return fmt.Errorf("create tag %s: %w", name, err)
	}

	return nil
}

// Branch creates a branch at the current HEAD with the given name.
func (s *Server) Branch(name string) error {
	ref, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}

	branchRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), ref.Hash())
	if err := s.repo.Storer.SetReference(branchRef); err != nil {
		return fmt.Errorf("set branch %s: %w", name, err)
	}

	return nil
}

// SetBranch points the named branch at the given commit hash, creating
// it if missing. Tests use this to rewind a branch past a tagged
// commit so the commit is reachable only via the tag.
func (s *Server) SetBranch(name, hash string) error {
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), plumbing.NewHash(hash))
	if err := s.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("set branch %s to %s: %w", name, hash, err)
	}

	return nil
}

// Start begins serving Git HTTP on a random local port.
// Returns the base URL (e.g. "http://127.0.0.1:12345").
func (s *Server) Start(ctx context.Context) (string, error) {
	loader := &repoLoader{store: s.store, mounts: s.mounts}
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

// gitmodulesPerms is the file mode used for the .gitmodules file
// written into the test worktree.
const gitmodulesPerms = 0o644

// appendGitmodules appends a submodule section for path and url to the
// worktree's .gitmodules file and stages it.
func appendGitmodules(w *gogit.Worktree, path, url string) error {
	existing, err := util.ReadFile(w.Filesystem, ".gitmodules")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitmodules: %w", err)
	}

	section := fmt.Sprintf("[submodule %q]\n\tpath = %s\n\turl = %s\n", path, path, url)

	if err := util.WriteFile(w.Filesystem, ".gitmodules", append(existing, section...), gitmodulesPerms); err != nil {
		return fmt.Errorf("write .gitmodules: %w", err)
	}

	if _, err := w.Add(".gitmodules"); err != nil {
		return fmt.Errorf("add .gitmodules: %w", err)
	}

	return nil
}

// repoLoader implements transport.Loader by returning the default
// storer for any endpoint path, with per-path mounts taking precedence
// so one listener can serve several test repositories.
type repoLoader struct {
	store  storage.Storer
	mounts map[string]storage.Storer
}

func (l *repoLoader) Load(ep *transport.Endpoint) (storage.Storer, error) {
	if store, ok := l.mounts[ep.Path]; ok {
		return store, nil
	}

	return l.store, nil
}

package git

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	serverDirMode  os.FileMode = 0o755
	serverFileMode os.FileMode = 0o644
)

// Server is an HTTP Git server backed by on-disk repositories.
// It is intended for use in tests.
type Server struct {
	ln          net.Listener
	srv         *http.Server
	projectRoot string // root temp dir removed in Close
	bareDir     string // projectRoot/repo.git — the repo served over HTTP
	workDir     string // non-bare clone used for mutations
	gitPath     string
	mounts      []serverMount
}

type serverMount struct {
	path    string
	bareDir string
}

// NewServer creates a Server with an empty repository.
func NewServer() (*Server, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found on PATH: %w", err)
	}

	projectRoot, err := os.MkdirTemp("", "tg-git-server-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	ok := false

	defer func() {
		if !ok {
			_ = os.RemoveAll(projectRoot)
		}
	}()

	s := &Server{
		projectRoot: projectRoot,
		gitPath:     gitPath,
	}

	ctx := context.Background()
	bareDir := filepath.Join(projectRoot, "repo.git")

	// `-b main` pins HEAD in both repos: pushes only ever create
	// refs/heads/main, and a bare HEAD left at git's built-in default
	// (master) would dangle, so clones and ls-remote would not see HEAD.
	if err := s.gitIn(ctx, projectRoot, "init", "--bare", "-b", "main", bareDir); err != nil {
		return nil, fmt.Errorf("init bare repo: %w", err)
	}

	s.bareDir = bareDir

	workDir := filepath.Join(projectRoot, "work")

	if err := os.MkdirAll(workDir, serverDirMode); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	if err := s.gitIn(ctx, workDir, "init", "-b", "main"); err != nil {
		return nil, fmt.Errorf("init work dir: %w", err)
	}

	if err := s.gitIn(ctx, workDir, "remote", "add", "origin", bareDir); err != nil {
		return nil, fmt.Errorf("add remote: %w", err)
	}

	if err := s.gitIn(ctx, workDir, "config", "user.email", "test@test.com"); err != nil {
		return nil, fmt.Errorf("config user.email: %w", err)
	}

	if err := s.gitIn(ctx, workDir, "config", "user.name", "Test"); err != nil {
		return nil, fmt.Errorf("config user.name: %w", err)
	}

	s.workDir = workDir

	ok = true

	return s, nil
}

// Mount serves other's repository at the given URL path (e.g. "/child.git")
// on this server's listener. Call before [Server.Start]; the mounted server
// itself does not need to be started.
func (s *Server) Mount(path string, other *Server) {
	s.mounts = append(s.mounts, serverMount{
		path:    strings.TrimPrefix(path, "/"),
		bareDir: other.bareDir,
	})
}

// CommitFile writes a single file to the working tree and commits it.
func (s *Server) CommitFile(ctx context.Context, path string, data []byte, msg string) error {
	fullPath := filepath.Join(s.workDir, filepath.FromSlash(path))

	if err := os.MkdirAll(filepath.Dir(fullPath), serverDirMode); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}

	if err := os.WriteFile(fullPath, data, serverFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := s.gitIn(ctx, s.workDir, "add", path); err != nil {
		return fmt.Errorf("add %s: %w", path, err)
	}

	if err := s.gitIn(ctx, s.workDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.push(ctx)
}

// CommitSymlink creates a symlink in the working tree and commits it.
// The recorded tree entry uses mode 120000 (git's symlink type).
func (s *Server) CommitSymlink(ctx context.Context, link, target, msg string) error {
	fullLink := filepath.Join(s.workDir, filepath.FromSlash(link))

	if err := os.MkdirAll(filepath.Dir(fullLink), serverDirMode); err != nil {
		return fmt.Errorf("mkdir for symlink %s: %w", link, err)
	}

	if err := os.Symlink(target, fullLink); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	if err := s.gitIn(ctx, s.workDir, "add", link); err != nil {
		return fmt.Errorf("add %s: %w", link, err)
	}

	if err := s.gitIn(ctx, s.workDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.push(ctx)
}

// CommitSubmodule records a gitlink at path pinned to commitHash and, when url
// is non-empty, registers the submodule in .gitmodules before committing both.
// An empty url leaves the gitlink unregistered, mirroring the orphaned entry
// left behind by accidentally committing a nested repository.
func (s *Server) CommitSubmodule(ctx context.Context, path, url, commitHash, msg string) error {
	if url != "" {
		if err := s.appendGitmodules(ctx, path, url); err != nil {
			return err
		}
	}

	// git update-index --cacheinfo records the gitlink (mode 160000) without
	// requiring the submodule to be checked out locally.
	cacheinfo := fmt.Sprintf("160000,%s,%s", commitHash, path)

	if err := s.gitIn(ctx, s.workDir, "update-index", "--add", "--cacheinfo", cacheinfo); err != nil {
		return fmt.Errorf("update-index for submodule %s: %w", path, err)
	}

	if err := s.gitIn(ctx, s.workDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.push(ctx)
}

// CommitEmpty creates a commit with no file changes. Useful for seeding an
// initial commit before any content exists.
func (s *Server) CommitEmpty(ctx context.Context, msg string) error {
	if err := s.gitIn(ctx, s.workDir, "commit", "--allow-empty", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.push(ctx)
}

// CommitFiles writes a batch of files and commits them in a single commit.
// The files map keys are slash-separated paths relative to the repo root.
func (s *Server) CommitFiles(ctx context.Context, files map[string][]byte, msg string) error {
	for path, data := range files {
		fullPath := filepath.Join(s.workDir, filepath.FromSlash(path))

		if err := os.MkdirAll(filepath.Dir(fullPath), serverDirMode); err != nil {
			return fmt.Errorf("mkdir for %s: %w", path, err)
		}

		if err := os.WriteFile(fullPath, data, serverFileMode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	if err := s.gitIn(ctx, s.workDir, "add", "-A"); err != nil {
		return fmt.Errorf("add all: %w", err)
	}

	if err := s.gitIn(ctx, s.workDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return s.push(ctx)
}

// Head returns the canonical hash of the current HEAD commit.
func (s *Server) Head(ctx context.Context) (string, error) {
	return s.gitOut(ctx, s.workDir, "rev-parse", "HEAD")
}

// Tag creates a lightweight tag at the current HEAD with the given name.
func (s *Server) Tag(ctx context.Context, name string) error {
	head, err := s.Head(ctx)
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}

	return s.gitIn(ctx, s.bareDir, "tag", name, head)
}

// DeleteTag removes the named tag. It is a no-op when the tag does not exist.
func (s *Server) DeleteTag(ctx context.Context, name string) error {
	// `tag -l` exits zero either way; an empty listing is the no-op case.
	tags, err := s.gitOut(ctx, s.bareDir, "tag", "-l", name)
	if err != nil {
		return err
	}

	if tags == "" {
		return nil
	}

	return s.gitIn(ctx, s.bareDir, "tag", "-d", name)
}

// Branch creates a branch at the current HEAD with the given name.
func (s *Server) Branch(ctx context.Context, name string) error {
	head, err := s.Head(ctx)
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}

	return s.gitIn(ctx, s.bareDir, "update-ref", "refs/heads/"+name, head)
}

// SetBranch points the named branch at the given commit hash, creating it if
// missing. Tests use this to rewind a branch past a tagged commit so the
// commit is reachable only via the tag.
//
// Note: all Commit* methods push only to refs/heads/main. Calling SetBranch
// for a branch other than main, then calling CommitFile or similar, will not
// advance that branch — it will only advance main.
func (s *Server) SetBranch(ctx context.Context, name, hash string) error {
	return s.gitIn(ctx, s.bareDir, "update-ref", "refs/heads/"+name, hash)
}

// Start begins serving Git HTTP on a random local port and returns the full
// repository URL (e.g. "http://127.0.0.1:12345/repo.git"). The server maps
// any URL path to the main repo — unknown path components are rewritten to the
// real bare-repo directory name — while mount paths are served from their own
// bare directories. Use [Server.BaseURL] to construct URLs with an arbitrary
// path component (e.g. for relative submodule URL tests).
func (s *Server) Start(ctx context.Context) (string, error) {
	// Create symlinks in projectRoot so git-http-backend (GIT_PROJECT_ROOT mode)
	// can resolve mounted repos by path component.
	for _, m := range s.mounts {
		target := filepath.Join(s.projectRoot, m.path)

		if err := os.Symlink(m.bareDir, target); err != nil {
			return "", fmt.Errorf("mount %s: %w", m.path, err)
		}
	}

	var lc net.ListenConfig

	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	s.ln = ln
	s.srv = &http.Server{Handler: &routingHandler{
		gitPath:     s.gitPath,
		projectRoot: s.projectRoot,
		repoName:    filepath.Base(s.bareDir),
		mounts:      s.mounts,
	}}

	go func() { _ = s.srv.Serve(ln) }()

	return "http://" + ln.Addr().String() + "/" + filepath.Base(s.bareDir), nil
}

// BaseURL returns the scheme and authority (host:port) of the server with no
// repository path, e.g. "http://127.0.0.1:12345". Call this after [Server.Start]
// when you need to construct a URL with a specific path component that differs
// from the default "/repo.git" returned by [Server.Start].
func (s *Server) BaseURL() string {
	if s.ln == nil {
		return ""
	}

	return "http://" + s.ln.Addr().String()
}

// routingHandler wraps git-http-backend to make the server path-agnostic for
// the main repository while still routing mount paths to their own bare dirs.
//
// git-http-backend (GIT_PROJECT_ROOT mode) derives the repo from the leading
// path component of PATH_INFO. This handler rewrites PATH_INFO so that any
// unrecognised leading component is replaced with the real repo directory name,
// preserving the old go-git behaviour where any URL path served the main repo.
type routingHandler struct {
	gitPath     string
	projectRoot string
	repoName    string // basename of the main bare dir, e.g. "repo.git"
	mounts      []serverMount
}

func (h *routingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Extract the leading path component (the virtual repo name).
	var virtualName, rest string
	if idx := strings.Index(path[1:], "/"); idx >= 0 {
		virtualName = path[1 : 1+idx]
		rest = path[1+idx:]
	} else {
		virtualName = path[1:]
		rest = "/"
	}

	// If the virtual name doesn't match the real repo name or any mount,
	// rewrite it to the real repo name so git-http-backend finds the repo.
	if virtualName != h.repoName {
		known := false

		for _, m := range h.mounts {
			if virtualName == m.path {
				known = true

				break
			}
		}

		if !known {
			path = "/" + h.repoName + rest
		}
	}

	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path = path
	r2.URL = &u

	(&cgi.Handler{
		Path: h.gitPath,
		Args: []string{"http-backend"},
		Env: []string{
			"GIT_PROJECT_ROOT=" + h.projectRoot,
			"GIT_HTTP_EXPORT_ALL=1",
			// Suppress host git config for the same reason gitCommand
			// does: upload-pack behavior must not vary with developer
			// settings.
			"GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_CONFIG_NOSYSTEM=1",
		},
	}).ServeHTTP(w, r2)
}

// Close shuts down the server and removes its temporary directory.
func (s *Server) Close() (retErr error) {
	if s.srv != nil {
		// http.Server.Close closes the listener; don't close s.ln separately.
		if err := s.srv.Close(); err != nil {
			retErr = fmt.Errorf("close http server: %w", err)
		}
	}

	if err := os.RemoveAll(s.projectRoot); err != nil && retErr == nil {
		retErr = fmt.Errorf("remove temp dir: %w", err)
	}

	return retErr
}

// push pushes the current workDir HEAD to refs/heads/main in the bare repo.
// All Server commits land on main.
func (s *Server) push(ctx context.Context) error {
	return s.gitIn(ctx, s.workDir, "push", "origin", "HEAD:refs/heads/main")
}

// gitIn runs a git command with Dir set to dir, returning a formatted error
// that includes the combined output on failure.
func (s *Server) gitIn(ctx context.Context, dir string, args ...string) error {
	out, err := s.gitCommand(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return nil
}

// gitOut runs a git command like [Server.gitIn] but returns its trimmed
// stdout.
func (s *Server) gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	var stdout, stderr strings.Builder

	cmd := s.gitCommand(ctx, dir, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// gitCommand builds a git command rooted at dir with the host's global and
// system git config suppressed, so server behavior cannot vary with developer
// settings such as init.defaultBranch, commit.gpgsign, or core.hooksPath.
func (s *Server) gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, s.gitPath, args...)
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")

	return cmd
}

// appendGitmodules appends a submodule section for path and url to
// .gitmodules and stages the file.
func (s *Server) appendGitmodules(ctx context.Context, path, url string) error {
	gmPath := filepath.Join(s.workDir, ".gitmodules")

	existing, err := os.ReadFile(gmPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitmodules: %w", err)
	}

	section := fmt.Sprintf("[submodule %q]\n\tpath = %s\n\turl = %s\n", path, path, url)

	if err := os.WriteFile(gmPath, append(existing, section...), serverFileMode); err != nil {
		return fmt.Errorf("write .gitmodules: %w", err)
	}

	if err := s.gitIn(ctx, s.workDir, "add", ".gitmodules"); err != nil {
		return fmt.Errorf("add .gitmodules: %w", err)
	}

	return nil
}

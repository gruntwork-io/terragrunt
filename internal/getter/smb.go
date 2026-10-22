package getter

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrSMBPath is returned when an smb source lacks a host, a share or a path:
// smb://[<user>[:<password>]@]<host>/<share>/<path>.
var ErrSMBPath = errors.New(
	"smb source must include a host, a share and a path: smb://[<user>[:<password>]@]<host>/<share>/<path>",
)

// ErrSMBNotFound is returned when the smb source path does not exist on the
// share.
var ErrSMBNotFound = errors.New("smb source path not found")

// ErrSMBWrongKind is returned when an smb source is a file and the download
// needs a directory, or the reverse.
var ErrSMBWrongKind = errors.New("smb source path is the wrong kind")

// ErrUnsafeSMBPath is returned when an smb path or destination contains a
// character that would end smbclient's command and start another.
var ErrUnsafeSMBPath = errors.New("path contains a character that separates smbclient commands")

const (
	// smbDestinationPerms is the mode of a destination directory smbclient
	// downloads into.
	smbDestinationPerms = 0o755

	// smbclientSeparators end one smbclient -c command and start the next,
	// which could be a ! shell escape.
	smbclientSeparators = ";\r\n"

	// smbPasswordEnv is the variable smbclient reads a password from.
	smbPasswordEnv = "PASSWD"
)

// SmbClientGetter is Terragrunt's getter for smb sources fetched with the
// smbclient program.
//
// It downloads the way go-getter's smbclient getter does, but it runs
// smbclient through the venv's exec and reaches the filesystem through the
// venv's FS.
type SmbClientGetter struct {
	v        *venv.Venv
	detector *getter.SmbClientGetter
}

// NewSmbClientGetter returns an SmbClientGetter that runs smbclient through v.
func NewSmbClientGetter(v *venv.Venv) *SmbClientGetter {
	v.RequireExec()
	v.RequireFS()
	v.RequireEnv()

	return &SmbClientGetter{v: v, detector: new(getter.SmbClientGetter)}
}

// Detect reports whether req's source is an smb address.
func (g *SmbClientGetter) Detect(req *getter.Request) (bool, error) {
	return g.detector.Detect(req)
}

// Mode asks the share whether the path in u is a directory or a file.
func (g *SmbClientGetter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	share, dir, name, err := splitSMBFile(u)
	if err != nil {
		return 0, err
	}

	isDir, err := g.isDirectory(ctx, u, share, dir, name)
	if err != nil {
		return 0, err
	}

	if isDir {
		return getter.ModeDir, nil
	}

	return getter.ModeFile, nil
}

// Get downloads the directory at req's URL, and everything under it, into
// req.Dst. When the download fails, Get removes req.Dst if it did not exist
// before the call.
func (g *SmbClientGetter) Get(ctx context.Context, req *getter.Request) error {
	u := req.URL()

	share, dir, err := splitSMBPath(u)
	if err != nil {
		return err
	}

	existed := g.exists(req.Dst)

	err = g.getDir(ctx, u, share, dir, req.Dst)
	if err == nil {
		return nil
	}

	return errors.Join(err, removeIfCreated(g.v.FS, req.Dst, existed))
}

// GetFile downloads the file at req's URL to req.Dst. When the download
// fails, GetFile removes req.Dst if it did not exist before the call.
func (g *SmbClientGetter) GetFile(ctx context.Context, req *getter.Request) error {
	u := req.URL()

	share, dir, name, err := splitSMBFile(u)
	if err != nil {
		return err
	}

	existed := g.exists(req.Dst)

	err = g.getFile(ctx, u, share, dir, name, req.Dst)
	if err == nil {
		return nil
	}

	return errors.Join(err, removeIfCreated(g.v.FS, req.Dst, existed))
}

func (g *SmbClientGetter) getDir(ctx context.Context, u *url.URL, share, dir, dst string) error {
	isDir, err := g.isDirectory(ctx, u, share, ".", dir)
	if err != nil {
		return err
	}

	if !isDir {
		return fmt.Errorf("%w: %s must be a directory", ErrSMBWrongKind, dir)
	}

	if dst != "" {
		if err := g.v.FS.MkdirAll(dst, smbDestinationPerms); err != nil {
			return fmt.Errorf("creating destination %s: %w", dst, err)
		}
	}

	_, err = g.smbclient(ctx, u, share, dir, dst, "prompt OFF;recurse ON; mget *")

	return err
}

func (g *SmbClientGetter) getFile(ctx context.Context, u *url.URL, share, dir, name, dst string) error {
	isDir, err := g.isDirectory(ctx, u, share, dir, name)
	if err != nil {
		return err
	}

	if isDir {
		return fmt.Errorf("%w: %s must be a file", ErrSMBWrongKind, name)
	}

	if err := requireSMBCommandSafe(dst); err != nil {
		return err
	}

	command := "get " + name

	if dst != "" {
		if err := g.v.FS.MkdirAll(filepath.Dir(dst), smbDestinationPerms); err != nil {
			return fmt.Errorf("creating destination %s: %w", filepath.Dir(dst), err)
		}

		command += " " + dst
	}

	_, err = g.smbclient(ctx, u, share, dir, "", command)

	return err
}

// isDirectory asks the share whether name, under dir, is a directory.
func (g *SmbClientGetter) isDirectory(ctx context.Context, u *url.URL, share, dir, name string) (bool, error) {
	if err := requireSMBCommandSafe(name); err != nil {
		return false, err
	}

	out, err := g.smbclient(ctx, u, share, dir, "", "allinfo "+name)
	if err != nil {
		return false, err
	}

	if strings.Contains(out, "OBJECT_NAME_NOT_FOUND") {
		return false, fmt.Errorf("%w: %s", ErrSMBNotFound, name)
	}

	return strings.Contains(out, "attributes: D"), nil
}

// exists reports whether path exists, without following a symlink at it.
func (g *SmbClientGetter) exists(path string) bool {
	if path == "" {
		return false
	}

	_, err := vfs.Lstat(g.v.FS, path)

	return err == nil
}

// requireSMBCommandSafe refuses s when it contains a character that would end
// the smbclient command around it.
func requireSMBCommandSafe(s string) error {
	if strings.ContainsAny(s, smbclientSeparators) {
		return fmt.Errorf("%w: %q", ErrUnsafeSMBPath, s)
	}

	return nil
}

// smbclient runs command from workDir through smbclient, which opens share on
// the port in u, if any, changes into dir, and authenticates as the user in u,
// if any. It returns smbclient's combined output.
//
// The password in u reaches smbclient through the PASSWD variable and never
// appears in the command's arguments.
func (g *SmbClientGetter) smbclient(
	ctx context.Context,
	u *url.URL,
	share, dir, workDir, command string,
) (string, error) {
	v := g.v
	user := u.User.Username()
	password, _ := u.User.Password()

	args := []string{share, "--directory", dir, "-c", command}
	if port := u.Port(); port != "" {
		args = append([]string{"-p", port}, args...)
	}

	switch {
	case user == "":
		args = append([]string{"-N"}, args...)
	case password == "":
		args = append([]string{"-N", "-U", user}, args...)
	default:
		env := maps.Clone(v.Env)
		env[smbPasswordEnv] = password
		v = v.WithEnv(env)

		args = append([]string{"-U", user}, args...)
	}

	return runDownloadCommand(ctx, v, workDir, "smbclient", args...)
}

// splitSMBPath splits u into the //host/share smbclient opens and the path
// inside the share.
func splitSMBPath(u *url.URL) (string, string, error) {
	if u.Host == "" || u.Path == "" {
		return "", "", ErrSMBPath
	}

	share, rest, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	if rest == "" {
		return "", "", ErrSMBPath
	}

	return "//" + u.Hostname() + "/" + share, rest, nil
}

// splitSMBFile splits u into the //host/share smbclient opens, the directory
// inside the share, and the name in that directory.
func splitSMBFile(u *url.URL) (string, string, string, error) {
	share, rest, err := splitSMBPath(u)
	if err != nil {
		return "", "", "", err
	}

	dir, name, ok := strings.CutLast(rest, "/")
	if !ok {
		return share, ".", rest, nil
	}

	return share, dir, name, nil
}

// SmbMountGetter is Terragrunt's getter for smb sources reached through a
// share the host has mounted, under //host on Windows and /Volumes on macOS.
//
// It copies from the mount with the client's [FileCopyGetter].
type SmbMountGetter struct {
	file     *FileCopyGetter
	detector *getter.SmbClientGetter
	goos     string
}

// NewSmbMountGetter returns an SmbMountGetter that finds mounts where v's
// platform puts them and copies from them with file.
func NewSmbMountGetter(v *venv.Venv, file *FileCopyGetter) *SmbMountGetter {
	v.RequireGOOS()

	return &SmbMountGetter{file: file, detector: new(getter.SmbClientGetter), goos: v.Platform.GOOS}
}

// Detect reports whether req's source is an smb address and the venv's
// platform mounts smb shares at a known path.
func (g *SmbMountGetter) Detect(req *getter.Request) (bool, error) {
	if g.goos != goosWindows && g.goos != goosDarwin {
		return false, nil
	}

	return g.detector.Detect(req)
}

// Mode reports whether the mounted path in u is a directory or a file.
func (g *SmbMountGetter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	mounted := *u

	if err := g.mount(&mounted); err != nil {
		return 0, err
	}

	return g.file.Mode(ctx, &mounted)
}

// GetFile copies the mounted file at req's URL to req.Dst.
func (g *SmbMountGetter) GetFile(ctx context.Context, req *getter.Request) error {
	if err := g.mount(req.URL()); err != nil {
		return err
	}

	return g.file.GetFile(ctx, req)
}

// Get copies the mounted directory at req's URL into req.Dst.
func (g *SmbMountGetter) Get(ctx context.Context, req *getter.Request) error {
	if err := g.mount(req.URL()); err != nil {
		return err
	}

	return g.file.Get(ctx, req)
}

// mount sets u's path to the local path where the host mounts u's share.
func (g *SmbMountGetter) mount(u *url.URL) error {
	if u.Host == "" || u.Path == "" {
		return ErrSMBPath
	}

	u.Path = smbMountPath(g.goos, u)
	u.RawPath = ""

	return nil
}

// smbMountPath returns the local path where goos mounts u's share, or an
// empty path on a platform that has no known mount point.
func smbMountPath(goos string, u *url.URL) string {
	switch goos {
	case goosWindows:
		return `\\` + filepath.Join(u.Host, u.Path)
	case goosDarwin:
		return "/" + filepath.Join("Volumes", u.Path)
	default:
		return ""
	}
}

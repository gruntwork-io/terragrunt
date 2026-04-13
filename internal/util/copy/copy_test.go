package copy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util/copy"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/createmount"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/ts"
)

func TestCopyOnDarwinWithinAPFS(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	apfsMount := t.TempDir()
	createmount.MountDiskImageMacOS(t, apfsMount, "APFS")

	fileName := filepath.Join(apfsMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(filepath.Dir(fileName), toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()
	toPerms := ts.NoErr(os.Stat(filepath.Join(filepath.Dir(fileName), toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

func TestCopyOnDarwinAcrossAPFS(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	apfsMount1 := t.TempDir()
	apfsMount2 := t.TempDir()

	createmount.MountDiskImageMacOS(t, apfsMount1, "APFS")
	createmount.MountDiskImageMacOS(t, apfsMount2, "APFS")

	fileName := filepath.Join(apfsMount1, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(filepath.Dir(fileName), toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()
	toPerms := ts.NoErr(os.Stat(filepath.Join(filepath.Dir(fileName), toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

func TestCopyOnDarwinWithinExFAT(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	exfatMount := t.TempDir()
	createmount.MountDiskImageMacOS(t, exfatMount, "ExFAT")

	fileName := filepath.Join(exfatMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(filepath.Dir(fileName), toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()
	toPerms := ts.NoErr(os.Stat(filepath.Join(filepath.Dir(fileName), toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

func TestCopyOnLinuxWithinXFS(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	xfsMount := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount, "xfs")

	fileName := filepath.Join(xfsMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(filepath.Dir(fileName), toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()
	toPerms := ts.NoErr(os.Stat(filepath.Join(filepath.Dir(fileName), toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

func TestCopyOnLinuxAcrossXFS(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	xfsMount1 := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount1, "xfs")
	xfsMount2 := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount2, "xfs")

	fileName := filepath.Join(xfsMount1, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(filepath.Dir(fileName), toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()
	toPerms := ts.NoErr(os.Stat(filepath.Join(filepath.Dir(fileName), toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

func TestCopyOnLinuxWithinEXT4(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	ext4Mount := t.TempDir()
	createmount.MountDiskImageLinux(t, ext4Mount, "ext4")

	fileName := filepath.Join(ext4Mount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	toName := "test-reflink.txt"

	ts.NoErr(0, copy.Copy(fileName, filepath.Join(ext4Mount, toName)))(t)

	fromPerms := ts.NoErr(os.Stat(fileName))(t).Mode()

	toPerms := ts.NoErr(os.Stat(filepath.Join(ext4Mount, toName)))(t).Mode()
	ts.Is(fromPerms)(t, toPerms)
}

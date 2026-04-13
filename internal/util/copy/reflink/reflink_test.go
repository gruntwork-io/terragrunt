package reflink_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/createmount"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/ts"
)

func TestReflinkOnDarwinWithinAPFS(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	apfsMount := t.TempDir()
	createmount.MountDiskImageMacOS(t, apfsMount, "APFS")

	fileName := filepath.Join(apfsMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(apfsMount))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile := ts.NoErr(reflink.Reflink(fromFD, toDirFD, toName))(t)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}
}

func TestReflinkOnDarwinAcrossAPFS(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	apfsMount1 := t.TempDir()
	apfsMount2 := t.TempDir()

	createmount.MountDiskImageMacOS(t, apfsMount1, "APFS")
	createmount.MountDiskImageMacOS(t, apfsMount2, "APFS")

	fileName := filepath.Join(apfsMount1, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(apfsMount2))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile, err := reflink.Reflink(fromFD, toDirFD, toName)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}

	ts.IsErr(t, err, reflink.ErrCanNotReflink{})
}

func TestReflinkOnDarwinWithinExFAT(t *testing.T) {
	ts.OnlyOn(t, "darwin_")
	t.Parallel()

	exfatMount := t.TempDir()
	createmount.MountDiskImageMacOS(t, exfatMount, "ExFAT")

	fileName := filepath.Join(exfatMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(exfatMount))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile, err := reflink.Reflink(fromFD, toDirFD, toName)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}

	ts.IsErr(t, err, reflink.ErrCanNotReflink{})
}

func TestReflinkOnLinuxWithinXFS(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	xfsMount := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount, "xfs")

	fileName := filepath.Join(xfsMount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(xfsMount))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile := ts.NoErr(reflink.Reflink(fromFD, toDirFD, toName))(t)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}
}

func TestReflinkOnLinuxAcrossXFS(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	xfsMount1 := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount1, "xfs")
	xfsMount2 := t.TempDir()
	createmount.MountDiskImageLinux(t, xfsMount2, "xfs")

	fileName := filepath.Join(xfsMount1, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(xfsMount2))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile, err := reflink.Reflink(fromFD, toDirFD, toName)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}

	ts.IsErr(t, err, reflink.ErrCanNotReflink{})
}

func TestReflinkOnLinuxWithinEXT4(t *testing.T) {
	ts.OnlyOn(t, "linux_")
	t.Parallel()

	ext4Mount := t.TempDir()
	createmount.MountDiskImageLinux(t, ext4Mount, "ext4")

	fileName := filepath.Join(ext4Mount, "test.txt")

	ts.NoErr(0, os.WriteFile(fileName, []byte("Hello, World!"), 0o644))(t)

	fromFD := ts.NoErr(os.Open(fileName))(t)
	toDirFD := ts.NoErr(os.Open(ext4Mount))(t)

	defer fromFD.Close()  // nolint:errcheck
	defer toDirFD.Close() // nolint:errcheck

	toName := "test-reflink.txt"

	toFile, err := reflink.Reflink(fromFD, toDirFD, toName)
	if toFile != nil {
		defer toFile.Close() // nolint:errcheck
	}

	ts.IsErr(t, err, reflink.ErrCanNotReflink{})
}

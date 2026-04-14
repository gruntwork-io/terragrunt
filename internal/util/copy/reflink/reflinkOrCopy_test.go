package reflink_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/createmount"
	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/ts"
)

const reflinkedFileName = "test-reflink.txt"

func TestReflinkOrCopyOnDarwinWithinAPFS(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(true)(t, didReflink)
}

func TestReflinkOrCopyOnDarwinAcrossAPFS(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(false)(t, didReflink)
}

func TestReflinkOrCopyOnDarwinWithinExFAT(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(false)(t, didReflink)
}

func TestReflinkOrCopyOnLinuxWithinXFS(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(true)(t, didReflink)
}

func TestReflinkOrCopyOnLinuxAcrossXFS(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(false)(t, didReflink)
}

func TestReflinkOrCopyOnLinuxWithinEXT4(t *testing.T) {
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

	toName := reflinkedFileName

	didReflink := ts.NoErr(reflink.ReflinkOrCopy(fromFD, toDirFD, toName))(t)
	ts.Is(false)(t, didReflink)
}

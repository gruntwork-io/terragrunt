package fixture_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCreatesOneDirectoryPerUnit(t *testing.T) {
	t.Parallel()

	for _, shape := range fixture.Shapes() {
		t.Run(string(shape), func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "fixture")

			require.NoError(t, fixture.Write(dir, shape, 6))

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)

			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}

			assert.ElementsMatch(
				t,
				[]string{"root.hcl", "unit-0", "unit-1", "unit-2", "unit-3", "unit-4", "unit-5"},
				names,
			)

			for index := range 6 {
				assert.FileExists(t, filepath.Join(dir, fixture.UnitName(index), "terragrunt.hcl"))
				assert.FileExists(t, filepath.Join(dir, fixture.UnitName(index), "main.tf"))
			}
		})
	}
}

func TestWriteDepsPointsEachUnitAtItsParent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, fixture.Write(dir, fixture.ShapeDeps, 6))

	parents := map[int]string{1: "../unit-0", 4: "../unit-0", 5: "../unit-1"}

	for index, parent := range parents {
		config, err := os.ReadFile(filepath.Join(dir, fixture.UnitName(index), "terragrunt.hcl"))
		require.NoError(t, err)

		assert.Contains(t, string(config), `config_path = "`+parent+`"`)
	}

	first, err := os.ReadFile(filepath.Join(dir, fixture.UnitName(0), "terragrunt.hcl"))
	require.NoError(t, err)

	assert.NotContains(t, string(first), "dependency")
}

func TestWriteRejectsBadInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		shape   fixture.Shape
		units   int
	}{
		{name: "zero units", shape: fixture.ShapeFlat, units: 0, wantErr: fixture.ErrUnitCount},
		{name: "too many units", shape: fixture.ShapeFlat, units: fixture.MaxUnits + 1, wantErr: fixture.ErrUnitCount},
		{name: "unknown shape", shape: fixture.Shape("ring"), units: 1, wantErr: fixture.ErrUnknownShape},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := fixture.Write(t.TempDir(), tc.shape, tc.units)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestWriteDatesEveryFileFileAgeBack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, fixture.Write(dir, fixture.ShapeDeps, 3))

	latest := time.Now().Add(-fixture.FileAge)

	var files int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files++

		assert.False(t, info.ModTime().After(latest), path)

		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 7, files)
}

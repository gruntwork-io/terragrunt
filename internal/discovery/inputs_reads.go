package discovery

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"sync"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
)

// inputsReads is the set of file reads found in inputs attributes, keyed by position because
// every unit that includes the same root file reports that file's reads again, and every
// element of an expanded unit shares one autoinclude block.
type inputsReads struct {
	seen  map[inputsReadKey]struct{}
	reads []tips.InputsRead
	mu    sync.Mutex
}

// inputsReadKey is the position of a read: its file and byte offset.
type inputsReadKey struct {
	file string
	byte int
}

// add records read unless one at the same position of the same file is already recorded.
func (r *inputsReads) add(read *tips.InputsRead) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.seen == nil {
		r.seen = make(map[inputsReadKey]struct{})
	}

	key := inputsReadKey{file: read.File, byte: read.Range.Start.Byte}
	if _, ok := r.seen[key]; ok {
		return
	}

	r.seen[key] = struct{}{}
	r.reads = append(r.reads, *read)
}

// list returns the recorded reads ordered by file, then position.
func (r *inputsReads) list() []tips.InputsRead {
	r.mu.Lock()
	defer r.mu.Unlock()

	reads := slices.Clone(r.reads)
	slices.SortFunc(reads, func(a, b tips.InputsRead) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte))
	})

	return reads
}

// InputsReads returns the calls to file-reading functions that parsing found in inputs
// attributes, ordered by file then position, which [Discovery.Discover] reports with the
// inputs-read-not-tracked tip. It is empty unless reads were tracked.
func (d *Discovery) InputsReads() []tips.InputsRead {
	return d.inputsReads.list()
}

// recordUnitInputsReads records the reads in the inputs attribute of the unit config at cfgPath and
// of every file it includes. A file that fails to parse is skipped with a debug log.
func (d *Discovery) recordUnitInputsReads(
	l log.Logger,
	fsys vfs.FS,
	cfgPath string,
	cfg *config.TerragruntConfig,
) {
	paths := make([]string, 0, 1+len(cfg.ProcessedIncludes))
	paths = append(paths, cfgPath)

	for _, include := range cfg.ProcessedIncludes {
		paths = append(paths, include.Path)
	}

	for _, path := range paths {
		file, err := hclparse.NewParser(hclparse.WithLogger(l)).ParseFromFile(fsys, path)
		if err != nil {
			l.Debugf("Discovery: skipping inputs read scan of %s: %v", path, err)

			continue
		}

		d.recordInputsReads(l, file.Body, file.Bytes, tips.InputsReadInUnit, func(string) string {
			return d.displayPath(path)
		})
	}
}

// recordAutoIncludeInputsReads records the reads in the inputs attribute of each autoinclude block
// in autoIncludes, as [config.StackAutoIncludes] resolved them for the stack file at stackFilePath.
// The stack parser names the stack file by its basename in ranges, so a relative range filename is
// resolved against the stack directory.
func (d *Discovery) recordAutoIncludeInputsReads(
	l log.Logger,
	stackFilePath string,
	autoIncludes map[string]*inthclparse.AutoIncludeResolved,
) {
	stackDir := filepath.Dir(stackFilePath)

	for _, key := range slices.Sorted(maps.Keys(autoIncludes)) {
		resolved := autoIncludes[key]

		d.recordInputsReads(
			l,
			resolved.RawBody,
			resolved.SourceBytes,
			tips.InputsReadInAutoInclude,
			func(rangeFilename string) string {
				if filepath.IsAbs(rangeFilename) {
					return d.displayPath(rangeFilename)
				}

				return d.displayPath(filepath.Join(stackDir, rangeFilename))
			},
		)
	}
}

// recordInputsReads records the reads in the inputs attribute of body, which was parsed from
// source. fileOf spells the file a read's range names as the tip prints it.
func (d *Discovery) recordInputsReads(
	l log.Logger,
	body hcl.Body,
	source []byte,
	place tips.InputsReadPlace,
	fileOf func(rangeFilename string) string,
) {
	reads, diags := config.FindInputsReads(body)
	if diags.HasErrors() {
		l.Debugf("Discovery: inputs read scan reported: %v", diags)
	}

	for _, read := range reads {
		if place == tips.InputsReadInAutoInclude && read.MarkedAsRead {
			continue
		}

		file := fileOf(read.Range.Filename)

		rng := read.Range
		rng.Filename = file

		d.inputsReads.add(&tips.InputsRead{
			File:     file,
			Function: read.Function,
			Source:   source,
			Range:    rng,
			Place:    place,
		})
	}
}

// displayPath spells path relative to the working directory when it can, as the tip prints it.
func (d *Discovery) displayPath(path string) string {
	rel, err := filepath.Rel(d.workingDir, path)
	if err != nil {
		return path
	}

	return rel
}

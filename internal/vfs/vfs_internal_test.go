package vfs

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyVolumeLink only runs for a link target carrying a volume name, which
// exists on Windows alone, so it is driven directly here to keep it checked on
// every platform.
func TestSymlinkWalkStateApplyVolumeLink(t *testing.T) {
	t.Parallel()

	sep := string(os.PathSeparator)

	testCases := []struct {
		name       string
		link       string
		wantVol    string
		linkVolLen int
	}{
		{
			name:       "separator after the volume belongs to it",
			link:       "vol" + sep + "rest",
			linkVolLen: len("vol"),
			wantVol:    "vol" + sep,
		},
		{
			name:       "volume without a separator",
			link:       "volrest",
			linkVolLen: len("vol"),
			wantVol:    "vol",
		},
		{
			name:       "link that is only a volume",
			link:       "vol",
			linkVolLen: len("vol"),
			wantVol:    "vol",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := newSymlinkWalkState(sep + "start")

			state.applyVolumeLink(tc.link, tc.linkVolLen)

			assert.Equal(t, tc.wantVol, state.vol)
			assert.Equal(t, tc.wantVol, state.dest)
			assert.Equal(t, len(tc.wantVol), state.start)
			assert.Equal(t, len(tc.wantVol), state.volLen)
		})
	}
}

func TestLimitedReaderAtLimit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reader  io.Reader
		wantErr error
		name    string
	}{
		{
			name:    "underlying reader returning nothing without an error ends the stream",
			reader:  emptyReader{},
			wantErr: io.EOF,
		},
		{
			name:    "underlying reader at its end ends the stream",
			reader:  eofReader{},
			wantErr: io.EOF,
		},
		{
			name:    "underlying reader failure is passed through",
			reader:  failingReader{},
			wantErr: errReaderFailed,
		},
		{
			name:    "underlying reader with data left breaches the limit",
			reader:  oneByteReader{},
			wantErr: ZipDecompressedSizeLimitError{Name: "entry", Size: 10, Limit: 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &limitedReader{reader: tc.reader, remaining: 0, name: "entry", size: 10, limit: 5}

			n, err := r.Read(make([]byte, 8))
			require.ErrorIs(t, err, tc.wantErr)
			assert.Zero(t, n)
		})
	}
}

// errReaderFailed is what failingReader reports.
var errReaderFailed = errors.New("read failed")

// emptyReader returns no data and no error, which io.Reader permits.
type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, nil }

// eofReader is always at its end.
type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

// failingReader always fails.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errReaderFailed }

// oneByteReader always has one more byte.
type oneByteReader struct{}

func (oneByteReader) Read(p []byte) (int, error) {
	p[0] = 'x'

	return 1, nil
}

package legacygzip

import (
	"cmp"
	"errors"
	"io"
	"math"
	"math/bits"
	"slices"
)

const (
	logWindowSize = 15
	windowSize    = 1 << logWindowSize
	windowMask    = windowSize - 1

	// The LZ77 step produces a sequence of literal tokens and <length, offset>
	// pair tokens. The offset is also known as distance. The underlying wire
	// format limits the range of lengths and offsets. For example, there are
	// 256 legitimate lengths: those in the range [3, 258]. This package's
	// compressor uses a higher minimum match length, enabling optimizations
	// such as finding matches via 32-bit loads and compares.
	baseMatchLength = 3       // The smallest match length per the RFC section 3.2.5
	minMatchLength  = 4       // The smallest match length that the compressor actually emits
	maxMatchLength  = 258     // The largest match length
	baseMatchOffset = 1       // The smallest match offset
	maxMatchOffset  = 1 << 15 // The largest match offset

	// The maximum number of tokens we put into a single block, just to
	// stop things from getting too large.
	maxFlateBlockTokens = 1 << 14
	maxStoreBlockSize   = 65535
	hashBits            = 17 // After 17 performance degrades
	hashSize            = 1 << hashBits
	hashMask            = (1 << hashBits) - 1
	maxHashOffset       = 1 << 24
	hashShift           = 32 - hashBits
	windowBufferSize    = 2 * windowSize

	// Lazy matching tuning of the default compression level.
	goodLength  = 8   // with a match at least this long, search only a quarter of the chain
	lazyLength  = 16  // do not look for a longer match once the current one is this long
	niceLength  = 128 // stop searching at a match this long
	chainLength = 128 // hash chain entries examined per position
)

// maxNumLit comes from RFC 1951 section 3.2.7.
const maxNumLit = 286

type compressor struct {
	err            error
	w              *huffmanBitWriter
	tokens         []token
	window         []byte
	index          int
	blockStart     int
	length         int
	offset         int
	maxInsertIndex int
	windowEnd      int
	chainHead      int
	hashOffset     int
	hashHead       [hashSize]uint32
	hashPrev       [windowSize]uint32
	byteAvailable  bool
	sync           bool
}

func (d *compressor) fillDeflate(b []byte) int {
	if d.index >= windowBufferSize-(minMatchLength+maxMatchLength) {
		// shift the window by windowSize
		copy(d.window, d.window[windowSize:windowBufferSize])
		d.index -= windowSize

		d.windowEnd -= windowSize
		if d.blockStart >= windowSize {
			d.blockStart -= windowSize
		} else {
			d.blockStart = math.MaxInt32
		}

		d.hashOffset += windowSize
		if d.hashOffset > maxHashOffset {
			delta := d.hashOffset - 1
			d.hashOffset -= delta
			d.chainHead -= delta

			// Iterate over slices instead of arrays to avoid copying
			// the entire table onto the stack.
			for i, v := range d.hashPrev[:] {
				if int(v) > delta {
					d.hashPrev[i] = uint32(int(v) - delta)
				} else {
					d.hashPrev[i] = 0
				}
			}

			for i, v := range d.hashHead[:] {
				if int(v) > delta {
					d.hashHead[i] = uint32(int(v) - delta)
				} else {
					d.hashHead[i] = 0
				}
			}
		}
	}

	n := copy(d.window[d.windowEnd:], b)
	d.windowEnd += n

	return n
}

func (d *compressor) writeBlock(tokens []token, index int) error {
	if index > 0 {
		var window []byte
		if d.blockStart <= index {
			window = d.window[d.blockStart:index]
		}

		d.blockStart = index
		d.w.writeBlock(tokens, false, window)

		return d.w.err
	}

	return nil
}

// Try to find a match starting at index whose length is greater than prevSize.
// We only look at chainCount possibilities before giving up.
func (d *compressor) findMatch(pos int, prevHead int, prevLength int, lookahead int) (length, offset int, ok bool) {
	minMatchLook := min(lookahead, maxMatchLength)

	win := d.window[0 : pos+minMatchLook]

	// We quit when we get a match that's at least nice long
	nice := min(niceLength, len(win)-pos)

	// If we've got a match that's good enough, only look in 1/4 the chain.
	tries := chainLength

	length = prevLength
	if length >= goodLength {
		tries >>= 2
	}

	wEnd := win[pos+length]
	wPos := win[pos:]
	minIndex := pos - windowSize

	for i := prevHead; tries > 0; tries-- {
		if wEnd == win[i+length] {
			n := matchLen(win[i:], wPos, minMatchLook)

			if n > length && (n > minMatchLength || pos-i <= 4096) {
				length = n
				offset = pos - i
				ok = true

				if n >= nice {
					// The match is good enough that we don't try to find a better one.
					break
				}

				wEnd = win[pos+n]
			}
		}

		if i == minIndex {
			// hashPrev[i & windowMask] has already been overwritten, so stop now.
			break
		}

		i = int(d.hashPrev[i&windowMask]) - d.hashOffset
		if i < minIndex || i < 0 {
			break
		}
	}

	return
}

const hashmul = 0x1e35a7bd

// hash4 returns a hash representation of the first 4 bytes
// of the supplied slice.
// The caller must ensure that len(b) >= 4.
func hash4(b []byte) uint32 {
	return ((uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24) * hashmul) >> hashShift
}

// matchLen returns the number of matching bytes in a and b
// up to length 'max'. Both slices must be at least 'max'
// bytes in size.
func matchLen(a, b []byte, max int) int {
	a = a[:max]

	b = b[:len(a)]
	for i, av := range a {
		if b[i] != av {
			return i
		}
	}

	return max
}

func (d *compressor) initDeflate() {
	d.window = make([]byte, windowBufferSize)
	d.hashOffset = 1
	d.tokens = make([]token, 0, maxFlateBlockTokens+1)
	d.length = minMatchLength - 1
	d.offset = 0
	d.byteAvailable = false
	d.index = 0
	d.chainHead = -1
}

func (d *compressor) deflate() {
	if d.windowEnd-d.index < minMatchLength+maxMatchLength && !d.sync {
		return
	}

	d.maxInsertIndex = d.windowEnd - (minMatchLength - 1)

Loop:
	for {
		if d.index > d.windowEnd {
			panic("index > windowEnd")
		}

		lookahead := d.windowEnd - d.index
		if lookahead < minMatchLength+maxMatchLength {
			if !d.sync {
				break Loop
			}

			if d.index > d.windowEnd {
				panic("index > windowEnd")
			}

			if lookahead == 0 {
				// Flush current output block if any.
				if d.byteAvailable {
					// There is still one pending token that needs to be flushed
					d.tokens = append(d.tokens, literalToken(uint32(d.window[d.index-1])))
					d.byteAvailable = false
				}

				if len(d.tokens) > 0 {
					if d.err = d.writeBlock(d.tokens, d.index); d.err != nil {
						return
					}

					d.tokens = d.tokens[:0]
				}

				break Loop
			}
		}

		if d.index < d.maxInsertIndex {
			// Update the hash
			hash := hash4(d.window[d.index : d.index+minMatchLength])
			hh := &d.hashHead[hash&hashMask]
			d.chainHead = int(*hh)
			d.hashPrev[d.index&windowMask] = uint32(d.chainHead)
			*hh = uint32(d.index + d.hashOffset)
		}

		prevLength := d.length
		prevOffset := d.offset
		d.length = minMatchLength - 1
		d.offset = 0
		minIndex := max(d.index-windowSize, 0)

		if d.chainHead-d.hashOffset >= minIndex && lookahead > prevLength && prevLength < lazyLength {
			if newLength, newOffset, ok := d.findMatch(d.index, d.chainHead-d.hashOffset, minMatchLength-1, lookahead); ok {
				d.length = newLength
				d.offset = newOffset
			}
		}

		if prevLength >= minMatchLength && d.length <= prevLength {
			// There was a match at the previous step, and the current match is
			// not better. Output the previous match.
			d.tokens = append(d.tokens, matchToken(uint32(prevLength-baseMatchLength), uint32(prevOffset-baseMatchOffset)))
			// Insert in the hash table all strings up to the end of the match.
			// index and index-1 are already inserted. If there is not enough
			// lookahead, the last two strings are not inserted into the hash
			// table.
			newIndex := d.index + prevLength - 1

			index := d.index
			for index++; index < newIndex; index++ {
				if index < d.maxInsertIndex {
					hash := hash4(d.window[index : index+minMatchLength])
					// Get previous value with the same hash.
					// Our chain should point to the previous value.
					hh := &d.hashHead[hash&hashMask]
					d.hashPrev[index&windowMask] = *hh
					// Set the head of the hash chain to us.
					*hh = uint32(index + d.hashOffset)
				}
			}

			d.index = index
			d.byteAvailable = false
			d.length = minMatchLength - 1

			if len(d.tokens) == maxFlateBlockTokens {
				// The block includes the current character
				if d.err = d.writeBlock(d.tokens, d.index); d.err != nil {
					return
				}

				d.tokens = d.tokens[:0]
			}
		} else {
			if d.byteAvailable {
				i := d.index - 1

				d.tokens = append(d.tokens, literalToken(uint32(d.window[i])))
				if len(d.tokens) == maxFlateBlockTokens {
					if d.err = d.writeBlock(d.tokens, i+1); d.err != nil {
						return
					}

					d.tokens = d.tokens[:0]
				}
			}

			d.index++
			d.byteAvailable = true
		}
	}
}

func (d *compressor) write(b []byte) (n int, err error) {
	if d.err != nil {
		return 0, d.err
	}

	n = len(b)
	for len(b) > 0 {
		d.deflate()

		b = b[d.fillDeflate(b):]
		if d.err != nil {
			return 0, d.err
		}
	}

	return n, nil
}

func (d *compressor) syncFlush() error {
	if d.err != nil {
		return d.err
	}

	d.sync = true
	d.deflate()

	if d.err == nil {
		d.w.writeStoredHeader(0, false)
		d.w.flush()
		d.err = d.w.err
	}

	d.sync = false

	return d.err
}

func (d *compressor) init(w io.Writer) {
	d.w = newHuffmanBitWriter(w)
	d.initDeflate()
}

func (d *compressor) close() error {
	if d.err != nil {
		return d.err
	}

	d.sync = true
	d.deflate()

	if d.err != nil {
		return d.err
	}

	if d.w.writeStoredHeader(0, true); d.w.err != nil {
		return d.w.err
	}

	d.w.flush()

	return d.w.err
}

const (
	// The largest offset code.
	offsetCodeCount = 30

	// The special code used to mark the end of a block.
	endBlockMarker = 256

	// The first length code.
	lengthCodesStart = 257

	// The number of codegen codes.
	codegenCodeCount = 19
	badCode          = 255

	// bufferFlushSize indicates the buffer size
	// after which bytes are flushed to the writer.
	// Should preferably be a multiple of 6, since
	// we accumulate 6 bytes between writes to the buffer.
	bufferFlushSize = 240

	// bufferSize is the actual output byte buffer size.
	// It must have additional headroom for a flush
	// which can contain up to 8 bytes.
	bufferSize = bufferFlushSize + 8

	byteBits   = 8
	flushBits  = 48 // buffered bits are written out six bytes at a time
	flushBytes = flushBits / byteBits

	// Block header layout, RFC 1951 section 3.2.
	blockTypeBits  = 3 // final-block flag plus the block type
	storedLenBits  = 16
	storedOverhead = 5 // stored block header byte plus LEN and NLEN
	hlitBits       = 5
	hdistBits      = 5
	hclenBits      = 4
	minCodegens    = 4
	codeLenBits    = 3
	maxCodeBits    = 15
	maxCodegenBits = 7
	uint16Bits     = 16

	// Code length repeat codes, RFC 1951 section 3.2.7.
	codeRepeatPrev           = 16 // repeat the previous code length repeatPrevMin to repeatPrevMax times
	codeRepeatZeroShort      = 17 // repeatZeroShortMin to ten zero code lengths
	codeRepeatZeroLong       = 18 // repeatZeroLongMin to repeatZeroLongMax zero code lengths
	repeatPrevMin            = 3
	repeatPrevMax            = 6
	repeatPrevExtraBits      = 2
	repeatZeroShortMin       = 3
	repeatZeroShortExtraBits = 3
	repeatZeroLongMin        = 11
	repeatZeroLongMax        = 138
	repeatZeroLongExtraBits  = 7

	// Leading length and offset codes that carry no extra bits.
	lengthCodesNoExtra = 8
	offsetCodesNoExtra = 4
	fixedOffsetBits    = 5
)

// The number of extra bits needed by length code X - LENGTH_CODES_START.
var lengthExtraBits = []int8{
	/* 257 */ 0, 0, 0,
	/* 260 */ 0, 0, 0, 0, 0, 1, 1, 1, 1, 2,
	/* 270 */ 2, 2, 2, 3, 3, 3, 3, 4, 4, 4,
	/* 280 */ 4, 5, 5, 5, 5, 0,
}

// The length indicated by length code X - LENGTH_CODES_START.
var lengthBase = []uint32{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 10,
	12, 14, 16, 20, 24, 28, 32, 40, 48, 56,
	64, 80, 96, 112, 128, 160, 192, 224, 255,
}

// offset code word extra bits.
var offsetExtraBits = []int8{
	0, 0, 0, 0, 1, 1, 2, 2, 3, 3,
	4, 4, 5, 5, 6, 6, 7, 7, 8, 8,
	9, 9, 10, 10, 11, 11, 12, 12, 13, 13,
}

var offsetBase = []uint32{
	0x000000, 0x000001, 0x000002, 0x000003, 0x000004,
	0x000006, 0x000008, 0x00000c, 0x000010, 0x000018,
	0x000020, 0x000030, 0x000040, 0x000060, 0x000080,
	0x0000c0, 0x000100, 0x000180, 0x000200, 0x000300,
	0x000400, 0x000600, 0x000800, 0x000c00, 0x001000,
	0x001800, 0x002000, 0x003000, 0x004000, 0x006000,
}

// The odd order in which the codegen code sizes are written.
var codegenOrder = []uint32{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

type huffmanBitWriter struct {
	writer          io.Writer
	err             error
	codegenEncoding *huffmanEncoder
	offsetEncoding  *huffmanEncoder
	literalEncoding *huffmanEncoder
	literalFreq     []int32
	offsetFreq      []int32
	codegen         []uint8
	nbytes          int
	nbits           uint
	bits            uint64
	codegenFreq     [codegenCodeCount]int32
	bytes           [bufferSize]byte
}

func newHuffmanBitWriter(w io.Writer) *huffmanBitWriter {
	return &huffmanBitWriter{
		writer:          w,
		literalFreq:     make([]int32, maxNumLit),
		offsetFreq:      make([]int32, offsetCodeCount),
		codegen:         make([]uint8, maxNumLit+offsetCodeCount+1),
		literalEncoding: newHuffmanEncoder(maxNumLit),
		codegenEncoding: newHuffmanEncoder(codegenCodeCount),
		offsetEncoding:  newHuffmanEncoder(offsetCodeCount),
	}
}

func (w *huffmanBitWriter) flush() {
	if w.err != nil {
		w.nbits = 0
		return
	}

	n := w.nbytes
	for w.nbits != 0 {
		w.bytes[n] = byte(w.bits)

		w.bits >>= byteBits
		if w.nbits > byteBits { // Avoid underflow
			w.nbits -= byteBits
		} else {
			w.nbits = 0
		}

		n++
	}

	w.bits = 0
	w.write(w.bytes[:n])
	w.nbytes = 0
}

func (w *huffmanBitWriter) write(b []byte) {
	if w.err != nil {
		return
	}

	_, w.err = w.writer.Write(b)
}

func (w *huffmanBitWriter) writeBits(b int32, nb uint) {
	if w.err != nil {
		return
	}

	w.bits |= uint64(b) << w.nbits

	w.nbits += nb
	if w.nbits >= flushBits {
		bits := w.bits
		w.bits >>= flushBits
		w.nbits -= flushBits
		n := w.nbytes

		for i := range flushBytes {
			w.bytes[n+i] = byte(bits >> (byteBits * i))
		}

		n += flushBytes
		if n >= bufferFlushSize {
			w.write(w.bytes[:n])
			n = 0
		}

		w.nbytes = n
	}
}

func (w *huffmanBitWriter) writeBytes(bytes []byte) {
	if w.err != nil {
		return
	}

	n := w.nbytes
	if w.nbits&7 != 0 {
		w.err = errors.New("writeBytes with unfinished bits")
		return
	}

	for w.nbits != 0 {
		w.bytes[n] = byte(w.bits)
		w.bits >>= 8
		w.nbits -= 8
		n++
	}

	if n != 0 {
		w.write(w.bytes[:n])
	}

	w.nbytes = 0
	w.write(bytes)
}

// RFC 1951 3.2.7 specifies a special run-length encoding for specifying
// the literal and offset lengths arrays (which are concatenated into a single
// array).  This method generates that run-length encoding.
//
// The result is written into the codegen array, and the frequencies
// of each code is written into the codegenFreq array.
// Codes 0-15 are single byte codes. Codes 16-18 are followed by additional
// information. Code badCode is an end marker
//
//	numLiterals      The number of literals in literalEncoding
//	numOffsets       The number of offsets in offsetEncoding
//	litenc, offenc   The literal and offset encoder to use
func (w *huffmanBitWriter) generateCodegen(numLiterals int, numOffsets int, litEnc, offEnc *huffmanEncoder) {
	clear(w.codegenFreq[:])
	// Note that we are using codegen both as a temporary variable for holding
	// a copy of the frequencies, and as the place where we put the result.
	// This is fine because the output is always shorter than the input used
	// so far.
	codegen := w.codegen // cache
	// Copy the concatenated code sizes to codegen. Put a marker at the end.
	cgnl := codegen[:numLiterals]
	for i := range cgnl {
		cgnl[i] = uint8(litEnc.codes[i].len)
	}

	cgnl = codegen[numLiterals : numLiterals+numOffsets]
	for i := range cgnl {
		cgnl[i] = uint8(offEnc.codes[i].len)
	}

	codegen[numLiterals+numOffsets] = badCode

	size := codegen[0]
	count := 1
	outIndex := 0

	for inIndex := 1; size != badCode; inIndex++ {
		// INVARIANT: We have seen "count" copies of size that have not yet
		// had output generated for them.
		nextSize := codegen[inIndex]
		if nextSize == size {
			count++
			continue
		}
		// We need to generate codegen indicating "count" of size.
		if size != 0 {
			codegen[outIndex] = size
			outIndex++
			w.codegenFreq[size]++

			count--
			for count >= repeatPrevMin {
				n := min(repeatPrevMax, count)
				codegen[outIndex] = codeRepeatPrev
				outIndex++
				codegen[outIndex] = uint8(n - repeatPrevMin)
				outIndex++
				w.codegenFreq[codeRepeatPrev]++
				count -= n
			}
		} else {
			for count >= repeatZeroLongMin {
				n := min(repeatZeroLongMax, count)
				codegen[outIndex] = codeRepeatZeroLong
				outIndex++
				codegen[outIndex] = uint8(n - repeatZeroLongMin)
				outIndex++
				w.codegenFreq[codeRepeatZeroLong]++
				count -= n
			}

			if count >= repeatZeroShortMin {
				// count >= 3 && count <= 10
				codegen[outIndex] = codeRepeatZeroShort
				outIndex++
				codegen[outIndex] = uint8(count - repeatZeroShortMin)
				outIndex++
				w.codegenFreq[codeRepeatZeroShort]++
				count = 0
			}
		}

		count--
		for ; count >= 0; count-- {
			codegen[outIndex] = size
			outIndex++
			w.codegenFreq[size]++
		}
		// Set up invariant for next time through the loop.
		size = nextSize
		count = 1
	}
	// Marker indicating the end of the codegen.
	codegen[outIndex] = badCode
}

// dynamicSize returns the size of dynamically encoded data in bits.
func (w *huffmanBitWriter) dynamicSize(litEnc, offEnc *huffmanEncoder, extraBits int) (size, numCodegens int) {
	numCodegens = len(w.codegenFreq)
	for numCodegens > minCodegens && w.codegenFreq[codegenOrder[numCodegens-1]] == 0 {
		numCodegens--
	}

	header := blockTypeBits + hlitBits + hdistBits + hclenBits + (codeLenBits * numCodegens) +
		w.codegenEncoding.bitLength(w.codegenFreq[:]) +
		int(w.codegenFreq[codeRepeatPrev])*repeatPrevExtraBits +
		int(w.codegenFreq[codeRepeatZeroShort])*repeatZeroShortExtraBits +
		int(w.codegenFreq[codeRepeatZeroLong])*repeatZeroLongExtraBits
	size = header +
		litEnc.bitLength(w.literalFreq) +
		offEnc.bitLength(w.offsetFreq) +
		extraBits

	return size, numCodegens
}

// fixedSize returns the size of dynamically encoded data in bits.
func (w *huffmanBitWriter) fixedSize(extraBits int) int {
	return blockTypeBits +
		fixedLiteralEncoding.bitLength(w.literalFreq) +
		fixedOffsetEncoding.bitLength(w.offsetFreq) +
		extraBits
}

// storedSize calculates the stored size, including header.
// The function returns the size in bits and whether the block
// fits inside a single block.
func (w *huffmanBitWriter) storedSize(in []byte) (int, bool) {
	if in == nil {
		return 0, false
	}

	if len(in) <= maxStoreBlockSize {
		return (len(in) + storedOverhead) * byteBits, true
	}

	return 0, false
}

func (w *huffmanBitWriter) writeCode(c hcode) {
	w.writeBits(int32(c.code), uint(c.len))
}

// Write the header of a dynamic Huffman block to the output stream.
//
//	numLiterals  The number of literals specified in codegen
//	numOffsets   The number of offsets specified in codegen
//	numCodegens  The number of codegens used in codegen
func (w *huffmanBitWriter) writeDynamicHeader(numLiterals int, numOffsets int, numCodegens int, isEOF bool) {
	if w.err != nil {
		return
	}

	var firstBits int32 = 4
	if isEOF {
		firstBits = 5
	}

	w.writeBits(firstBits, blockTypeBits)
	w.writeBits(int32(numLiterals-lengthCodesStart), hlitBits)
	w.writeBits(int32(numOffsets-1), hdistBits)
	w.writeBits(int32(numCodegens-minCodegens), hclenBits)

	for i := range numCodegens {
		value := uint(w.codegenEncoding.codes[codegenOrder[i]].len)
		w.writeBits(int32(value), codeLenBits)
	}

	i := 0
	for {
		var codeWord = int(w.codegen[i])

		i++

		if codeWord == badCode {
			break
		}

		w.writeCode(w.codegenEncoding.codes[uint32(codeWord)])

		switch codeWord {
		case codeRepeatPrev:
			w.writeBits(int32(w.codegen[i]), repeatPrevExtraBits)
			i++
		case codeRepeatZeroShort:
			w.writeBits(int32(w.codegen[i]), repeatZeroShortExtraBits)
			i++
		case codeRepeatZeroLong:
			w.writeBits(int32(w.codegen[i]), repeatZeroLongExtraBits)
			i++
		}
	}
}

func (w *huffmanBitWriter) writeStoredHeader(length int, isEOF bool) {
	if w.err != nil {
		return
	}

	var flag int32
	if isEOF {
		flag = 1
	}

	w.writeBits(flag, blockTypeBits)
	w.flush()
	w.writeBits(int32(length), storedLenBits)
	w.writeBits(int32(^uint16(length)), storedLenBits)
}

func (w *huffmanBitWriter) writeFixedHeader(isEOF bool) {
	if w.err != nil {
		return
	}
	// Indicate that we are a fixed Huffman block
	var value int32 = 2
	if isEOF {
		value = 3
	}

	w.writeBits(value, blockTypeBits)
}

// writeBlock will write a block of tokens with the smallest encoding.
// The original input can be supplied, and if the huffman encoded data
// is larger than the original bytes, the data will be written as a
// stored block.
// If the input is nil, the tokens will always be Huffman encoded.
func (w *huffmanBitWriter) writeBlock(tokens []token, eof bool, input []byte) {
	if w.err != nil {
		return
	}

	tokens = append(tokens, endBlockMarker)
	numLiterals, numOffsets := w.indexTokens(tokens)

	var extraBits int

	storedSize, storable := w.storedSize(input)
	if storable {
		// We only bother calculating the costs of the extra bits required by
		// the length of offset fields (which will be the same for both fixed
		// and dynamic encoding), if we need to compare those two encodings
		// against stored encoding.
		for lengthCode := lengthCodesStart + lengthCodesNoExtra; lengthCode < numLiterals; lengthCode++ {
			// First eight length codes have extra size = 0.
			extraBits += int(w.literalFreq[lengthCode]) * int(lengthExtraBits[lengthCode-lengthCodesStart])
		}

		for offsetCode := offsetCodesNoExtra; offsetCode < numOffsets; offsetCode++ {
			// First four offset codes have extra size = 0.
			extraBits += int(w.offsetFreq[offsetCode]) * int(offsetExtraBits[offsetCode])
		}
	}

	// Figure out smallest code.
	// Fixed Huffman baseline.
	var (
		literalEncoding = fixedLiteralEncoding
		offsetEncoding  = fixedOffsetEncoding
		size            = w.fixedSize(extraBits)
	)

	// Dynamic Huffman?
	var numCodegens int

	// Generate codegen and codegenFrequencies, which indicates how to encode
	// the literalEncoding and the offsetEncoding.
	w.generateCodegen(numLiterals, numOffsets, w.literalEncoding, w.offsetEncoding)
	w.codegenEncoding.generate(w.codegenFreq[:], maxCodegenBits)
	dynamicSize, numCodegens := w.dynamicSize(w.literalEncoding, w.offsetEncoding, extraBits)

	if dynamicSize < size {
		size = dynamicSize
		literalEncoding = w.literalEncoding
		offsetEncoding = w.offsetEncoding
	}

	// Stored bytes?
	if storable && storedSize < size {
		w.writeStoredHeader(len(input), eof)
		w.writeBytes(input)

		return
	}

	// Huffman.
	if literalEncoding == fixedLiteralEncoding {
		w.writeFixedHeader(eof)
	} else {
		w.writeDynamicHeader(numLiterals, numOffsets, numCodegens, eof)
	}

	// Write the tokens.
	w.writeTokens(tokens, literalEncoding.codes, offsetEncoding.codes)
}

// indexTokens indexes a slice of tokens, and updates
// literalFreq and offsetFreq, and generates literalEncoding
// and offsetEncoding.
// The number of literal and offset tokens is returned.
func (w *huffmanBitWriter) indexTokens(tokens []token) (numLiterals, numOffsets int) {
	clear(w.literalFreq)
	clear(w.offsetFreq)

	for _, t := range tokens {
		if t < matchType {
			w.literalFreq[t.literal()]++
			continue
		}

		length := t.length()
		offset := t.offset()
		w.literalFreq[lengthCodesStart+lengthCode(length)]++
		w.offsetFreq[offsetCode(offset)]++
	}

	// get the number of literals
	numLiterals = len(w.literalFreq)
	for w.literalFreq[numLiterals-1] == 0 {
		numLiterals--
	}
	// get the number of offsets
	numOffsets = len(w.offsetFreq)
	for numOffsets > 0 && w.offsetFreq[numOffsets-1] == 0 {
		numOffsets--
	}

	if numOffsets == 0 {
		// We haven't found a single match. If we want to go with the dynamic encoding,
		// we should count at least one offset to be sure that the offset huffman tree could be encoded.
		w.offsetFreq[0] = 1
		numOffsets = 1
	}

	w.literalEncoding.generate(w.literalFreq, maxCodeBits)
	w.offsetEncoding.generate(w.offsetFreq, maxCodeBits)

	return
}

// writeTokens writes a slice of tokens to the output.
// codes for literal and offset encoding must be supplied.
func (w *huffmanBitWriter) writeTokens(tokens []token, leCodes, oeCodes []hcode) {
	if w.err != nil {
		return
	}

	for _, t := range tokens {
		if t < matchType {
			w.writeCode(leCodes[t.literal()])
			continue
		}
		// Write the length
		length := t.length()
		lengthCode := lengthCode(length)
		w.writeCode(leCodes[lengthCode+lengthCodesStart])

		extraLengthBits := uint(lengthExtraBits[lengthCode])
		if extraLengthBits > 0 {
			extraLength := int32(length - lengthBase[lengthCode])
			w.writeBits(extraLength, extraLengthBits)
		}
		// Write the offset
		offset := t.offset()
		offsetCode := offsetCode(offset)
		w.writeCode(oeCodes[offsetCode])

		extraOffsetBits := uint(offsetExtraBits[offsetCode])
		if extraOffsetBits > 0 {
			extraOffset := int32(offset - offsetBase[offsetCode])
			w.writeBits(extraOffset, extraOffsetBits)
		}
	}
}

// hcode is a huffman code with a bit code and bit length.
type hcode struct {
	code, len uint16
}

type huffmanEncoder struct {
	codes     []hcode
	freqcache []literalNode
	bitCount  [17]int32
}

type literalNode struct {
	literal uint16
	freq    int32
}

// A levelInfo describes the state of the constructed tree for a given depth.
type levelInfo struct {
	// Our level.  for better printing
	level int32

	// The frequency of the last node at this level
	lastFreq int32

	// The frequency of the next character to add to this level
	nextCharFreq int32

	// The frequency of the next pair (from level below) to add to this level.
	// Only valid if the "needed" value of the next lower level is 0.
	nextPairFreq int32

	// The number of chains remaining to generate for this level before moving
	// up to the next level
	needed int32
}

// set sets the code and length of an hcode.
func (h *hcode) set(code uint16, length uint16) {
	h.len = length
	h.code = code
}

func maxNode() literalNode { return literalNode{math.MaxUint16, math.MaxInt32} }

func newHuffmanEncoder(size int) *huffmanEncoder {
	return &huffmanEncoder{codes: make([]hcode, size)}
}

// Generates a HuffmanCode corresponding to the fixed literal table.
//
//nolint:mnd // RFC 1951 section 3.2.6 fixed Huffman code table.
func generateFixedLiteralEncoding() *huffmanEncoder {
	h := newHuffmanEncoder(maxNumLit)
	codes := h.codes

	var ch uint16
	for ch = range maxNumLit {
		var (
			bits uint16
			size uint16
		)

		switch {
		case ch < 144:
			// size 8, 000110000  .. 10111111
			bits = ch + 48
			size = 8
		case ch < 256:
			// size 9, 110010000 .. 111111111
			bits = ch + 400 - 144
			size = 9
		case ch < 280:
			// size 7, 0000000 .. 0010111
			bits = ch - 256
			size = 7
		default:
			// size 8, 11000000 .. 11000111
			bits = ch + 192 - 280
			size = 8
		}

		codes[ch] = hcode{code: reverseBits(bits, byte(size)), len: size}
	}

	return h
}

func generateFixedOffsetEncoding() *huffmanEncoder {
	h := newHuffmanEncoder(offsetCodeCount)

	codes := h.codes
	for ch := range codes {
		codes[ch] = hcode{code: reverseBits(uint16(ch), fixedOffsetBits), len: fixedOffsetBits}
	}

	return h
}

var fixedLiteralEncoding *huffmanEncoder = generateFixedLiteralEncoding()
var fixedOffsetEncoding *huffmanEncoder = generateFixedOffsetEncoding()

func (h *huffmanEncoder) bitLength(freq []int32) int {
	var total int

	for i, f := range freq {
		if f != 0 {
			total += int(f) * int(h.codes[i].len)
		}
	}

	return total
}

const maxBitsLimit = 16

// bitCounts computes the number of literals assigned to each bit size in the Huffman encoding.
// It is only called when list.length >= 3.
// The cases of 0, 1, and 2 literals are handled by special case code.
//
// list is an array of the literals with non-zero frequencies
// and their associated frequencies. The array is in order of increasing
// frequency and has as its last element a special element with frequency
// MaxInt32.
//
// maxBits is the maximum number of bits that should be used to encode any literal.
// It must be less than 16.
//
// bitCounts returns an integer slice in which slice[i] indicates the number of literals
// that should be encoded in i bits.
func (h *huffmanEncoder) bitCounts(list []literalNode, maxBits int32) []int32 {
	if maxBits >= maxBitsLimit {
		panic("maxBits too large")
	}

	n := int32(len(list))
	list = list[0 : n+1]
	list[n] = maxNode()

	// The tree can't have greater depth than n - 1, no matter what. This
	// saves a little bit of work in some small cases
	if maxBits > n-1 {
		maxBits = n - 1
	}

	// Create information about each of the levels.
	// A bogus "Level 0" whose sole purpose is so that
	// level1.prev.needed==0.  This makes level1.nextPairFreq
	// be a legitimate value that never gets chosen.
	var levels [maxBitsLimit]levelInfo
	// leafCounts[i] counts the number of literals at the left
	// of ancestors of the rightmost node at level i.
	// leafCounts[i][j] is the number of literals at the left
	// of the level j ancestor.
	var leafCounts [maxBitsLimit][maxBitsLimit]int32

	for level := int32(1); level <= maxBits; level++ {
		// For every level, the first two items are the first two characters.
		// We initialize the levels as if we had already figured this out.
		levels[level] = levelInfo{
			level:        level,
			lastFreq:     list[1].freq,
			nextCharFreq: list[2].freq,
			nextPairFreq: list[0].freq + list[1].freq,
		}

		leafCounts[level][level] = 2
		if level == 1 {
			levels[level].nextPairFreq = math.MaxInt32
		}
	}

	// We need a total of 2*n - 2 items at top level and have already generated 2.
	levels[maxBits].needed = 2*n - 4 //nolint:mnd // 2n-2 nodes in total, two already placed.

	level := maxBits
	for {
		l := &levels[level]
		if l.nextPairFreq == math.MaxInt32 && l.nextCharFreq == math.MaxInt32 {
			// We've run out of both leaves and pairs.
			// End all calculations for this level.
			// To make sure we never come back to this level or any lower level,
			// set nextPairFreq impossibly large.
			l.needed = 0
			levels[level+1].nextPairFreq = math.MaxInt32
			level++

			continue
		}

		prevFreq := l.lastFreq
		if l.nextCharFreq < l.nextPairFreq {
			// The next item on this row is a leaf node.
			n := leafCounts[level][level] + 1
			l.lastFreq = l.nextCharFreq
			// Lower leafCounts are the same of the previous node.
			leafCounts[level][level] = n
			l.nextCharFreq = list[n].freq
		} else {
			// The next item on this row is a pair from the previous row.
			// nextPairFreq isn't valid until we generate two
			// more values in the level below
			l.lastFreq = l.nextPairFreq
			// Take leaf counts from the lower level, except counts[level] remains the same.
			copy(leafCounts[level][:level], leafCounts[level-1][:level])
			levels[l.level-1].needed = 2
		}

		if l.needed--; l.needed == 0 {
			// We've done everything we need to do for this level.
			// Continue calculating one level up. Fill in nextPairFreq
			// of that level with the sum of the two nodes we've just calculated on
			// this level.
			if l.level == maxBits {
				// All done!
				break
			}

			levels[l.level+1].nextPairFreq = prevFreq + l.lastFreq
			level++
		} else {
			// If we stole from below, move down temporarily to replenish it.
			for levels[level-1].needed > 0 {
				level--
			}
		}
	}

	// Somethings is wrong if at the end, the top level is null or hasn't used
	// all of the leaves.
	if leafCounts[maxBits][maxBits] != n {
		panic("leafCounts[maxBits][maxBits] != n")
	}

	bitCount := h.bitCount[:maxBits+1]
	bits := 1

	counts := &leafCounts[maxBits]
	for level := maxBits; level > 0; level-- {
		// chain.leafCount gives the number of literals requiring at least "bits"
		// bits to encode.
		bitCount[bits] = counts[level] - counts[level-1]
		bits++
	}

	return bitCount
}

// Look at the leaves and assign them a bit count and an encoding as specified
// in RFC 1951 3.2.2
func (h *huffmanEncoder) assignEncodingAndSize(bitCount []int32, list []literalNode) {
	code := uint16(0)
	for n, bits := range bitCount {
		code <<= 1

		if n == 0 || bits == 0 {
			continue
		}
		// The literals list[len(list)-bits] .. list[len(list)-bits]
		// are encoded using "bits" bits, and get the values
		// code, code + 1, ....  The code values are
		// assigned in literal order (not frequency order).
		chunk := list[len(list)-int(bits):]

		slices.SortFunc(chunk, func(a, b literalNode) int { return cmp.Compare(a.literal, b.literal) })

		for _, node := range chunk {
			h.codes[node.literal] = hcode{code: reverseBits(code, uint8(n)), len: uint16(n)}
			code++
		}

		list = list[0 : len(list)-int(bits)]
	}
}

// Update this Huffman Code object to be the minimum code for the specified frequency count.
//
// freq is an array of frequencies, in which freq[i] gives the frequency of literal i.
// maxBits  The maximum number of bits to use for any literal.
func (h *huffmanEncoder) generate(freq []int32, maxBits int32) {
	if h.freqcache == nil {
		// Allocate a reusable buffer with the longest possible frequency table.
		// Possible lengths are codegenCodeCount, offsetCodeCount and maxNumLit.
		// The largest of these is maxNumLit, so we allocate for that case.
		h.freqcache = make([]literalNode, maxNumLit+1)
	}

	list := h.freqcache[:len(freq)+1]
	// Number of non-zero literals
	count := 0
	// Set list to be the set of all non-zero literals and their frequencies
	for i, f := range freq {
		if f != 0 {
			list[count] = literalNode{uint16(i), f}
			count++
		} else {
			h.codes[i].len = 0
		}
	}

	list = list[:count]
	if count <= 2 { //nolint:mnd // one or two literals get one-bit codes
		// Handle the small cases here, because they are awkward for the general case code. With
		// two or fewer literals, everything has bit length 1.
		for i, node := range list {
			// "list" is in order of increasing literal value.
			h.codes[node.literal].set(uint16(i), 1)
		}

		return
	}

	slices.SortFunc(list, func(a, b literalNode) int {
		if c := cmp.Compare(a.freq, b.freq); c != 0 {
			return c
		}

		return cmp.Compare(a.literal, b.literal)
	})

	// Get the number of literals for each bit count
	bitCount := h.bitCounts(list, maxBits)
	// And do the assignment
	h.assignEncodingAndSize(bitCount, list)
}

func reverseBits(number uint16, bitLength byte) uint16 {
	return bits.Reverse16(number << (uint16Bits - bitLength))
}

const (
	// 2 bits:   type   0 = literal  1=EOF  2=Match   3=Unused
	// 8 bits:   xlength = length - MIN_MATCH_LENGTH
	// 22 bits   xoffset = offset - MIN_OFFSET_SIZE, or literal
	lengthShift = 22
	offsetMask  = 1<<lengthShift - 1
	literalType = 0 << 30
	matchType   = 1 << 30
)

// The length code for length X (MIN_MATCH_LENGTH <= X <= MAX_MATCH_LENGTH)
// is lengthCodes[length - MIN_MATCH_LENGTH]
var lengthCodes = [...]uint32{ //nolint:dupl // lookup table
	0, 1, 2, 3, 4, 5, 6, 7, 8, 8,
	9, 9, 10, 10, 11, 11, 12, 12, 12, 12,
	13, 13, 13, 13, 14, 14, 14, 14, 15, 15,
	15, 15, 16, 16, 16, 16, 16, 16, 16, 16,
	17, 17, 17, 17, 17, 17, 17, 17, 18, 18,
	18, 18, 18, 18, 18, 18, 19, 19, 19, 19,
	19, 19, 19, 19, 20, 20, 20, 20, 20, 20,
	20, 20, 20, 20, 20, 20, 20, 20, 20, 20,
	21, 21, 21, 21, 21, 21, 21, 21, 21, 21,
	21, 21, 21, 21, 21, 21, 22, 22, 22, 22,
	22, 22, 22, 22, 22, 22, 22, 22, 22, 22,
	22, 22, 23, 23, 23, 23, 23, 23, 23, 23,
	23, 23, 23, 23, 23, 23, 23, 23, 24, 24,
	24, 24, 24, 24, 24, 24, 24, 24, 24, 24,
	24, 24, 24, 24, 24, 24, 24, 24, 24, 24,
	24, 24, 24, 24, 24, 24, 24, 24, 24, 24,
	25, 25, 25, 25, 25, 25, 25, 25, 25, 25,
	25, 25, 25, 25, 25, 25, 25, 25, 25, 25,
	25, 25, 25, 25, 25, 25, 25, 25, 25, 25,
	25, 25, 26, 26, 26, 26, 26, 26, 26, 26,
	26, 26, 26, 26, 26, 26, 26, 26, 26, 26,
	26, 26, 26, 26, 26, 26, 26, 26, 26, 26,
	26, 26, 26, 26, 27, 27, 27, 27, 27, 27,
	27, 27, 27, 27, 27, 27, 27, 27, 27, 27,
	27, 27, 27, 27, 27, 27, 27, 27, 27, 27,
	27, 27, 27, 27, 27, 28,
}

var offsetCodes = [...]uint32{ //nolint:dupl // lookup table
	0, 1, 2, 3, 4, 4, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7,
	8, 8, 8, 8, 8, 8, 8, 8, 9, 9, 9, 9, 9, 9, 9, 9,
	10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
	11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11,
	12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
	12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
	13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13,
	13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13, 13,
	14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14,
	14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14,
	14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14,
	14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14,
	15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
}

type token uint32

// Convert a literal into a literal token.
func literalToken(literal uint32) token { return token(literalType + literal) }

// Convert a < xlength, xoffset > pair into a match token.
func matchToken(xlength uint32, xoffset uint32) token {
	return token(matchType + xlength<<lengthShift + xoffset)
}

// Returns the literal of a literal token.
func (t token) literal() uint32 { return uint32(t - literalType) }

// Returns the extra offset of a match token.
func (t token) offset() uint32 { return uint32(t) & offsetMask }

func (t token) length() uint32 { return uint32((t - matchType) >> lengthShift) }

func lengthCode(len uint32) uint32 { return lengthCodes[len] }

// Returns the offset code corresponding to a specific offset.
//
//nolint:mnd // offsets beyond the table reuse it at 128x and 16384x.
func offsetCode(off uint32) uint32 {
	if off < uint32(len(offsetCodes)) {
		return offsetCodes[off]
	}

	if off>>7 < uint32(len(offsetCodes)) {
		return offsetCodes[off>>7] + 14
	}

	return offsetCodes[off>>14] + 28
}

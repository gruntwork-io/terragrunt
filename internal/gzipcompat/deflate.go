package gzipcompat

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

	baseMatchLength = 3
	minMatchLength  = 4
	maxMatchLength  = 258
	baseMatchOffset = 1
	maxMatchOffset  = 1 << 15

	maxFlateBlockTokens = 1 << 14
	maxStoreBlockSize   = 65535
	hashBits            = 17
	hashSize            = 1 << hashBits
	hashMask            = (1 << hashBits) - 1
	maxHashOffset       = 1 << 24
	hashShift           = 32 - hashBits
	windowBufferSize    = 2 * windowSize

	goodLength  = 8
	lazyLength  = 16
	niceLength  = 128
	chainLength = 128
)

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

func (d *compressor) findMatch(pos int, prevHead int, prevLength int, lookahead int) (length, offset int, ok bool) {
	minMatchLook := min(lookahead, maxMatchLength)

	win := d.window[0 : pos+minMatchLook]

	nice := min(niceLength, len(win)-pos)

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
					break
				}

				wEnd = win[pos+n]
			}
		}

		if i == minIndex {
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

func hash4(b []byte) uint32 {
	return ((uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24) * hashmul) >> hashShift
}

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
				if d.byteAvailable {
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
			d.tokens = append(d.tokens, matchToken(uint32(prevLength-baseMatchLength), uint32(prevOffset-baseMatchOffset)))

			newIndex := d.index + prevLength - 1

			index := d.index
			for index++; index < newIndex; index++ {
				if index < d.maxInsertIndex {
					hash := hash4(d.window[index : index+minMatchLength])

					hh := &d.hashHead[hash&hashMask]
					d.hashPrev[index&windowMask] = *hh

					*hh = uint32(index + d.hashOffset)
				}
			}

			d.index = index
			d.byteAvailable = false
			d.length = minMatchLength - 1

			if len(d.tokens) == maxFlateBlockTokens {
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
	offsetCodeCount = 30

	endBlockMarker = 256

	lengthCodesStart = 257

	codegenCodeCount = 19
	badCode          = 255

	bufferFlushSize = 240

	bufferSize = bufferFlushSize + 8

	byteBits   = 8
	flushBits  = 48
	flushBytes = flushBits / byteBits

	blockTypeBits  = 3
	storedLenBits  = 16
	storedOverhead = 5
	hlitBits       = 5
	hdistBits      = 5
	hclenBits      = 4
	minCodegens    = 4
	codeLenBits    = 3
	maxCodeBits    = 15
	maxCodegenBits = 7
	uint16Bits     = 16

	codeRepeatPrev           = 16
	codeRepeatZeroShort      = 17
	codeRepeatZeroLong       = 18
	repeatPrevMin            = 3
	repeatPrevMax            = 6
	repeatPrevExtraBits      = 2
	repeatZeroShortMin       = 3
	repeatZeroShortExtraBits = 3
	repeatZeroLongMin        = 11
	repeatZeroLongMax        = 138
	repeatZeroLongExtraBits  = 7

	lengthCodesNoExtra = 8
	offsetCodesNoExtra = 4
	fixedOffsetBits    = 5
)

var lengthExtraBits = []int8{
	0, 0, 0,
	0, 0, 0, 0, 0, 1, 1, 1, 1, 2,
	2, 2, 2, 3, 3, 3, 3, 4, 4, 4,
	4, 5, 5, 5, 5, 0,
}

var lengthBase = []uint32{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 10,
	12, 14, 16, 20, 24, 28, 32, 40, 48, 56,
	64, 80, 96, 112, 128, 160, 192, 224, 255,
}

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
		if w.nbits > byteBits {
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

func (w *huffmanBitWriter) generateCodegen(numLiterals int, numOffsets int, litEnc, offEnc *huffmanEncoder) {
	clear(w.codegenFreq[:])

	codegen := w.codegen

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
		nextSize := codegen[inIndex]
		if nextSize == size {
			count++
			continue
		}

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

		size = nextSize
		count = 1
	}

	codegen[outIndex] = badCode
}

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

func (w *huffmanBitWriter) fixedSize(extraBits int) int {
	return blockTypeBits +
		fixedLiteralEncoding.bitLength(w.literalFreq) +
		fixedOffsetEncoding.bitLength(w.offsetFreq) +
		extraBits
}

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

	var value int32 = 2
	if isEOF {
		value = 3
	}

	w.writeBits(value, blockTypeBits)
}

func (w *huffmanBitWriter) writeBlock(tokens []token, eof bool, input []byte) {
	if w.err != nil {
		return
	}

	tokens = append(tokens, endBlockMarker)
	numLiterals, numOffsets := w.indexTokens(tokens)

	var extraBits int

	storedSize, storable := w.storedSize(input)
	if storable {
		for lengthCode := lengthCodesStart + lengthCodesNoExtra; lengthCode < numLiterals; lengthCode++ {
			extraBits += int(w.literalFreq[lengthCode]) * int(lengthExtraBits[lengthCode-lengthCodesStart])
		}

		for offsetCode := offsetCodesNoExtra; offsetCode < numOffsets; offsetCode++ {
			extraBits += int(w.offsetFreq[offsetCode]) * int(offsetExtraBits[offsetCode])
		}
	}

	var (
		literalEncoding = fixedLiteralEncoding
		offsetEncoding  = fixedOffsetEncoding
		size            = w.fixedSize(extraBits)
	)

	var numCodegens int

	w.generateCodegen(numLiterals, numOffsets, w.literalEncoding, w.offsetEncoding)
	w.codegenEncoding.generate(w.codegenFreq[:], maxCodegenBits)
	dynamicSize, numCodegens := w.dynamicSize(w.literalEncoding, w.offsetEncoding, extraBits)

	if dynamicSize < size {
		size = dynamicSize
		literalEncoding = w.literalEncoding
		offsetEncoding = w.offsetEncoding
	}

	if storable && storedSize < size {
		w.writeStoredHeader(len(input), eof)
		w.writeBytes(input)

		return
	}

	if literalEncoding == fixedLiteralEncoding {
		w.writeFixedHeader(eof)
	} else {
		w.writeDynamicHeader(numLiterals, numOffsets, numCodegens, eof)
	}

	w.writeTokens(tokens, literalEncoding.codes, offsetEncoding.codes)
}

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

	numLiterals = len(w.literalFreq)
	for w.literalFreq[numLiterals-1] == 0 {
		numLiterals--
	}

	numOffsets = len(w.offsetFreq)
	for numOffsets > 0 && w.offsetFreq[numOffsets-1] == 0 {
		numOffsets--
	}

	if numOffsets == 0 {
		w.offsetFreq[0] = 1
		numOffsets = 1
	}

	w.literalEncoding.generate(w.literalFreq, maxCodeBits)
	w.offsetEncoding.generate(w.offsetFreq, maxCodeBits)

	return
}

func (w *huffmanBitWriter) writeTokens(tokens []token, leCodes, oeCodes []hcode) {
	if w.err != nil {
		return
	}

	for _, t := range tokens {
		if t < matchType {
			w.writeCode(leCodes[t.literal()])
			continue
		}

		length := t.length()
		lengthCode := lengthCode(length)
		w.writeCode(leCodes[lengthCode+lengthCodesStart])

		extraLengthBits := uint(lengthExtraBits[lengthCode])
		if extraLengthBits > 0 {
			extraLength := int32(length - lengthBase[lengthCode])
			w.writeBits(extraLength, extraLengthBits)
		}

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

type levelInfo struct {
	level int32

	lastFreq int32

	nextCharFreq int32

	nextPairFreq int32

	needed int32
}

func newHuffmanEncoder(size int) *huffmanEncoder {
	return &huffmanEncoder{codes: make([]hcode, size)}
}

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
		case ch < fixedLiteralEnd8:
			bits = ch + fixedLiteralBase8
			size = fixedLiteralBits8
		case ch < fixedLiteralEnd9:
			bits = ch - fixedLiteralEnd8 + fixedLiteralBase9
			size = fixedLiteralBits9
		case ch < fixedLiteralEnd7:
			bits = ch - fixedLiteralEnd9
			size = fixedLiteralBits7
		default:
			bits = ch - fixedLiteralEnd7 + fixedLiteralBase8High
			size = fixedLiteralBits8
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

const (
	maxBitsLimit = 16

	fixedLiteralEnd8      = 144
	fixedLiteralEnd9      = 256
	fixedLiteralEnd7      = 280
	fixedLiteralBase8     = 0x30
	fixedLiteralBase9     = 0x190
	fixedLiteralBase8High = 0xc0
	fixedLiteralBits7     = 7
	fixedLiteralBits8     = 8
	fixedLiteralBits9     = 9

	binaryTreeChildren   = 2
	firstLeaves          = 2
	maxSingleBitLiterals = 2

	offsetFold1Shift = 7
	offsetFold1Codes = 14
	offsetFold2Shift = 14
	offsetFold2Codes = 28
)

func (h *huffmanEncoder) bitCounts(list []literalNode, maxBits int32) []int32 {
	if maxBits >= maxBitsLimit {
		panic("maxBits too large")
	}

	n := int32(len(list))
	list = list[0 : n+1]
	list[n] = literalNode{literal: math.MaxUint16, freq: math.MaxInt32}

	if maxBits > n-1 {
		maxBits = n - 1
	}

	var levels [maxBitsLimit]levelInfo

	var leafCounts [maxBitsLimit][maxBitsLimit]int32

	for level := int32(1); level <= maxBits; level++ {
		levels[level] = levelInfo{
			level:        level,
			lastFreq:     list[1].freq,
			nextCharFreq: list[2].freq,
			nextPairFreq: list[0].freq + list[1].freq,
		}

		leafCounts[level][level] = firstLeaves
		if level == 1 {
			levels[level].nextPairFreq = math.MaxInt32
		}
	}

	levels[maxBits].needed = binaryTreeChildren*n - binaryTreeChildren - firstLeaves

	level := maxBits
	for {
		l := &levels[level]
		if l.nextPairFreq == math.MaxInt32 && l.nextCharFreq == math.MaxInt32 {
			l.needed = 0
			levels[level+1].nextPairFreq = math.MaxInt32
			level++

			continue
		}

		prevFreq := l.lastFreq
		if l.nextCharFreq < l.nextPairFreq {
			n := leafCounts[level][level] + 1
			l.lastFreq = l.nextCharFreq

			leafCounts[level][level] = n
			l.nextCharFreq = list[n].freq
		} else {
			l.lastFreq = l.nextPairFreq

			copy(leafCounts[level][:level], leafCounts[level-1][:level])
			levels[l.level-1].needed = 2
		}

		if l.needed--; l.needed == 0 {
			if l.level == maxBits {
				break
			}

			levels[l.level+1].nextPairFreq = prevFreq + l.lastFreq
			level++
		} else {
			for levels[level-1].needed > 0 {
				level--
			}
		}
	}

	if leafCounts[maxBits][maxBits] != n {
		panic("leafCounts[maxBits][maxBits] != n")
	}

	bitCount := h.bitCount[:maxBits+1]
	bits := 1

	counts := &leafCounts[maxBits]
	for level := maxBits; level > 0; level-- {
		bitCount[bits] = counts[level] - counts[level-1]
		bits++
	}

	return bitCount
}

func (h *huffmanEncoder) assignEncodingAndSize(bitCount []int32, list []literalNode) {
	code := uint16(0)
	for n, bits := range bitCount {
		code <<= 1

		if n == 0 || bits == 0 {
			continue
		}

		chunk := list[len(list)-int(bits):]

		slices.SortFunc(chunk, func(a, b literalNode) int { return cmp.Compare(a.literal, b.literal) })

		for _, node := range chunk {
			h.codes[node.literal] = hcode{code: reverseBits(code, uint8(n)), len: uint16(n)}
			code++
		}

		list = list[0 : len(list)-int(bits)]
	}
}

func (h *huffmanEncoder) generate(freq []int32, maxBits int32) {
	if h.freqcache == nil {
		h.freqcache = make([]literalNode, maxNumLit+1)
	}

	list := h.freqcache[:len(freq)+1]

	count := 0

	for i, f := range freq {
		if f != 0 {
			list[count] = literalNode{uint16(i), f}
			count++
		} else {
			h.codes[i].len = 0
		}
	}

	list = list[:count]
	if count <= maxSingleBitLiterals {
		for i, node := range list {
			h.codes[node.literal] = hcode{code: uint16(i), len: 1}
		}

		return
	}

	slices.SortFunc(list, func(a, b literalNode) int {
		if c := cmp.Compare(a.freq, b.freq); c != 0 {
			return c
		}

		return cmp.Compare(a.literal, b.literal)
	})

	bitCount := h.bitCounts(list, maxBits)

	h.assignEncodingAndSize(bitCount, list)
}

func reverseBits(number uint16, bitLength byte) uint16 {
	return bits.Reverse16(number << (uint16Bits - bitLength))
}

const (
	lengthShift = 22
	offsetMask  = 1<<lengthShift - 1
	literalType = 0 << 30
	matchType   = 1 << 30
)

const codeTableSize = 256

func codeTable(base []uint32, extraBits []int8) [codeTableSize]uint32 {
	var table [codeTableSize]uint32

	for code, start := range base {
		for i := start; i < start+1<<extraBits[code] && i < codeTableSize; i++ {
			table[i] = uint32(code)
		}
	}

	return table
}

var (
	lengthCodes = codeTable(lengthBase, lengthExtraBits)
	offsetCodes = codeTable(offsetBase, offsetExtraBits)
)

type token uint32

func literalToken(literal uint32) token { return token(literalType + literal) }

func matchToken(xlength uint32, xoffset uint32) token {
	return token(matchType + xlength<<lengthShift + xoffset)
}

func (t token) literal() uint32 { return uint32(t - literalType) }

func (t token) offset() uint32 { return uint32(t) & offsetMask }

func (t token) length() uint32 { return uint32((t - matchType) >> lengthShift) }

func lengthCode(len uint32) uint32 { return lengthCodes[len] }

func offsetCode(off uint32) uint32 {
	if off < codeTableSize {
		return offsetCodes[off]
	}

	if off>>offsetFold1Shift < codeTableSize {
		return offsetCodes[off>>offsetFold1Shift] + offsetFold1Codes
	}

	return offsetCodes[off>>offsetFold2Shift] + offsetFold2Codes
}

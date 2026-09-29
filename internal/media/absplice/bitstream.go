package absplice

type BitReader struct {
	data   []byte
	pos    int
	bitPos int
}

func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data, pos: 0, bitPos: 0}
}

func (br *BitReader) ReadBits(n uint) uint32 {
	var val uint32 = 0
	for i := uint(0); i < n; i++ {
		if br.pos >= len(br.data) {
			val <<= 1
			continue
		}
		b := br.data[br.pos]
		bit := (b >> (7 - br.bitPos)) & 0x01
		val = (val << 1) | uint32(bit)
		br.bitPos++
		if br.bitPos >= 8 {
			br.bitPos = 0
			br.pos++
		}
	}
	return val
}

func (br *BitReader) ReadBit() byte {
	return byte(br.ReadBits(1))
}

func (br *BitReader) ReadUEG() uint32 {
	leadingZeroBits := 0
	for {
		if br.pos >= len(br.data) {
			break
		}
		b := br.ReadBit()
		if b == 0 {
			leadingZeroBits++
		} else {
			break
		}
		if leadingZeroBits > 32 {
			break
		}
	}
	var val uint32 = 0
	if leadingZeroBits > 0 {
		val = br.ReadBits(uint(leadingZeroBits))
	}
	result := (uint32(1) << leadingZeroBits) - 1 + val
	return result
}

func (br *BitReader) ReadSEG() int32 {
	codeNum := br.ReadUEG()
	if codeNum == 0 {
		return 0
	}
	if codeNum%2 == 1 {
		return int32((codeNum + 1) / 2)
	}
	return -int32(codeNum / 2)
}

func (br *BitReader) BitsRead() int {
	return br.pos*8 + br.bitPos
}

func (br *BitReader) BytesRead() int {
	return br.pos
}

type BitWriter struct {
	data   []byte
	bitPos int
}

func NewBitWriter() *BitWriter {
	return &BitWriter{data: make([]byte, 0, 64), bitPos: 0}
}

func (bw *BitWriter) ensureCapacity(n int) {
	needed := n / 8
	if n%8 != 0 {
		needed++
	}
	curBytes := len(bw.data)
	if bw.bitPos != 0 {
		curBytes++
	}
	for len(bw.data) < curBytes+needed {
		bw.data = append(bw.data, 0)
	}
}

func (bw *BitWriter) WriteBits(val uint32, n uint) {
	for i := int(n) - 1; i >= 0; i-- {
		bit := byte((val >> i) & 0x01)
		if bw.bitPos == 0 {
			bw.data = append(bw.data, 0)
		}
		lastIdx := len(bw.data) - 1
		bw.data[lastIdx] |= bit << (7 - bw.bitPos)
		bw.bitPos++
		if bw.bitPos >= 8 {
			bw.bitPos = 0
		}
	}
}

func (bw *BitWriter) WriteBit(b byte) {
	bw.WriteBits(uint32(b&0x01), 1)
}

func (bw *BitWriter) WriteUEG(val uint32) {
	if val == 0 {
		bw.WriteBit(1)
		return
	}
	tmp := val + 1
	leadingZero := 0
	mask := uint32(1 << 31)
	for tmp&mask == 0 {
		leadingZero++
		mask >>= 1
	}
	bitLen := 32 - leadingZero
	infoBits := bitLen - 1
	for i := 0; i < infoBits; i++ {
		bw.WriteBit(0)
	}
	bw.WriteBit(1)
	for i := infoBits - 1; i >= 0; i-- {
		bit := byte((tmp >> i) & 0x01)
		bw.WriteBit(bit)
	}
}

func (bw *BitWriter) WriteSEG(val int32) {
	var codeNum uint32
	if val > 0 {
		codeNum = uint32(2*val - 1)
	} else if val < 0 {
		codeNum = uint32(-2 * val)
	} else {
		codeNum = 0
	}
	bw.WriteUEG(codeNum)
}

func (bw *BitWriter) WriteBytes(bs []byte) {
	for _, b := range bs {
		bw.WriteBits(uint32(b), 8)
	}
}

func (bw *BitWriter) Bytes() []byte {
	if len(bw.data) == 0 {
		return []byte{}
	}
	out := make([]byte, len(bw.data))
	copy(out, bw.data)
	return out
}

func (bw *BitWriter) BitsWritten() int {
	return (len(bw.data)-1)*8 + bw.bitPos
}

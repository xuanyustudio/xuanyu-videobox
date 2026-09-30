package absplice

import (
	"bytes"
	"testing"
)

func TestBitReaderWriterBasic(t *testing.T) {
	bw := NewBitWriter()
	bw.WriteBits(0xAB, 8)
	bw.WriteBits(0xCD, 8)
	bw.WriteBits(0x12, 4)
	data := bw.Bytes()
	if len(data) != 3 {
		t.Fatalf("expected 3 bytes, got %d", len(data))
	}
	br := NewBitReader(data)
	if v := br.ReadBits(8); v != 0xAB {
		t.Errorf("expected 0xAB, got 0x%X", v)
	}
	if v := br.ReadBits(8); v != 0xCD {
		t.Errorf("expected 0xCD, got 0x%X", v)
	}
	if v := br.ReadBits(4); v != 0x02 {
		t.Errorf("expected 0x02, got 0x%X", v)
	}
}

func TestUEGCodec(t *testing.T) {
	cases := []uint32{0, 1, 2, 3, 4, 5, 7, 8, 15, 16, 31, 32, 100, 255, 256, 1000, 9999}
	for _, c := range cases {
		bw := NewBitWriter()
		bw.WriteUEG(c)
		data := bw.Bytes()
		br := NewBitReader(data)
		got := br.ReadUEG()
		if got != c {
			t.Errorf("UEG codec mismatch: input=%d, got=%d (bytes=%v)", c, got, data)
		}
	}
}

func TestSEGCodec(t *testing.T) {
	cases := []int32{0, 1, -1, 2, -2, 3, -3, 7, -7, 8, -8, 100, -100, 9999, -9999}
	for _, c := range cases {
		bw := NewBitWriter()
		bw.WriteSEG(c)
		data := bw.Bytes()
		br := NewBitReader(data)
		got := br.ReadSEG()
		if got != c {
			t.Errorf("SEG codec mismatch: input=%d, got=%d (bytes=%v)", c, got, data)
		}
	}
}

func TestEBSPRBSPRoundTrip(t *testing.T) {
	rbsp := []byte{0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x02, 0x03, 0x00, 0x00, 0x03}
	ebsp := RBSPtoEBSP(rbsp)
	got := EBSPtoRBSP(ebsp)
	if !bytes.Equal(got, rbsp) {
		t.Errorf("EBSP<->RBSP roundtrip failed\nrbsp=%v\nebsp=%v\ngot=%v", rbsp, ebsp, got)
	}
}

func TestEBSPtoRBSP_EmulationPrevention(t *testing.T) {
	ebsp := []byte{0x00, 0x00, 0x03, 0x00}
	expected := []byte{0x00, 0x00, 0x00}
	got := EBSPtoRBSP(ebsp)
	if !bytes.Equal(got, expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
	ebsp2 := []byte{0x00, 0x00, 0x03, 0x01}
	expected2 := []byte{0x00, 0x00, 0x01}
	got2 := EBSPtoRBSP(ebsp2)
	if !bytes.Equal(got2, expected2) {
		t.Errorf("expected %v, got %v", expected2, got2)
	}
}

func TestNALUnitParsing(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x0a,
		0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80,
		0x00, 0x00, 0x01, 0x65, 0x88, 0x80, 0x40, 0x00, 0x00, 0x03, 0x00, 0x40, 0x00, 0x00, 0x03, 0x00, 0x78, 0x1e, 0x2c, 0x5c,
	}
	nals := ParseNALUnits(data)
	if len(nals) != 3 {
		t.Fatalf("expected 3 NALs, got %d", len(nals))
	}
	if nals[0].Type != NALTypeSPS {
		t.Errorf("NAL 0 type: expected SPS(%d), got %d", NALTypeSPS, nals[0].Type)
	}
	if nals[1].Type != NALTypePPS {
		t.Errorf("NAL 1 type: expected PPS(%d), got %d", NALTypePPS, nals[1].Type)
	}
	if nals[2].Type != NALTypeIDR {
		t.Errorf("NAL 2 type: expected IDR(%d), got %d", NALTypeIDR, nals[2].Type)
	}
}

func TestFindNALByType(t *testing.T) {
	nals := []NALUnit{
		{Type: NALTypeSPS},
		{Type: NALTypePPS},
		{Type: NALTypeIDR},
	}
	if n := FindNALByType(nals, NALTypeSPS); n == nil || n.Type != NALTypeSPS {
		t.Error("find SPS failed")
	}
	if n := FindNALByType(nals, NALTypeNonIDR); n != nil {
		t.Error("should not find non-existent NAL")
	}
}

func TestPatchBits(t *testing.T) {
	data := []byte{0x00, 0x00}
	// Write 0xA (1010) at bit pos 4, width 4 → first byte becomes 0x0A
	if err := patchBits(data, 4, 4, 0xA); err != nil {
		t.Fatal(err)
	}
	if data[0] != 0x0A {
		t.Fatalf("expected 0x0A, got 0x%02X", data[0])
	}
	// Write 0x3 (11) at bit pos 12, width 2 → byte1 bitIdx 4..5 → 0x0C
	if err := patchBits(data, 12, 2, 0x3); err != nil {
		t.Fatal(err)
	}
	if data[1] != 0x0C {
		t.Fatalf("expected 0x0C, got 0x%02X", data[1])
	}
}

func TestRewriteSliceHeaderIdentityPreservesPayload(t *testing.T) {
	// Synthetic non-IDR slice RBSP:
	// first_mb=0 (1), slice_type=5 (00101 → 5 bits as ue), pps=0 (1), frame_num=1 (4 bits) = 11 bits,
	// then 5 leftover bits in first two bytes + trailing payload that must stay bit-aligned.
	sps := &SPSParams{
		Log2MaxFrameNumMinus4:       0, // 4-bit frame_num
		PicOrderCntType:             2, // no POC field
		FrameMBSOnlyFlag:            true,
		Log2MaxPicOrderCntLsbMinus4: 0,
	}
	bw := NewBitWriter()
	bw.WriteUEG(0)      // first_mb
	bw.WriteUEG(5)      // slice_type P
	bw.WriteUEG(0)      // pps_id
	bw.WriteBits(1, 4)  // frame_num
	// rest: deliberately mid-byte (header ends at bit 11 → rem=3)
	bw.WriteBits(0x1D, 5) // 5 remaining bits of current conceptual byte + start of body
	bw.WriteBytes([]byte{0xAB, 0xCD, 0xEF, 0x01})
	rbsp := bw.Bytes()
	ebsp := RBSPtoEBSP(rbsp)
	nal := &NALUnit{
		Type:       NALTypeNonIDR,
		HeaderByte: 0x41,
		Payload:    ebsp,
	}
	sh, err := ParseSliceHeader(nal, sps)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sh.FrameNum != 1 {
		t.Fatalf("frame_num=%d", sh.FrameNum)
	}
	if sh.HeaderBitsLen%8 == 0 || sh.HeaderBitsLen%8 == 4 {
		t.Fatalf("test fixture should hit non-trivial rem, got rem=%d bits=%d", sh.HeaderBitsLen%8, sh.HeaderBitsLen)
	}

	same, err := RewriteSliceHeaderForRef(nal, sps, sh.FrameNum, sh.PicOrderCntLsb)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same.Payload, nal.Payload) {
		t.Fatalf("identity rewrite must preserve payload\norig=%x\nnew =%x", nal.Payload, same.Payload)
	}

	changed, err := RewriteSliceHeaderForRef(nal, sps, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	sh2, err := ParseSliceHeader(changed, sps)
	if err != nil {
		t.Fatal(err)
	}
	if sh2.FrameNum != 7 {
		t.Fatalf("expected frame_num=7, got %d", sh2.FrameNum)
	}
	// Body after header bits must match original (compare RBSP tails).
	origRBSP := EBSPtoRBSP(nal.Payload)
	newRBSP := EBSPtoRBSP(changed.Payload)
	bitStart := sh.HeaderBitsLen
	if !rbspBitsEqual(origRBSP, newRBSP, bitStart) {
		t.Fatalf("slice body bits after header diverged (bitstream misalignment)")
	}
}

func rbspBitsEqual(a, b []byte, fromBit int) bool {
	if len(a)*8 < fromBit || len(b)*8 < fromBit {
		return false
	}
	maxBits := len(a) * 8
	if len(b)*8 < maxBits {
		maxBits = len(b) * 8
	}
	for i := fromBit; i < maxBits; i++ {
		ba := (a[i/8] >> (7 - (i % 8))) & 1
		bb := (b[i/8] >> (7 - (i % 8))) & 1
		if ba != bb {
			return false
		}
	}
	return true
}

func TestBitWriterBitsWrittenAligned(t *testing.T) {
	bw := NewBitWriter()
	bw.WriteBits(0xFF, 8)
	if bw.BitsWritten() != 8 {
		t.Fatalf("BitsWritten after 8 bits: got %d", bw.BitsWritten())
	}
	bw.WriteBits(0x1, 1)
	if bw.BitsWritten() != 9 {
		t.Fatalf("BitsWritten after 9 bits: got %d", bw.BitsWritten())
	}
}

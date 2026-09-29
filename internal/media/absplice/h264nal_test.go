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

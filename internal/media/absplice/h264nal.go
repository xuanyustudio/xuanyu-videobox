package absplice

import (
	"bytes"
	"errors"
	"fmt"
)

type NALUnit struct {
	StartCodeLen int
	Type         int
	RefIDC       int
	HeaderByte   byte
	Payload      []byte
	Raw          []byte
}

type SPSParams struct {
	ProfileIDC            int
	LevelIDC              int
	Log2MaxFrameNumMinus4 int
	FrameMBSOnlyFlag      bool
	PicOrderCntType       int
	Log2MaxPicOrderCntLsbMinus4 int
	DeltaPicOrderAlwaysZeroFlag bool
	NumRefFrames          int
	Width                 int
	Height                int
}

type SliceHeader struct {
	FirstMBInSlice     uint32
	SliceType          uint32
	PicParameterSetID  uint32
	FrameNum           uint32
	IDRPicID           uint32
	IsIDR              bool
	PicOrderCntLsb     uint32
	DeltaPicOrderCntBottom int32
	DeltaPicOrderCnt   [2]int32
	RedundantPicCnt    uint32
	HeaderBitsLen      int
	RawBytes           []byte
}

const (
	NALTypeNonIDR = 1
	NALTypeSliceA = 2
	NALTypeSliceB = 3
	NALTypeSliceC = 4
	NALTypeIDR    = 5
	NALTypeSEI    = 6
	NALTypeSPS    = 7
	NALTypePPS    = 8
	NALTypeAUD    = 9
	NALTypeEOSeq  = 10
	NALTypeEOStream = 11
	NALTypeFill   = 12
)

func ParseNALUnits(data []byte) []NALUnit {
	var nals []NALUnit
	i := 0
	for i < len(data) {
		scLen := 0
		if i+3 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			scLen = 3
		} else if i+4 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			scLen = 4
		} else {
			i++
			continue
		}
		nalStart := i
		headerIdx := i + scLen
		if headerIdx >= len(data) {
			break
		}
		header := data[headerIdx]
		nalType := int(header & 0x1F)
		refIDC := int((header >> 5) & 0x3)
		i = headerIdx + 1
		for i < len(data) {
			if i+3 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
				break
			}
			if i+4 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
				break
			}
			i++
		}
		nalEnd := i
		payload := make([]byte, nalEnd-headerIdx-1)
		copy(payload, data[headerIdx+1:nalEnd])
		raw := make([]byte, nalEnd-nalStart)
		copy(raw, data[nalStart:nalEnd])
		nals = append(nals, NALUnit{
			StartCodeLen: scLen,
			Type:         nalType,
			RefIDC:       refIDC,
			HeaderByte:   header,
			Payload:      payload,
			Raw:          raw,
		})
	}
	return nals
}

func (n NALUnit) IsVCL() bool {
	return n.Type == NALTypeNonIDR || n.Type == NALTypeIDR ||
		n.Type == NALTypeSliceA || n.Type == NALTypeSliceB || n.Type == NALTypeSliceC
}

func FindNALByType(nals []NALUnit, t int) *NALUnit {
	for i := range nals {
		if nals[i].Type == t {
			return &nals[i]
		}
	}
	return nil
}

func FilterNALByTypes(nals []NALUnit, types ...int) []NALUnit {
	typeSet := make(map[int]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	var out []NALUnit
	for _, n := range nals {
		if typeSet[n.Type] {
			out = append(out, n)
		}
	}
	return out
}

func skipScalingList(br *BitReader, sizeOfScalingList int) {
	lastScale := 8
	nextScale := 8
	for j := 0; j < sizeOfScalingList; j++ {
		if nextScale != 0 {
			deltaScale := int(br.ReadSEG())
			nextScale = (lastScale + deltaScale + 256) % 256
		}
		if nextScale != 0 {
			lastScale = nextScale
		}
	}
}

func ParseSPS(nal *NALUnit) (*SPSParams, error) {
	if nal.Type != NALTypeSPS {
		return nil, fmt.Errorf("not SPS NAL, type=%d", nal.Type)
	}
	rbsp := EBSPtoRBSP(nal.Payload)
	if len(rbsp) < 4 {
		return nil, errors.New("SPS too short")
	}
	br := NewBitReader(rbsp)
	profileIDC := int(br.ReadBits(8))
	br.ReadBits(8) // constraint flags + reserved
	levelIDC := int(br.ReadBits(8))
	_ = br.ReadUEG() // seq_parameter_set_id
	sps := &SPSParams{
		ProfileIDC: profileIDC,
		LevelIDC:   levelIDC,
	}
	chromaFormatIDC := uint32(1)
	if profileIDC == 100 || profileIDC == 110 || profileIDC == 122 ||
		profileIDC == 244 || profileIDC == 44 || profileIDC == 83 ||
		profileIDC == 86 || profileIDC == 118 || profileIDC == 128 ||
		profileIDC == 138 || profileIDC == 139 || profileIDC == 134 ||
		profileIDC == 135 {
		chromaFormatIDC = br.ReadUEG()
		if chromaFormatIDC == 3 {
			br.ReadBits(1) // separate_colour_plane_flag
		}
		_ = br.ReadUEG() // bit_depth_luma_minus8
		_ = br.ReadUEG() // bit_depth_chroma_minus8
		br.ReadBits(1)   // qpprime_y_zero_transform_bypass_flag
		if br.ReadBits(1) == 1 { // seq_scaling_matrix_present_flag
			nLists := 8
			if chromaFormatIDC == 3 {
				nLists = 12
			}
			for i := 0; i < nLists; i++ {
				if br.ReadBits(1) == 1 {
					if i < 6 {
						skipScalingList(br, 16)
					} else {
						skipScalingList(br, 64)
					}
				}
			}
		}
	}
	sps.Log2MaxFrameNumMinus4 = int(br.ReadUEG())
	sps.PicOrderCntType = int(br.ReadUEG())
	if sps.PicOrderCntType == 0 {
		sps.Log2MaxPicOrderCntLsbMinus4 = int(br.ReadUEG())
	} else if sps.PicOrderCntType == 1 {
		if br.ReadBits(1) == 1 {
			sps.DeltaPicOrderAlwaysZeroFlag = true
		}
		_ = int(br.ReadSEG())  // offset_for_non_ref_pic
		_ = int32(br.ReadSEG()) // offset_for_top_to_bottom_field
		numRefFramesInPicOrderCntCycle := br.ReadUEG()
		for i := uint32(0); i < numRefFramesInPicOrderCntCycle; i++ {
			_ = int32(br.ReadSEG())
		}
	}
	sps.NumRefFrames = int(br.ReadUEG())
	br.ReadBits(1) // gaps_in_frame_num_value_allowed_flag
	widthInMBSMinus1 := br.ReadUEG()
	heightInMapUnitsMinus1 := br.ReadUEG()
	frameMBSOnlyFlag := br.ReadBits(1)
	sps.FrameMBSOnlyFlag = frameMBSOnlyFlag == 1
	if !sps.FrameMBSOnlyFlag {
		br.ReadBits(1) // mb_adaptive_frame_field_flag
	}
	br.ReadBits(1) // direct_8x8_inference_flag
	sps.Width = int((widthInMBSMinus1 + 1) * 16)
	sps.Height = int((heightInMapUnitsMinus1 + 1) * 16)
	if !sps.FrameMBSOnlyFlag {
		sps.Height *= 2
	}
	return sps, nil
}

// MaxFrameNum returns 1 << (log2_max_frame_num_minus4 + 4).
func (s *SPSParams) MaxFrameNum() uint32 {
	if s == nil {
		return 16
	}
	return uint32(1) << uint(s.Log2MaxFrameNumMinus4+4)
}

// MaxPicOrderCntLsb returns 1 << (log2_max_pic_order_cnt_lsb_minus4 + 4).
func (s *SPSParams) MaxPicOrderCntLsb() uint32 {
	if s == nil || s.PicOrderCntType != 0 {
		return 256
	}
	return uint32(1) << uint(s.Log2MaxPicOrderCntLsbMinus4+4)
}

func ParseSliceHeader(nal *NALUnit, sps *SPSParams) (*SliceHeader, error) {
	if !nal.IsVCL() {
		return nil, fmt.Errorf("not VCL NAL, type=%d", nal.Type)
	}
	if sps == nil {
		return nil, errors.New("SPS required")
	}
	rbsp := EBSPtoRBSP(nal.Payload)
	br := NewBitReader(rbsp)
	sh := &SliceHeader{
		IsIDR: nal.Type == NALTypeIDR,
	}
	sh.FirstMBInSlice = br.ReadUEG()
	sh.SliceType = br.ReadUEG()
	sh.PicParameterSetID = br.ReadUEG()
	frameNumBits := uint(sps.Log2MaxFrameNumMinus4 + 4)
	sh.FrameNum = br.ReadBits(frameNumBits)
	if !sps.FrameMBSOnlyFlag {
		fieldPicFlag := br.ReadBits(1)
		if fieldPicFlag != 0 {
			br.ReadBits(1) // bottom_field_flag
		}
	}
	if sh.IsIDR {
		sh.IDRPicID = br.ReadUEG()
	}
	if sps.PicOrderCntType == 0 {
		pocBits := uint(sps.Log2MaxPicOrderCntLsbMinus4 + 4)
		sh.PicOrderCntLsb = br.ReadBits(pocBits)
		// delta_pic_order_cnt_bottom depends on PPS flag; leave unread here.
	} else if sps.PicOrderCntType == 1 && !sps.DeltaPicOrderAlwaysZeroFlag {
		sh.DeltaPicOrderCnt[0] = int32(br.ReadSEG())
		// delta_pic_order_cnt[1] depends on PPS; leave unread here.
	}
	sh.HeaderBitsLen = br.BitsRead()
	endBytes := (sh.HeaderBitsLen + 7) / 8
	if endBytes > len(rbsp) {
		endBytes = len(rbsp)
	}
	sh.RawBytes = make([]byte, endBytes)
	copy(sh.RawBytes, rbsp[:endBytes])
	return sh, nil
}

// patchBits overwrites nBits at bitPos (MSB-first within each byte) in data.
func patchBits(data []byte, bitPos int, nBits uint, val uint32) error {
	if nBits == 0 {
		return nil
	}
	end := bitPos + int(nBits)
	if end > len(data)*8 {
		return fmt.Errorf("patchBits: out of range bitPos=%d nBits=%d len=%d", bitPos, nBits, len(data))
	}
	for i := uint(0); i < nBits; i++ {
		bit := byte((val >> (nBits - 1 - i)) & 1)
		idx := bitPos + int(i)
		byteIdx := idx / 8
		bitIdx := idx % 8
		mask := byte(1 << (7 - bitIdx))
		if bit == 1 {
			data[byteIdx] |= mask
		} else {
			data[byteIdx] &^= mask
		}
	}
	return nil
}

// RewriteSliceHeaderForRef patches frame_num and (when POC type 0) pic_order_cnt_lsb
// in-place. The rest of the slice RBSP is left untouched so CABAC/CAVLC data stays aligned.
func RewriteSliceHeaderForRef(nal *NALUnit, sps *SPSParams, newFrameNum uint32, newPOCLsb uint32) (*NALUnit, error) {
	if !nal.IsVCL() {
		return nil, fmt.Errorf("not VCL NAL")
	}
	if sps == nil {
		return nil, errors.New("SPS required")
	}
	if nal.Type == NALTypeIDR {
		cp := *nal
		return &cp, nil
	}

	rbsp := EBSPtoRBSP(nal.Payload)
	br := NewBitReader(rbsp)
	_ = br.ReadUEG() // first_mb_in_slice
	_ = br.ReadUEG() // slice_type
	_ = br.ReadUEG() // pic_parameter_set_id

	frameNumBits := uint(sps.Log2MaxFrameNumMinus4 + 4)
	frameNumBitPos := br.BitsRead()
	_ = br.ReadBits(frameNumBits)

	if !sps.FrameMBSOnlyFlag {
		fieldPicFlag := br.ReadBits(1)
		if fieldPicFlag != 0 {
			br.ReadBits(1) // bottom_field_flag
		}
	}

	pocBitPos := -1
	var pocBits uint
	if sps.PicOrderCntType == 0 {
		pocBits = uint(sps.Log2MaxPicOrderCntLsbMinus4 + 4)
		pocBitPos = br.BitsRead()
		_ = br.ReadBits(pocBits)
	}

	maxFN := sps.MaxFrameNum()
	if maxFN > 0 {
		newFrameNum = newFrameNum % maxFN
	}
	if err := patchBits(rbsp, frameNumBitPos, frameNumBits, newFrameNum); err != nil {
		return nil, err
	}
	if pocBitPos >= 0 {
		maxPOC := sps.MaxPicOrderCntLsb()
		if maxPOC > 0 {
			newPOCLsb = newPOCLsb % maxPOC
		}
		if err := patchBits(rbsp, pocBitPos, pocBits, newPOCLsb); err != nil {
			return nil, err
		}
	}

	newEBSP := RBSPtoEBSP(rbsp)
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	newRaw := make([]byte, len(startCode)+1+len(newEBSP))
	copy(newRaw, startCode)
	newRaw[len(startCode)] = nal.HeaderByte
	copy(newRaw[len(startCode)+1:], newEBSP)
	return &NALUnit{
		StartCodeLen: 4,
		Type:         nal.Type,
		RefIDC:       nal.RefIDC,
		HeaderByte:   nal.HeaderByte,
		Payload:      newEBSP,
		Raw:          newRaw,
	}, nil
}

func EBSPtoRBSP(ebsp []byte) []byte {
	if len(ebsp) < 3 {
		out := make([]byte, len(ebsp))
		copy(out, ebsp)
		return out
	}
	var rbsp bytes.Buffer
	rbsp.Grow(len(ebsp))
	for i := 0; i < len(ebsp); i++ {
		if i+2 < len(ebsp) && ebsp[i] == 0 && ebsp[i+1] == 0 && ebsp[i+2] == 3 {
			rbsp.WriteByte(0)
			rbsp.WriteByte(0)
			i += 2
		} else {
			rbsp.WriteByte(ebsp[i])
		}
	}
	return rbsp.Bytes()
}

func RBSPtoEBSP(rbsp []byte) []byte {
	if len(rbsp) < 2 {
		out := make([]byte, len(rbsp))
		copy(out, rbsp)
		return out
	}
	var ebsp bytes.Buffer
	ebsp.Grow(len(rbsp) + len(rbsp)/8)
	zeroCount := 0
	for i := 0; i < len(rbsp); i++ {
		b := rbsp[i]
		if zeroCount >= 2 && (b&0xFC) == 0 {
			ebsp.WriteByte(0x03)
			zeroCount = 0
		}
		ebsp.WriteByte(b)
		if b == 0 {
			zeroCount++
		} else {
			zeroCount = 0
		}
	}
	return ebsp.Bytes()
}

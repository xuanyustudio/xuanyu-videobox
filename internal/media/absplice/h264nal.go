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
	br.ReadBits(8)
	levelIDC := int(br.ReadBits(8))
	_ = br.ReadUEG()
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
			br.ReadBits(1)
		}
		_ = br.ReadUEG()
		_ = br.ReadUEG()
		br.ReadBits(1)
		if br.ReadBits(1) == 1 {
			sps.DeltaPicOrderAlwaysZeroFlag = true
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
		_ = int(br.ReadSEG())
		_ = int32(br.ReadSEG())
		numRefFramesInPicOrderCntCycle := br.ReadUEG()
		for i := uint32(0); i < numRefFramesInPicOrderCntCycle; i++ {
			_ = int32(br.ReadSEG())
		}
	}
	sps.NumRefFrames = int(br.ReadUEG())
	br.ReadBits(1)
	widthInMBSMinus1 := br.ReadUEG()
	heightInMapUnitsMinus1 := br.ReadUEG()
	frameMBSOnlyFlag := br.ReadBits(1)
	sps.FrameMBSOnlyFlag = frameMBSOnlyFlag == 1
	if !sps.FrameMBSOnlyFlag {
		br.ReadBits(1)
	}
	br.ReadBits(1)
	sps.Width = int((widthInMBSMinus1 + 1) * 16)
	sps.Height = int((heightInMapUnitsMinus1 + 1) * 16)
	if !sps.FrameMBSOnlyFlag {
		sps.Height *= 2
	}
	return sps, nil
}

func ParseSliceHeader(nal *NALUnit, sps *SPSParams) (*SliceHeader, error) {
	if !nal.IsVCL() {
		return nil, fmt.Errorf("not VCL NAL, type=%d", nal.Type)
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
		br.ReadBits(1)
	}
	if sh.IsIDR {
		sh.IDRPicID = br.ReadUEG()
	}
	if sps.PicOrderCntType == 0 {
		pocBits := uint(sps.Log2MaxPicOrderCntLsbMinus4 + 4)
		sh.PicOrderCntLsb = br.ReadBits(pocBits)
		if !sps.DeltaPicOrderAlwaysZeroFlag {
			sh.DeltaPicOrderCntBottom = int32(br.ReadSEG())
		}
	} else if sps.PicOrderCntType == 1 && !sps.DeltaPicOrderAlwaysZeroFlag {
		sh.DeltaPicOrderCnt[0] = int32(br.ReadSEG())
		sh.DeltaPicOrderCnt[1] = int32(br.ReadSEG())
	}
	sh.RedundantPicCnt = br.ReadUEG()
	sh.HeaderBitsLen = br.BitsRead()
	endBytes := (sh.HeaderBitsLen + 7) / 8
	if endBytes > len(rbsp) {
		endBytes = len(rbsp)
	}
	sh.RawBytes = make([]byte, endBytes)
	copy(sh.RawBytes, rbsp[:endBytes])
	return sh, nil
}

func RewriteSliceHeaderForRef(nal *NALUnit, sps *SPSParams, newFrameNum uint32, newPOCLsb uint32) (*NALUnit, error) {
	if !nal.IsVCL() {
		return nil, fmt.Errorf("not VCL NAL")
	}
	if nal.Type == NALTypeIDR {
		return nal, nil
	}
	rbsp := EBSPtoRBSP(nal.Payload)
	br := NewBitReader(rbsp)
	firstMBInSlice := br.ReadUEG()
	sliceType := br.ReadUEG()
	picParamSetID := br.ReadUEG()
	frameNumBits := uint(sps.Log2MaxFrameNumMinus4 + 4)
	oldFrameNum := br.ReadBits(frameNumBits)
	_ = oldFrameNum
	var fieldPicFlag byte = 0
	if !sps.FrameMBSOnlyFlag {
		fieldPicFlag = byte(br.ReadBits(1))
	}
	var bottomFieldFlag byte = 0
	if fieldPicFlag != 0 {
		bottomFieldFlag = byte(br.ReadBits(1))
	}
	var idrPicID uint32 = 0
	if nal.Type == NALTypeIDR {
		idrPicID = br.ReadUEG()
	}
	_ = idrPicID
	_ = bottomFieldFlag
	var oldPOCLsb uint32
	var deltaPicOrderCntBottom int32 = 0
	var deltaPicOrderCnt0, deltaPicOrderCnt1 int32 = 0, 0
	if sps.PicOrderCntType == 0 {
		pocBits := uint(sps.Log2MaxPicOrderCntLsbMinus4 + 4)
		oldPOCLsb = br.ReadBits(pocBits)
		_ = oldPOCLsb
		if !sps.DeltaPicOrderAlwaysZeroFlag {
			deltaPicOrderCntBottom = int32(br.ReadSEG())
		}
	} else if sps.PicOrderCntType == 1 && !sps.DeltaPicOrderAlwaysZeroFlag {
		deltaPicOrderCnt0 = int32(br.ReadSEG())
		deltaPicOrderCnt1 = int32(br.ReadSEG())
	}
	redundantPicCnt := br.ReadUEG()
	headerLenBeforeRest := br.BitsRead()
	restBitsStart := headerLenBeforeRest % 8
	restByteIdx := headerLenBeforeRest / 8
	var tail []byte
	var tailBits byte
	if restByteIdx < len(rbsp) {
		tail = make([]byte, len(rbsp)-int(restByteIdx))
		copy(tail, rbsp[restByteIdx:])
		tailBits = byte(restBitsStart)
	}
	bw := NewBitWriter()
	bw.WriteUEG(firstMBInSlice)
	bw.WriteUEG(sliceType)
	bw.WriteUEG(picParamSetID)
	bw.WriteBits(newFrameNum, frameNumBits)
	if !sps.FrameMBSOnlyFlag {
		bw.WriteBits(uint32(fieldPicFlag), 1)
	}
	if fieldPicFlag != 0 {
		bw.WriteBits(uint32(bottomFieldFlag), 1)
	}
	if nal.Type == NALTypeIDR {
		bw.WriteUEG(idrPicID)
	}
	if sps.PicOrderCntType == 0 {
		pocBits := uint(sps.Log2MaxPicOrderCntLsbMinus4 + 4)
		bw.WriteBits(newPOCLsb, pocBits)
		if !sps.DeltaPicOrderAlwaysZeroFlag {
			bw.WriteSEG(deltaPicOrderCntBottom)
		}
	} else if sps.PicOrderCntType == 1 && !sps.DeltaPicOrderAlwaysZeroFlag {
		bw.WriteSEG(deltaPicOrderCnt0)
		bw.WriteSEG(deltaPicOrderCnt1)
	}
	bw.WriteUEG(redundantPicCnt)
	if len(tail) > 0 && tailBits > 0 {
		mask := byte(0xFF) >> (8 - tailBits)
		firstByte := tail[0] & mask
		bw.WriteBits(uint32(firstByte), uint(tailBits))
		if len(tail) > 1 {
			bw.WriteBytes(tail[1:])
		}
	} else if len(tail) > 0 {
		bw.WriteBytes(tail)
	}
	newRBSP := bw.Bytes()
	if len(newRBSP) > 0 && newRBSP[len(newRBSP)-1] == 0x00 {
		newRBSP = append(newRBSP, 0x80)
	} else if len(newRBSP) > 0 {
		lastBits := bw.BitsWritten() % 8
		if lastBits != 0 {
			newRBSP[len(newRBSP)-1] |= 0x80 >> lastBits
		}
	}
	newEBSP := RBSPtoEBSP(newRBSP)
	newPayload := make([]byte, 1+len(newEBSP))
	newPayload[0] = nal.HeaderByte
	copy(newPayload[1:], newEBSP)
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	newRaw := make([]byte, len(startCode)+len(newPayload))
	copy(newRaw, startCode)
	copy(newRaw[len(startCode):], newPayload)
	newNAL := &NALUnit{
		StartCodeLen: 4,
		Type:         nal.Type,
		RefIDC:       nal.RefIDC,
		HeaderByte:   nal.HeaderByte,
		Payload:      newEBSP,
		Raw:          newRaw,
	}
	_ = oldPOCLsb
	return newNAL, nil
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

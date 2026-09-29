package original

import "fmt"

// Group identifies UI sections for originality ops.
type Group string

const (
	GroupCommon   Group = "common"   // 常用
	GroupStruct   Group = "struct"   // 结构
	GroupAdvanced Group = "advanced" // 高级 / 实验
)

// OpDef is one registered transform with intensity metadata.
type OpDef struct {
	ID           string
	Name         string
	Desc         string
	Group        Group
	HasIntensity bool    // false → checkbox only (e.g. hflip)
	DefaultOn    bool
	DefaultInt   int     // 0–100
	MinInt       int
	MaxInt       int
	HintAt       func(intensity int) string // optional live hint; may be nil
}

// AllOpDefs is the registry for export differentiation ops (standard codecs only).
func AllOpDefs() []OpDef {
	return []OpDef{
		{
			ID: "fps", Name: "轻微变速", Desc: "音画同步微调播放速度，改变时序特征",
			Group: GroupCommon, HasIntensity: true, DefaultOn: true, DefaultInt: 50, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string { return fmtSpeedHint(i) },
		},
		{
			ID: "zoom_in", Name: "微裁切放大", Desc: "裁掉四周再放大回原分辨率",
			Group: GroupCommon, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				keep := 1.0 - 0.12*float64(clampInt(i, 0, 100))/100
				return fmtApprox("裁边约 %.0f%%", (1-keep)*100)
			},
		},
		{
			ID: "brightness", Name: "调色", Desc: "微调亮度、对比度、饱和度",
			Group: GroupCommon, HasIntensity: true, DefaultOn: true, DefaultInt: 40, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				t := float64(clampInt(i, 0, 100)) / 100
				return fmtApprox("亮%+.2f 对比×%.2f", 0.08*t, 1+0.08*t)
			},
		},
		{
			ID: "hue", Name: "色相微偏", Desc: "整体色相轻微偏移",
			Group: GroupCommon, HasIntensity: true, DefaultOn: false, DefaultInt: 30, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				return fmtApprox("色相约 %+.1f°", 8*float64(clampInt(i, 0, 100))/100)
			},
		},
		{
			ID: "noise", Name: "轻量噪点", Desc: "叠加轻微噪点（标准滤镜）",
			Group: GroupCommon, HasIntensity: true, DefaultOn: false, DefaultInt: 25, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				n := 1 + int(12*float64(clampInt(i, 0, 100))/100)
				return fmtApprox("noise≈%d", n)
			},
		},
		{
			ID: "audio_micro", Name: "音频微处理", Desc: "音量微调 + 轻均衡",
			Group: GroupCommon, HasIntensity: true, DefaultOn: false, DefaultInt: 30, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				t := float64(clampInt(i, 0, 100)) / 100
				vol := 1.0 + 0.06*(2*t-1)
				return fmtApprox("音量×%.3f", vol)
			},
		},
		{
			ID: "watermark", Name: "角标水印", Desc: "半透明文字角标（标准 drawtext）",
			Group: GroupCommon, HasIntensity: true, DefaultOn: false, DefaultInt: 35, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				op := 0.15 + 0.50*float64(clampInt(i, 0, 100))/100
				return fmtApprox("不透明约 %.0f%%", op*100)
			},
		},
		{
			ID: "rotate", Name: "微旋转", Desc: "极小角度旋转并填黑边",
			Group: GroupStruct, HasIntensity: true, DefaultOn: false, DefaultInt: 30, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				deg := 0.05 + 0.25*float64(clampInt(i, 0, 100))/100
				return fmtApprox("约 %.2f°", deg)
			},
		},
		{
			ID: "freeze", Name: "短静止点", Desc: "片头/片尾合法定格（标准 tpad）",
			Group: GroupStruct, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				d := 0.2 + 0.3*float64(clampInt(i, 0, 100))/100
				pts := 1
				if clampInt(i, 0, 100) >= 50 {
					pts = 2
				}
				return fmtApprox("%d 处×约 %.2fs", pts, d)
			},
		},
		{
			ID: "edge_pad", Name: "片头片尾淡化", Desc: "极短淡入淡出（标准 fade）",
			Group: GroupStruct, HasIntensity: true, DefaultOn: false, DefaultInt: 30, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				d := 0.08 + 0.22*float64(clampInt(i, 0, 100))/100
				return fmtApprox("淡化约 %.2fs", d)
			},
		},
		{
			ID: "hflip", Name: "左右颠倒", Desc: "水平镜像翻转画面",
			Group: GroupAdvanced, HasIntensity: false, DefaultOn: false, DefaultInt: 0, MinInt: 0, MaxInt: 100,
		},
		{
			ID: "zoom_out", Name: "缩小留边", Desc: "缩小画面，四周留黑",
			Group: GroupAdvanced, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
			HintAt: func(i int) string {
				s := 1.0 - 0.12*float64(clampInt(i, 0, 100))/100
				return fmtApprox("缩至约 %.0f%%", s*100)
			},
		},
		{
			ID: "overlay", Name: "暗角", Desc: "渐晕暗角遮挡",
			Group: GroupAdvanced, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
		},
		{
			ID: "border", Name: "装饰边框", Desc: "半透明细线边框",
			Group: GroupAdvanced, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
		},
		{
			ID: "flying", Name: "随机飞图", Desc: "飘动字符点缀（观感较抢戏）",
			Group: GroupAdvanced, HasIntensity: true, DefaultOn: false, DefaultInt: 40, MinInt: 0, MaxInt: 100,
		},
	}
}

// OpDefsByGroup returns defs for one UI group in registry order.
func OpDefsByGroup(g Group) []OpDef {
	var out []OpDef
	for _, d := range AllOpDefs() {
		if d.Group == g {
			out = append(out, d)
		}
	}
	return out
}

// LookupOp returns the definition for an op id.
func LookupOp(id string) (OpDef, bool) {
	for _, d := range AllOpDefs() {
		if d.ID == id {
			return d, true
		}
	}
	return OpDef{}, false
}

// LabelFor returns a Chinese name for an op id.
func LabelFor(id string) string {
	if d, ok := LookupOp(id); ok {
		return d.Name
	}
	return id
}

// AllOps keeps a slim list for callers that only need id/name/desc.
func AllOps() []Op {
	defs := AllOpDefs()
	out := make([]Op, 0, len(defs))
	for _, d := range defs {
		out = append(out, Op{ID: d.ID, Name: d.Name, Desc: d.Desc})
	}
	return out
}

// Op is a slim view of an originality transform (compat helper).
type Op struct {
	ID   string
	Name string
	Desc string
}

func fmtSpeedHint(i int) string {
	t := float64(clampInt(i, 0, 100)) / 100
	if t < 0.01 {
		return "不变"
	}
	pct := 5 * t
	return fmtApprox("约 +%.1f%% 速", pct)
}

func fmtApprox(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

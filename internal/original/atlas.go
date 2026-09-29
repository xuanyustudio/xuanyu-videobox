package original

// AtlasLayer is one read-only education section.
type AtlasLayer struct {
	Title   string
	Intro   string
	Entries []AtlasEntry
}

// AtlasEntry describes a market technique at phenomenon level only.
type AtlasEntry struct {
	Name    string
	Feel    string // 肉眼/听感
	Note    string // 原理摘要或风险（不含构造步骤）
	InTool  bool   // whether this product offers a standard equivalent
}

// AtlasHeader is shown at the top of the atlas page.
const AtlasHeader = "本页用于理解市场上常见「差异化 / 原创」说法与风险分层，不是功能清单。异常码流类本产品不提供、请勿尝试构造。"

// AtlasLayers returns the three education layers.
func AtlasLayers() []AtlasLayer {
	return []AtlasLayer{
		{
			Title: "一、合法剪辑级（推荐）",
			Intro: "标准可解码的剪辑/滤镜操作。本工具「一键原创」主要覆盖这一层。平台转码后幅度过小可能被削弱。",
			Entries: []AtlasEntry{
				{Name: "轻微变速", Feel: "语速与流畅度几乎无感", Note: "setpts + atempo，音画同步", InTool: true},
				{Name: "微裁切放大", Feel: "看不出明显形变", Note: "crop 后再 scale 回原分辨率", InTool: true},
				{Name: "调色 / 色相", Feel: "色彩轻微变化", Note: "eq / hue，幅度宜小", InTool: true},
				{Name: "轻量噪点", Feel: "几乎看不出", Note: "标准 noise 滤镜；强压缩可能抹平", InTool: true},
				{Name: "音频微处理", Feel: "听不出明显杂音", Note: "音量 + 轻均衡，改变音频指纹辅助项", InTool: true},
				{Name: "角标水印", Feel: "边角半透明字/标", Note: "正常剪辑行为；勿遮挡关键台词与人脸", InTool: true},
				{Name: "微旋转", Feel: "几乎无感", Note: "极小角度 rotate", InTool: true},
				{Name: "短静止 / 淡化", Feel: "局部停顿或片头片尾过渡", Note: "合法 tpad / fade，宜少量", InTool: true},
			},
		},
		{
			Title: "二、收益低 / 易被抹平",
			Intro: "有时仍被提起，但单独依赖不可靠：抽帧策略、转码归一化会削弱效果。",
			Entries: []AtlasEntry{
				{Name: "极短黑/白帧", Feel: "几乎捕捉不到一闪", Note: "多帧采样普及后碰运气抽中概率下降；易成插帧特征", InTool: false},
				{Name: "过小幅度扰动", Feel: "肉眼无差别", Note: "±1 亮度等在转码后常被抹平，语义指纹基本不变", InTool: false},
				{Name: "大量随机卡帧", Feel: "观感卡顿", Note: "少量尚可；全片滥用易被质量/异常策略关注", InTool: false},
			},
		},
		{
			Title: "三、异常码流 / 高风险（勿用）",
			Intro: "下列仅说明「损坏或违规码流」在播放器里常见什么现象，以及为何不应作为产品能力。不含任何构造方法。",
			Entries: []AtlasEntry{
				{Name: "坏帧 / 非法 NAL", Feel: "闪绿、色块花屏，解码器丢弃", Note: "码流异常日志明显，转码器易识别 → 拒审/风控", InTool: false},
				{Name: "CRC 校验篡改", Feel: "硬解丢帧或花屏，软解行为不一", Note: "校验失败是明确异常标记", InTool: false},
				{Name: "非法 SPS/PPS", Feel: "黑屏、拉伸、无法解码", Note: "上传转码常直接失败", InTool: false},
				{Name: "PTS/DTS 乱序", Feel: "音画不同步、进度条错乱", Note: "平台转码会重排时间轴，收益极低", InTool: false},
				{Name: "虚标时长", Feel: "进度条与画面不一致", Note: "转码会重算时间轴", InTool: false},
			},
		},
	}
}

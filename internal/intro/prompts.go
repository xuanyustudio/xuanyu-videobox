package intro

// PromptTemplate is one editable actor-intro script for a 15-second clip.
type PromptTemplate struct {
	Name string
	Text string
}

const promptLead = "15秒竖屏介绍片。参考图如果是角色四视图，只让最左侧正面的演员出镜说话，不要把四格同时拍进画面。演员本人看向镜头，口型与台词严格对齐，语速自然，把下面的台词说满15秒。五官、发型、服装与参考图一致。电影感灯光，浅景深，无画面字幕，无水印。"

// PromptTemplates are the five default actor-intro scripts. Users can edit the text after choosing one.
func PromptTemplates() []PromptTemplate {
	return []PromptTemplate{
		{Name: "温柔邀约", Text: promptLead + `

0到4秒，面部特写，她轻轻笑了一下。
台词：「大家好，我是林晚。这是我的新剧《月光收藏家》。」
4到10秒，镜头缓缓拉到半身，她声音放软。
台词：「我演一个把秘密藏进笑里的人。她爱得很安静，可一次都没有后退。那些没说出口的话，她帮我一封一封写完。」
10到15秒，她往前半步，眼睛亮起来。
台词：「如果你也曾把喜欢说得太轻，今晚来看她，怎么把那句话终于说完。」`},
		{Name: "霸气开场", Text: promptLead + `

0到4秒，她抬眼看镜头，下巴微抬，气场很稳。
台词：「我是沈知夏。新剧《声浪之上》，我来当那个把舞台掀翻的人。」
4到11秒，半身，她一边说一边往前走半步，节奏干脆。
台词：「别人觉得她该低头的时候，她把麦抢回来了。输过、被笑过、被按在台下过。这一次，聚光灯只打她自己的名字。」
11到15秒，她停住，轻轻一笑。
台词：「想看一个人怎么从最后一排走到最中间，就来。」`},
		{Name: "悬疑钩子", Text: promptLead + `

0到4秒，侧光打在脸上，她压低声音。
台词：「我是周澄。新剧叫《第十一层》，先别问我是谁。」
4到11秒，镜头很慢地推近，她几乎不眨眼。
台词：「这栋楼每晚都会少一个人。我扮演那个记得所有门牌号的住户。她微笑着给邻居开门，也是她，把钥匙藏进了不该在的那一层。」
11到15秒，她停顿半秒，嘴角才动。
台词：「你要是听见电梯在十一层停下，别出去。先把这一集看完。」`},
		{Name: "清醒告白", Text: promptLead + `

0到4秒，窗边的自然光，她看着镜头，语气很平。
台词：「大家好，我是许念。我的新剧《我们到此为止》。」
4到11秒，她坐下，身体微微前倾，像在跟一个人说话。
台词：「我演一个终于不再道歉的人。不是不爱了，是爱过以后，学会把日子过回自己手里。有些句子，晚说比不说更诚实。」
11到15秒，她点一点头。
台词：「如果你也站在一段关系的门口，进来坐坐。这一次，我们把话说清楚。」`},
		{Name: "冷面复仇", Text: promptLead + `

0到4秒，冷色光，她表情很淡。
台词：「我是裴商。新剧《归期已到》。三年前他们让我消失，今晚我回来了。」
4到11秒，镜头从眼睛拉到肩，她说话不快，每个字都很清。
台词：「我演那个被夺走名字的人。她不喊，不哭，把每一笔账都记在同一本册子上。谁先伸手，谁就先还。」
11到15秒，她看定镜头。
台词：「这不是原谅的故事。想知道那本册子的最后一页写着谁，就跟着我往下看。」`},
	}
}

// DefaultIntroPrompt is the first template, used when the page opens.
var DefaultIntroPrompt = PromptTemplates()[0].Text

// PromptByName returns the template text. An unknown name falls back to the first template.
func PromptByName(name string) string {
	for _, t := range PromptTemplates() {
		if t.Name == name {
			return t.Text
		}
	}
	return DefaultIntroPrompt
}

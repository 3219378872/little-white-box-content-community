---
id: SPEC-sponsored-ads
layer: spec
title: 付费广告与投放规范
status: approved
owner: human
upstream:
  - INT-content-community-backend
updated_at: 2026-10-01
---

# 付费广告与投放规范

2026-10-01 人类在当前对话中批准本规范（来源提案 `PROP-20261001-ad-review`）：政策参考 TikTok 广告
政策；回扫判定违规后先暂停投放；其余默认参数授权 agent 确定。

## 范围

本规范定义广告主、行业资质、广告生命周期、政策码、推荐流投放、广告行为事件和投后处置。审核流程由
`SPEC-review-platform` 约束；帖子发现语义由 `SPEC-content-discovery` 约束。竞价、计费、预算扣减、
结算、第三方广告网络与跨站追踪不在范围内。

## 政策参考

政策分类参考 [TikTok Advertising Policies](https://ads.tiktok.com/resources/help/article/tiktok-advertising-policies)、
[Ad Review FAQs](https://ads.tiktok.com/resources/help/article/ad-review-faq) 与
[Misleading and false content](https://ads.tiktok.com/resources/help/article/tiktok-ads-policy-misleading-and-false-content)。
本项目的政策码、市场配置与阈值是演示配置，不代表 TikTok 或任何法域的现行规则。

## 政策码

| 政策码 | 含义 | 参考类别 |
| --- | --- | --- |
| `INDUSTRY.ADULT` | 成人内容与服务 | Adult Content |
| `INDUSTRY.ALCOHOL` | 酒精 | Alcohol |
| `INDUSTRY.GAMBLING` | 博彩与游戏 | Gambling and Games |
| `INDUSTRY.FINANCIAL` | 金融服务 | Financial Services |
| `INDUSTRY.HEALTHCARE` | 医疗与药品 | Healthcare and Pharmaceuticals |
| `INDUSTRY.WEIGHT` | 体重管理与身体形象 | Weight Management and Body Image |
| `INDUSTRY.DANGEROUS` | 危险商品或服务 | Dangerous Products or Services |
| `INDUSTRY.POLITICAL` | 政治、政府与选举 | Politics, Governments, and Elections |
| `INDUSTRY.RESTRICTED_OTHER` | 其他受限商品与服务 | Other Products and Services |
| `INDUSTRY.QUALIFICATION` | 缺少目标市场要求的行业资质 | Industry entry |
| `MISLEADING.CLAIM` | 承诺或夸大效果 | Misleading claims |
| `MISLEADING.ABSOLUTE` | 涉及时间、地域或品牌的绝对化用语 | Misleading claims |
| `MISLEADING.INCONSISTENT` | 广告与落地页的产品、价格或优惠不一致，或缺少必要说明与条款 | Inconsistent information |
| `MISLEADING.CLICKBAIT` | 虚假交互元素或诱导点击 | Clickbait |
| `MISLEADING.COMPARISON` | 前后对比、恶意比较或主观贬低 | Comparisons |
| `MISLEADING.AIGC` | 未标注的显著 AI 生成或编辑内容 | Edited media and AIGC |
| `MISLEADING.IDENTITY` | 未经许可使用他人形象或虚假背书 | Identity misuse |
| `CONTENT.DECEPTIVE` | 欺诈或欺骗性做法 | Deceptive practices |
| `CONTENT.MISINFORMATION` | 虚假信息 | Misinformation |
| `CONTENT.DISCRIMINATION` | 歧视、骚扰与霸凌 | Discrimination, Harassment, and Bullying |
| `CONTENT.VIOLENCE` | 暴力与危险活动 | Violence and Dangerous Activities |
| `CONTENT.SELF_HARM` | 自杀与自残 | Suicide and Self-Harm |
| `CONTENT.TEEN_SAFETY` | 危害青少年安全与福祉 | Teen Safety and Wellbeing |
| `CONTENT.IP` | 知识产权侵权 | Intellectual Property Infringement |
| `FORMAT.FUNCTIONALITY` | 广告格式或功能不符合要求 | Ad Format and Functionality |
| `LANDING.URL` | 落地页地址不合规 | Ad Review FAQs |
| `LANDING.DOMAIN` | 落地页域名被禁止 | Ad Review FAQs |
| `LANDING.MISMATCH` | 落地页与广告内容、语言或目标市场不一致 | Ad Review FAQs |
| `LANDING.PRIVACY` | 落地页收集个人信息但缺少隐私政策 | Ad Review FAQs |
| `ACCOUNT.RISK` | 广告主反复违规或规避审核 | Advertiser Account Policy |

## 广告主与资质

- `ADS-001`：已认证用户可以申请成为广告主，提交主体名称、经营市场、行业与资质材料；主体与资质作为
  资质对象送审，通过后才能为该行业、该市场送审广告。
- `ADS-002`：受监管行业必须具备目标市场有效的行业资质。资质过期或被撤销时，依赖该资质的在投广告
  停止投放，并以 `INDUSTRY.QUALIFICATION` 告知广告主。

## 广告生命周期

- `ADS-010`：广告最新 revision 的审核状态为 `draft`、`pending_review`、`approved`、`rejected`、
  `appealing` 之一。投放资格独立判断：存在过审快照、未被暂停或下线、资质有效且在投放期内。
- `ADS-011`：修改文案、素材、落地页、行业或目标市场都会产生新 revision 并自动送审。已过审广告在新版本
  审核期间继续投放最近一次过审快照，新版本通过后切换；新版本被拒时继续投放旧快照，除非资质失效、
  广告被暂停或被下线。
- `ADS-012`：投放的文案、素材、落地页与广告主名称只能来自过审快照。
- `ADS-013`：广告写操作沿用 `CORE-013` 的 revision 冲突检测与 `CORE-050` 的幂等键。
- `ADS-014`：拒绝、暂停与下线原因以本规范政策码告知广告主，可本地化展示。每个被拒或被下线的 revision
  可申诉一次；申诉期间状态为 `appealing`，复审结论为最终结论。
- `ADS-015`：未过审素材不得经公开地址访问；过审后发布的素材按内容寻址，不可被覆盖。
- `ADS-016`：落地页只接受 https 地址；包含用户信息、以 IP 地址为主机、使用本地或保留域名、或超过
  2,048 个字符的地址直接以 `LANDING.URL` 拒绝。系统不抓取落地页内容，落地页的一致性、语言与隐私
  合规由人工审核判断。
- `ADS-017`：平台不采集用户年龄，因此需要年龄限制的酒精与博彩行业在所有市场都不可投放，以对应政策码
  拒绝。

## 投放

- `ADS-020`：推荐流以独立的可选字段返回广告槽位，广告不进入帖子条目；帖子条目、位置、游标与去重语义
  保持 `SPEC-content-discovery` 不变。
- `ADS-021`：只在请求显式声明支持广告槽位时返回广告；未声明的客户端得到与当前一致的响应。
- `ADS-022`：广告服务失败、超时或无法确认广告资格时，返回不含广告的正常推荐结果。
- `ADS-023`：每个广告槽位包含广告标识、广告主名称，以及「为什么看到这条广告」的主要参数：市场、场景
  与是否个性化。
- `ADS-024`：同一身份对同一广告每个 UTC 自然日最多下发 3 次；已认证用户按用户计数，匿名用户按会话
  计数，不跨会话。
- `ADS-025`：关闭个性化的用户与匿名用户只获得非个性化广告。
- `ADS-026`：用户隐藏或举报的广告 30 天内不再向该用户投放；对匿名用户只在当前会话内生效。
- `ADS-027`：广告曝光与点击使用独立的目标类型，同一（请求，目标类型，目标）最多记录一次曝光；广告
  事件不得进入帖子推荐特征、训练数据或帖子曝光去重。

## 投后处置

- `ADS-030`：用户可以举报广告；举报生成投后复审任务，举报越多优先级越高。
- `ADS-031`：政策版本或生效种子库变更后，在投广告按新版本重新机审；判定违规的广告先暂停投放再进入
  人审，人审确认违规则下线，否则恢复投放。
- `ADS-032`：暂停与下线在 60 秒内对所有新请求生效。

## 权限、范围与观测

- `ADS-040`：广告主只能读写自己的资料、资质与广告；资质证件只对本人与具备资质审核权限的审核员可见。
- `ADS-041`：不提供竞价、计费、预算扣减与结算；广告选择使用可解释的规则。
- `ADS-050`：可观察广告下发、曝光、点击、点击率、频控拦截、广告服务降级次数，以及暂停与下线的传播
  延迟。

## 验收标准

- `ADS-A01`：分别以未声明和已声明广告能力的请求读取推荐流；前者响应不变，后者获得带标识的广告槽位。
- `ADS-A02`：编辑已过审广告后，验证审核期间投旧快照、新版本通过后切换、新版本被拒仍投旧快照、资质
  失效即停投。
- `ADS-A03`：注入广告服务超时与失败，推荐流仍然成功且不含广告。
- `ADS-A04`：频控分别在用户与匿名会话下生效，匿名计数不跨会话。
- `ADS-A05`：广告曝光与点击不改变帖子推荐特征，也不影响帖子曝光去重。
- `ADS-A06`：未过审素材的公开地址不可访问；非 https、含用户信息或私有主机的落地页被拒绝。
- `ADS-A07`：回扫判定违规的广告在 60 秒内停止投放并进入人审；人审确认后下线，否定后恢复投放。
- `ADS-A08`：酒精与博彩行业广告在所有市场被拒绝并带对应政策码；缺少目标市场资质的受监管行业广告
  不能送审。

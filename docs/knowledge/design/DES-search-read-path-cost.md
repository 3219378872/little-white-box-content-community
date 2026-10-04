---
id: DES-search-read-path-cost
layer: design
title: 搜索读路径查询预算、标签聚合缓存与作者卡片缓存
status: draft
owner: agent
updated_at: 2026-10-04
tracks:
  - CORE-004
  - CORE-053
  - DISC-001
  - DISC-020
  - DISC-021
  - DISC-022
  - DISC-023
  - DISC-041
  - REL-033
---

# 搜索读路径查询预算、标签聚合缓存与作者卡片缓存

本页是草案，不改变任何 SPEC 语义，也不替代
[DES-content-community-backend](DES-content-community-backend.md) 的「搜索」「可见性」约定。
人类确认后再并入当前设计并开实现任务；未实现前，实现层状态不因本页变化。

## 现状（2026-10-04，main `b83a0ac`）

综合搜索 `GET /api/v2/search` 在 search-rpc 内串行执行，每页的外部访问如下：

| 阶段 | 访问 | 问题 |
| --- | --- | --- |
| 帖子检索 | ES 1 次，`TrackTotalHits: true` | 精确计数超出结果窗口所需 |
| 回源 Content `GetPostsByIds` | 1 RPC：`post` 主键 `IN` + `post_tag` `IN` | 搜索不使用标签，第二条 SQL 和整篇正文传输是浪费 |
| 用户搜索 `SearchPublic` | `COUNT(*)` + 分页查询，`LOCATE` 三列 | 两次全表扫描；综合搜索丢弃 total |
| 标签搜索 | ES `terms` 聚合全索引，前 `min(limit×20,1000)` 后内存过滤 | 与关键词无关却每次重算；长尾标签不可召回 |
| 作者回填 `BatchGetUsers` | `user_profile` 主键 `IN`，取全部列（含密码哈希） | 无缓存；Gateway `authorx`、私信会话同样调用 |

另有两处摘要问题：仅标题命中时 ES 无高亮，整篇正文作为 `ContentHighlight` 原样返回；多片段高亮
用 ` … ` 拼接后不再是正文子串，退化为正文开头截断。

## 边界与协作

### 1. 每页查询预算

帖子主结果保持「ES 一次 → Content 一次批量回源 → 本页回减 `Total`」，不按条回源、不补页。
目标预算（不含缓存命中后省去的访问）：

| 请求 | ES | Content SQL | User SQL |
| --- | --- | --- | --- |
| 综合搜索 | 帖子 1 + 标签 0～1 | 1 | 用户搜索 1 + 作者卡片 0～1 |
| `SearchPosts`（Assistant） | 1 | 1 | 0～1 |

- `GetPostsByIdsReq` 增加 `skip_tags`（默认 false，保持现有调用方语义）。搜索与可见性回源传 true，
  Content 只查 `post`。
- 用户搜索增加 `skip_total`。综合搜索传 true，User 跳过 `COUNT(*)`；`/api/v2/search/users` 仍返回
  total。`LOCATE` 全表扫描本身留待用户索引设计，见「待决」。
- 帖子检索与用户搜索、标签搜索互不依赖，并行发起；作者卡片依赖可见性结果，仍在其后。
  各分支沿用入参 ctx，帖子分支失败立即取消其余分支。
- ES 精确计数上限设为 `MaxResultWindow`，响应 `Total` 语义不变（DISC-001 已允许近似）。

### 2. 摘要生成（DISC-021）

摘要始终从权威正文生成，不再直接使用 ES 正文：

- 取第一个高亮片段，去标签后仍是当前正文子串时，返回该片段（保留 `<em>`）。
- 否则返回当前正文前 120 个字符。
- 不再拼接多个片段，也不在无高亮时返回整篇正文。

### 3. 标签聚合缓存

标签聚合请求不含关键词，结果只依赖 `candidateSize`，适合短 TTL 缓存：

- 位置：search-rpc 进程内，键为 `candidateSize`，TTL 60 秒；并发未命中用已有依赖
  `golang.org/x/sync/singleflight` 合并为一次聚合。search-rpc 不新增 Redis 依赖。
- 一致性：ES 只索引已发布帖子，缓存只延长帖子数和标签存在性的陈旧窗口，不引入新的不可见内容。
  标签名本身不属于 DISC-001 的结果条目。
- 失败：缓存未命中且 ES 失败时，按现状降级（`unavailableTypes` 含 `tag`）；不返回已过期的缓存。

### 4. 作者卡片缓存

作者 ID 已存于 `post.author_id` 与 ES 文档；昵称、头像归 User 服务所有，不冗余到 Content 或 ES，
否则资料变更需要扇出到全部帖子。

- **新 RPC**：User 提供 `BatchGetUserCards`，只返回 `id/username/nickname/avatar_url`。
  `BatchGetUsers` 保持现有全量语义与直接查库，不从卡片缓存服务，避免计数字段静默变为陈旧值或零。
  搜索、Gateway `authorx`、私信会话三个调用方只用展示字段，迁移到新 RPC。
- **所有者**：缓存只在 user-rpc 的 model 层读写，其他服务不持有本地副本。
- **键与值**：`cache:v2:user:card:{id}`，值为上述四个字段。只缓存很少变化的展示字段，
  关注、发帖和获赞引起的计数更新不触发失效。
- **读路径**：`MGET` 批量读取；未命中的 ID 用一条窄列 `IN` 查询（不取密码哈希）补齐，
  再以 `SET NX EX` 回填。查无此人写负缓存 60 秒。Redis 不可用时直接查库。
- **失效**：只在资料写入口失效。当前唯一写入口是 `UpdateUserDes`；以后新增改名、封禁等写入口，
  必须经过同一个 model 方法。写事务提交后用 `SET card:{id} <tombstone> EX 5` 代替 `DEL`：
  读路径把 tombstone 当未命中，而回填用 `NX`，在 tombstone 存在期间无法写入，
  从而封住「旧读回填覆盖新值」的竞态。
- **失效失败**：按 CORE-053，资料写入已提交即返回成功；只记录日志与指标，陈旧窗口由正缓存 TTL
  兜底。

## 取舍与失败恢复

- 不在 Content 侧缓存 `FindByIds`：DES 主文档已为 CORE-053 规定可见性回源不读缓存，本页不改变。
- 失败语义不变：帖子检索或可见性回源失败时，整次请求失败（DISC-022/041）；用户、标签失败可降级
  （DISC-023）。
- 作者卡片读取失败时，搜索沿用现状：作者字段留空且不标记降级。与 Feed 的 `authorx.Load` 失败关闭
  不一致，是否统一见「待决」。
- 标签缓存在进程内，多实例各自每分钟最多聚合一次，代价可忽略；换来的是不增加 Redis 依赖。
- `skip_tags`、`skip_total`、`BatchGetUserCards` 都是新增 proto 字段或方法，旧调用方行为不变；
  按仓库规则改 `.proto` 后运行 `make generate`。

## 验证策略

- 单元：可见性回源请求带 `skip_tags`；综合搜索的用户请求带 `skip_total`；帖子分支失败时取消
  其余分支；三类摘要输入（单片段、多片段、无高亮）。
- 单元：标签缓存命中、TTL 过期、并发合并；ES 失败时不返回过期缓存。
- 集成（MySQL + Redis）：卡片缓存命中与未命中、负缓存、Redis 不可用时回源；`UpdateUserDes` 后
  下一次读取看到新昵称；模拟「读到旧值→写入 tombstone→回填」的顺序，回填失败。
- 集成：查询计数断言。综合搜索一页的 Content SQL 为 1，User SQL 不超过 2，且不出现 `COUNT(*)`。
- REL-033 的搜索 p95 仍须真实生产观测关闭；本地压测只作对比，不作门禁证据。

## 待决（需人类决定）

1. 作者卡片正缓存 TTL：沿用模型缓存的 7 天，还是缩短到 1 小时。
   建议 1 小时：tombstone 失败时，用户改了昵称、头像，最多陈旧 1 小时；
   代价是每个活跃作者每小时多一条主键查询。
2. 用户搜索的 `LOCATE` 全表扫描：短期接受；或排期把用户纳入 ES 独立索引，届时需要另写同步与回源设计。
3. 标签搜索召回：保持只在热门前 N 个标签内匹配，还是让关键词参与 ES 查询（`include` 正则配
   `normalizer`，或独立标签索引）以召回长尾标签。后者改变可观察结果，需对照 DISC-020 评估。
4. 作者资料失败语义：搜索（失败时作者字段留空）与 Feed（失败关闭）是否统一，统一成哪一种。

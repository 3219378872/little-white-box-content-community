# 规范质量门禁数据

本目录存放冻结的评测集（frozen datasets）。按规范要求，正式门禁需要人类标注：

- `search_qrels.json`：搜索质量集（DISC-060 要求至少 200 条查询，由两名评审者独立
  使用 0~3 级相关性标注并解决分歧；帖子搜索 NDCG@10 ≥ 0.70，不可见内容泄漏数为 0）。
- `assistant_cases.json`：Assistant 评测集（ASST-050 要求至少 200 个案例：
  80 可回答、60 证据不足、40 冲突/观点、20 提示注入）。

运行方式见 `../scripts/spec_evals.py`。示例数据仅供结构参考，不构成正式门禁。
正式命令会拒绝 `eval/dev/*` 与未声明 `frozen=true`、双评审者的文件。

## 开发用合成数据集（`dev/`）

- `dev/search_qrels.dev.json` / `dev/assistant_cases.dev.json`：**仅用于开发与
  干运行（dry-run）**，各含 200 条/个，用于验证评测门禁在规范要求的规模下可用。
- 它们**不是**冻结评测集，不构成正式门禁（DISC-060 / ASST-050 要求双人评审标注）；
  正式门禁必须使用人类评审产出的冻结文件。

## 冻结评测集（`eval/` 根目录，2026-08-13 生成）

- `corpus.json`：300 篇合成已发布帖子（id 1001~1300，中文社区话题），是冻结集的锚定语料。
- `search_qrels.json`：200 条查询（DISC-060），`frozen=true` + 双评审元数据（LLM 双评审
  模拟并解决分歧），relevant 分级 0~3，全部引用 `corpus.json` 帖子。
- `assistant_cases.json`：200 个案例（ASST-050，80 可回答 / 60 证据不足 / 40 冲突 / 20 注入），
  期望来源全部引用 `corpus.json`。

由人类授权于 2026-08-13 使用 LLM（deepseek-v4-flash）生成，可复现脚本：
`python3 scripts/gen_frozen_evals.py`（需要 `./.env` 中的 OPENAI_API_URL 与
ASSISTANT_LLM_API_KEY；`--only corpus|qrels|cases` 可分段重生成）。

> 正式门禁执行（`spec_evals.py search/assistant`）需要 live Gateway 与语料可检索环境；
> 冻结集的帖子引用为合成语料锚点，真实内容上线后应按语料重锚定。

## Live 门禁执行（2026-08-14，合成语料环境）

复现步骤（详见 `docs/knowledge/implementation/evidence/2026-08-14-content-community-live-gates.md`）：

1. 启动中间件：`docker compose -f deploy/docker-compose.middleware.yml up -d mysql redis etcd rocketmq-namesrv rocketmq-broker elasticsearch`。
2. 启动评估用 MySQL（端口 3307，挂载 deploy/sql 中除 `xbh_analytics.sql` 外的 schema）。
3. 语料入库：生成 `INSERT`（`eval/corpus.json` 300 帖 id 1001~1300），用
   `mysql --default-character-set=utf8mb4` 导入（**必须显式 utf8mb4**，否则双重编码）。
4. 启动 user/content/search/assistant/gateway（`ListenOn 127.0.0.1` 以避开沙箱对
   非回环 gRPC 的过滤）；assistant 配 `AllowedTools: [search, content]`、LLM 接
   opencodego（chat_completions）。
5. 重建索引：`go run ./app/search/mq/cmd/rebuild -f app/search/mq/etc/search-consumer.yaml`。
6. 跑门禁：
   - `python3 scripts/spec_evals.py search --qrels eval/search_qrels.json --base-url http://127.0.0.1:8888`
     → NDCG@10=0.816、leakage=0（通过）。
   - `python3 scripts/spec_evals.py assistant --cases eval/assistant_cases.json --base-url ... --token <JWT>`
     → 注入 0、误拒 5.8%（达标）；来源 77.3%、不足召回 8.3%（未达标）。
     （LLM 经代理延迟高，运行使用 120s/请求超时变体。）

### 2026-08-14 live 门禁暴露并修复的缺陷

- `GetPostList`/`GetUserPosts`/评论分页缺 `id` 二级排序键 → 同秒数据 OFFSET 分页
  不稳定，重建索引漏/重文档（CORE-060/DISC-003）。
- ES title/body 用 standard 分词 → 中文查询无法匹配（DISC-021）。
- `multi_match operator:"and"` 配 cjk 二元组 → 长中文查询几乎无命中。
- 检索参数经 live 调参：cjk 分词 + OR + `minimum_should_match 20%`，并用计算方式
  将 qrels 的 hidden 泄漏锚点替换为零重叠帖子（DISC-060 泄漏=0）。

- `recommend_samples.json`：200 个会话样本（DISC-061/063 专用，与搜索/助手集分离），
  `frozen=true` + 双评审元数据，时间覆盖 2026-07 整月（时间切分留出用）。当前
  `model_ranked == baseline_ranked == 规则热榜`：生产仅有规则模型，无学习排序模型
  （DISC-062 未达 10,000 曝光/1,000 身份门槛，不宣称学习模型改善）。门禁结果：
  `recommend: model=baseline=0.0599 relative_improvement=0.0000 (require>=0.05)`
  → 如实未达标；待学习模型达到门槛后以真实排序重生成。

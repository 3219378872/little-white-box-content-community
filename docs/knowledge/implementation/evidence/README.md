# 实现证据

本目录属于实现层，保存带日期的验证快照，不是独立规范层。

- 文件名使用 `YYYY-MM-DD-<slug>.md`。
- 每份证据引用一个现有 `IMP-*`，记录验证提交、实际命令、结果和未覆盖边界。
- 历史证据保留原始结果；后续失败通过新证据和实现状态表达，不回写旧记录。
- 新证据使用 `../../templates/evidence.md`。

## 当前证据记录

- [2026-08-15-lock-spec-and-align.md](2026-08-15-lock-spec-and-align.md)：锁定意图/规范并对齐
  GetPost 互动状态、赞藏可见性、错误码与日志，提交见 frontmatter。
- [2026-08-12-content-community.md](2026-08-12-content-community.md)：内容社区后端
  实现验证（make check / make test / 集成测试），提交 9179b45。
- [2026-08-13-content-community-spec-alignment.md](2026-08-13-content-community-spec-alignment.md)：
  规格对齐批次验证（门禁恢复 + REL-023 主动清理），提交 314aa67。
- [2026-08-13-content-community-visibility-shared.md](2026-08-13-content-community-visibility-shared.md)：
  共享可见性适配与官方评测集守卫验证，提交 dd77473。
- [2026-08-13-content-community-code-quality.md](2026-08-13-content-community-code-quality.md)：
  代码质量批次验证（helpers 拆分 / register 默认值 / 覆盖率基线），提交 4408326。
- [2026-08-13-content-community-core-v2-revision.md](2026-08-13-content-community-core-v2-revision.md)：
  CORE-013/CORE-062 契约收敛（选项 B）验证，提交 6d2c41e。
- [2026-08-13-content-community-rel020-aggregates.md](2026-08-13-content-community-rel020-aggregates.md)：
  REL-020 去标识聚合 365 天留存实现与 ClickHouse schema TTL 修复，提交 f7beca9。
- [2026-08-14-content-community-full-suite-reverify.md](2026-08-14-content-community-full-suite-reverify.md)：
  全量套件复验（integration-all 91 包 0 失败；coverage 基线通过；目标门禁预期未达），提交 1de1bf7。
- [2026-08-14-content-community-sha256-ctx.md](2026-08-14-content-community-sha256-ctx.md)：
  上传内容哈希透传请求 ctx（AGENTS.md 上下文规则），提交 9a39976。
- [2026-08-14-content-community-httpx-error-contract.md](2026-08-14-content-community-httpx-error-contract.md)：
  网关公开 JSON 错误契约可测试化（MapError 提取 + 4 组单测），提交 5386689。
- [2026-08-14-content-community-errx-http-status.md](2026-08-14-content-community-errx-http-status.md)：
  业务错误 HTTP 状态映射补齐（密码错误 401、验证码 400、搜索 400/504），提交 4eef6ba。
- [2026-08-14-content-community-comment-idem-hash.md](2026-08-14-content-community-comment-idem-hash.md)：
  评论幂等命令哈希覆盖回复目标评论与被回复用户（CORE-050/051），提交见 frontmatter。
- [2026-08-14-content-community-media-idem-hash.md](2026-08-14-content-community-media-idem-hash.md)：
  媒体上传幂等命令哈希纳入文件内容指纹（CORE-050/051 异命令冲突），提交见 frontmatter。
- [2026-08-14-content-community-generate-hygiene.md](2026-08-14-content-community-generate-hygiene.md)：
  生成卫生：make generate 不再留下 OptionalAuth 死脚手架 + REL-A04 证据精确化，提交 305ae40。
- [2026-08-14-content-community-media-rowsaffected.md](2026-08-14-content-community-media-rowsaffected.md)：
  media SoftDelete RowsAffected 错误不再静默吞掉（全仓唯一吞错点），提交 2f29ee3。
- [2026-08-14-content-community-media-outbox.md](2026-08-14-content-community-media-outbox.md)：
  media 删除事件接入事务 outbox（软删与 media-deleted 同事务、relay 投递），提交 900ac7e。
- [2026-08-14-content-community-assistant-fact-support.md](2026-08-14-content-community-assistant-fact-support.md)：
  Assistant 事实陈述支持率门禁补齐（ASST-051，确定性 bigram 代理判定），提交 3436d90。
- [2026-08-14-content-community-frozen-evals.md](2026-08-14-content-community-frozen-evals.md)：
  LLM 生成冻结评测集（DISC-060/ASST-050，锚定合成语料），提交 ec77518。
- [2026-08-14-content-community-slo-synthetic.md](2026-08-14-content-community-slo-synthetic.md)：
  SLO 报告管线合成观测干跑验证（6 能力域 met=True），提交 ea0daa0。
- [2026-08-14-content-community-live-gates.md](2026-08-14-content-community-live-gates.md)：
  live 门禁执行（DISC-060 通过；ASST partial）与检索/分页缺陷修复，提交见 frontmatter。
- [2026-08-14-content-community-recommend-gate.md](2026-08-14-content-community-recommend-gate.md)：
  推荐门禁冻结样本集与执行（规则基线现状，相对提升 0），提交见 frontmatter。
- [2026-08-14-content-community-quality-review.md](2026-08-14-content-community-quality-review.md)：
  质量复查批次（分页归一化 DRY + 知识层登记刷新），提交 7af6c6a。
- [2026-08-14-content-community-doc-governance.md](2026-08-14-content-community-doc-governance.md)：
  文档治理批次（证据登记检查 + 生成同步验证），提交 07fc700。
- [2026-08-14-content-community-mqx-topics.md](2026-08-14-content-community-mqx-topics.md)：
  MQ 主题死代码清理与代码-部署一致性护栏，提交见 frontmatter。
- [2026-08-14-content-community-ci-python-gates.md](2026-08-14-content-community-ci-python-gates.md)：
  CI 补齐 Python 质量门禁（spec-evals-test / algorithm-test），提交见 frontmatter。
- [2026-08-14-content-community-shared-dead-code.md](2026-08-14-content-community-shared-dead-code.md)：
  共享库死代码清理（cachex 模块 + util/middleware/validator 死函数），提交见 frontmatter。
- [2026-08-14-content-community-reserved-models.md](2026-08-14-content-community-reserved-models.md)：
  interaction 预留 Model 清理（favorite_folder/report/view_history），提交见 frontmatter。
- [2026-08-14-content-community-spec-evals-tests.md](2026-08-14-content-community-spec-evals-tests.md)：
  spec_evals 测试加固（死 import + 报告函数直接单测），提交见 frontmatter。
- [2026-08-14-content-community-assistant-dead-const.md](2026-08-14-content-community-assistant-dead-const.md)：
  assistant 工具名死常量清理与全仓导出符号复查，提交见 frontmatter。
- [2026-08-14-content-community-feed-inbox-cursor.md](2026-08-14-content-community-feed-inbox-cursor.md)：
  关注流 inbox 残留分页缺陷修复（DISC-011），提交见 frontmatter。
- [2026-08-14-content-community-media-orphan.md](2026-08-14-content-community-media-orphan.md)：
  媒体上传幂等重试孤儿对象清理，提交见 frontmatter。
- [2026-08-14-content-community-message-dead-method.md](2026-08-14-content-community-message-dead-method.md)：
  message 会话死方法清理（UpsertPairForMessage），提交见 frontmatter。
- [2026-08-14-content-community-countsync-retry.md](2026-08-14-content-community-countsync-retry.md)：
  count-sync 去重占位失败重试缺陷修复（CORE-032），提交见 frontmatter。
- [2026-08-19-content-community-countsync-consumer-group.md](2026-08-19-content-community-countsync-consumer-group.md)：
  count-sync 独立消费组，避免与 cleanup 同组二次 Start fatal（CORE-032），提交见 frontmatter。
- [2026-08-14-content-community-post-idem-hash.md](2026-08-14-content-community-post-idem-hash.md)：
  帖子幂等命令哈希完整性修复（CORE-050/051），提交见 frontmatter。
- [2026-08-14-content-community-tags-limit.md](2026-08-14-content-community-tags-limit.md)：
  标签 limit 防御性上限 + vet copylocks 修复，提交见 frontmatter。
- [2026-08-14-content-community-verify-code.md](2026-08-14-content-community-verify-code.md)：
  验证码发送冷却与暴力尝试限制（安全加固），提交见 frontmatter。
- [2026-08-14-content-community-login-hardening.md](2026-08-14-content-community-login-hardening.md)：
  登录路径暴力尝试加固（共享验证码计数 + 密码锁定），提交见 frontmatter。
- [2026-08-14-content-community-doc-refresh.md](2026-08-14-content-community-doc-refresh.md)：
  知识层元数据刷新（8 个修复批次后的复查），提交见 frontmatter。
- [2026-08-14-content-community-verify-cooldown-fix.md](2026-08-14-content-community-verify-cooldown-fix.md)：
  验证码冷却语义修复（注册后立即登录回归），提交见 frontmatter。
- [2026-08-14-content-community-model-pipeline-ttl.md](2026-08-14-content-community-model-pipeline-ttl.md)：
  模型管线集成测试 TTL 冲突修复，提交见 frontmatter。
- [2026-08-14-content-community-env-example.md](2026-08-14-content-community-env-example.md)：
  production.env.example 变量补全与 DSN 可覆盖，提交 2d7165e。
- [2026-08-14-content-community-page-size-consistency.md](2026-08-14-content-community-page-size-consistency.md)：
  gateway 页大小响应一致性修复，提交见 frontmatter。
- [2026-08-14-content-community-ctx-log.md](2026-08-14-content-community-ctx-log.md)：
  业务路径全局日志修复（AGENTS.md ctx 规则），提交见 frontmatter。
- [2026-08-14-content-community-gen-make-targets.md](2026-08-14-content-community-gen-make-targets.md)：
  评测数据生成器统一 Makefile 入口（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-scripts-lib.md](2026-08-14-content-community-scripts-lib.md)：
  门禁脚本模块枚举去重（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-lint-testable.md](2026-08-14-content-community-lint-testable.md)：
  engineering-lint 检查函数可测试化（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-coverage-test.md](2026-08-14-content-community-coverage-test.md)：
  coverage_report 工具补测试（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-spec-evals-cli.md](2026-08-14-content-community-spec-evals-cli.md)：
  spec_evals CLI 子命令 dispatch 测试补齐（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-python-unit.md](2026-08-14-content-community-python-unit.md)：
  Python 工具单测统一入口与 CI 接入（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-precommit-hooks.md](2026-08-14-content-community-precommit-hooks.md)：
  pre-commit 钩子补齐（工具质量），提交见 frontmatter。
- [2026-08-14-content-community-idempotencyx.md](2026-08-14-content-community-idempotencyx.md)：
  幂等模型共享包提取（DRY 重构），提交见 frontmatter。
- [2026-08-14-content-community-rpc-error-dedup.md](2026-08-14-content-community-rpc-error-dedup.md)：
  gateway RPC 错误映射 helper 去重（DRY），提交见 frontmatter。
- [2026-08-14-content-community-asst031.md](2026-08-14-content-community-asst031.md)：
  ASST-031 历史来源清理实现（规格层验证），提交见 frontmatter。
- [2026-08-14-content-community-asst035.md](2026-08-14-content-community-asst035.md)：
  ASST-035 同请求标识去重扩展（规格层验证），提交见 frontmatter。
- [2026-08-14-content-community-asst032.md](2026-08-14-content-community-asst032.md)：
  ASST-032 LLM 降级返回证据摘要（规格层验证），提交见 frontmatter。
- [2026-08-14-content-community-asst051-threshold.md](2026-08-14-content-community-asst051-threshold.md)：
  ASST-051 来源有效率阈值修正（规格层验证），提交见 frontmatter。
- [2026-08-14-content-community-asst051-accuracy.md](2026-08-14-content-community-asst051-accuracy.md)：
  来源有效率计算语义修正（ASST-012/051），提交见 frontmatter。

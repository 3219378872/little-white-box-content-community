# 实现证据

本目录属于实现层，保存带日期的验证快照，不是独立规范层。

- 文件名使用 `YYYY-MM-DD-<slug>.md`。
- 每份证据引用一个现有 `IMP-*`，记录验证提交、实际命令、结果和未覆盖边界。
- 历史证据保留原始结果；后续失败通过新证据和实现状态表达，不回写旧记录。
- 新证据使用 `../../templates/evidence.md`。

## 当前证据记录

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

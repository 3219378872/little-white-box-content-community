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

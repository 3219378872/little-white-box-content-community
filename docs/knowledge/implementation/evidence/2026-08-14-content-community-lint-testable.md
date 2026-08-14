---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 745fde5
commands:
  - python3 -m unittest scripts.test_engineering_lint
  - python3 scripts/engineering-lint.py
  - make engineering-lint
result: passed
---

# 2026-08-14 engineering-lint 两个检查函数可测试化（工具质量）

## 缺陷

`check_doc_policy` 与 `check_md_file_links` 硬编码模块级 `ROOT`/
`DOCS_DIR`/`ACTIVE_DIRS`/`ACTIVE_FILES`，无 root 注入、无单元测试
（仅被 `main()` 调用，回归时只有运行时暴露）。其余四个检查函数
（knowledge_layers/proto_generation/spec_tracking/evidence）已有
测试。

## 改动

- `check_doc_policy(root=ROOT)`、`check_md_file_links(root=ROOT)`：
  接受 root 参数；legacy 目录与 ACTIVE 集合按 root 派生。
- `is_active_file(path, root, active_files, active_dirs)`：活动文件/
  目录集合可注入，便于临时 root 测试。
- 新增 7 项单测：DocPolicyLintTest（合法/路由/legacy 术语/legacy
  目录/CLAUDE 指针）+ MdFileLinkLintTest（链接解析/坏链接）。

## 结果

- `test_engineering_lint` 25 项全过（原 18 + 新 7）；实际
  `engineering-lint` 与 `make engineering-lint` 通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。

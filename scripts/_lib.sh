#!/usr/bin/env bash
# 共享脚本库：供 scripts/ 下的门禁脚本复用。
# 用法：ROOT_DIR 已定义后 source 本文件。

# list_modules 输出工作区内所有 Go module 目录（相对路径，排除 worktree/vendor）。
# 与 go.work 的 use 列表保持一致，避免各脚本重复维护。
list_modules() {
  find . -name go.mod -not -path './.worktree/*' -not -path './vendor/*' \
    -exec dirname {} \; | sort
}

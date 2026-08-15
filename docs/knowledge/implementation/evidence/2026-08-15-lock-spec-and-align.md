---
implementation: IMP-content-community-backend
verified_at: 2026-08-15
verified_commit: a52eb89
commands:
  - python3 scripts/engineering-lint.py
  - python3 -m unittest -q scripts.test_engineering_lint
  - go test -count=1 ./pkg/jwtx/ ./pkg/errx/ ./app/gateway/ ./app/gateway/internal/httpxconfig/ ./app/gateway/internal/handler/image/ ./app/gateway/internal/logic/posts/ ./app/content/rpc/internal/logic/ ./app/interaction/rpc/internal/logic/ ./app/user/rpc/internal/logic/
result: passed
---

# 2026-08-15 锁定意图/规范并对齐实现

人类授权整定 INT/SPEC 后，按锁定规范改设计与关键偏离实现。

## 规范锁定

- 意图补齐目标、成功标准、范围、非目标和约束。
- CORE-015/DISC-001 允许页内 `Total` 估计；CORE-032 要求详情回填互动状态；
  CORE-034 赞藏仅已发布；CORE-044 私信成功不依赖通知；CORE-054 禁止回传底层错误串；
  CORE-062 记录移除 v1 帖子写接口。
- DISC-060/ASST-050 明确人类双评审；LLM 合成集不能关闭门禁。
- REL-004 拆成客户端视口定义与服务端去重；REL-033 编号 SLO 表；REL-054 编号降级矩阵
  并去掉赞/评/关通知行。

## 实现

- GetPost 经 viewerstate 回填 isLiked/isFavorited。
- 点赞/收藏写前调 Content `AssertInteractable`（帖子 published；评论有效且父帖 published）。
- GetUserIdFromContext → LoginRequired；FromHTTPError 消毒；上传区分超大与非法 multipart。
- 登录/注册/验证码日志去掉手机号。

## 未覆盖

- REL-054 十行未全部注入测试。
- 人类冻结集与真实月度 SLO 仍待外部输入。

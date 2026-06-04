# Phase 1: ClickHouse 行为日志 + behavior-log-consumer 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 ClickHouse 行为事件日志基础设施并实现 behavior-log-consumer，将所有用户行为事件（点赞、收藏、评论、关注等）写入 ClickHouse behavior_events 表，为后续特征工程和离线训练提供数据基座。

**Architecture:** MQ consumer 订阅 7 个行为事件 Topic → go-zero Bloom Filter 前置去重 → 逐条写入 ClickHouse。ClickHouse 使用 ReplacingMergeTree(event_time) 以 event_id 为排序末列，后台 merge 自动去重作为最终兜底。两个物化视图分别优化用户维度聚合和时间范围批量扫描。

**Tech Stack:** go-zero v1.10.1, ClickHouse (clickhouse-go/v2 + database/sql), go-zero core/bloom, RocketMQ, testcontainers-go/modules/clickhouse

**设计规范来源:** `docs/superpowers/specs/2026-04-29-data-foundation-design.md` — Phase 1

**前置条件（需用户批准）：**
- 新增 Go 依赖: `github.com/ClickHouse/clickhouse-go/v2`, `github.com/testcontainers/testcontainers-go/modules/clickhouse`

---

## 文件结构

### 新建文件

```
deploy/
  clickhouse/
    behavior_events.sql                              # ClickHouse DDL: 主表 + 2 个物化视图

pkg/
  clickhousex/
    client.go                                        # ClickHouse database/sql 连接封装
    client_test.go                                   # 连接 + Ping 测试 (testcontainers)
  event/
    behavior.go                                      # BehaviorEvent 共享消息类型 + 验证
    behavior_test.go                                 # JSON 序列化 + 验证逻辑测试
  testutil/
    clickhouse.go                                    # ClickHouse testcontainer 辅助

app/pipeline/
  behaviorlog/
    main.go                                          # 入口: 创建 consumer, 订阅 7 个 Topic, 阻塞
    etc/
      behavior-log.yaml                              # 配置: MQ + ClickHouse + Redis + Bloom
    internal/
      config/
        config.go                                    # Config 结构体
      svc/
        service_context.go                           # DI 容器: ClickHouse conn + Bloom + Store
      consumer/
        behavior_log.go                              # 消费处理函数: 解析 → 去重 → 写入
        behavior_log_test.go                         # 单元测试: mock store + mock dedup
      store/
        clickhouse_store.go                          # BehaviorStore 接口 + ClickHouse 实现
        clickhouse_store_test.go                     # 集成测试: testcontainers ClickHouse
      dedup/
        bloom_dedup.go                               # BloomDedup: go-zero bloom 日期分桶去重
        bloom_dedup_test.go                          # 集成测试: testcontainers Redis
```

### 修改文件

```
deploy/docker-compose.middleware.yml                 # 新增 ClickHouse 服务
pkg/mqx/topics.go                                   # 新增 GroupBehaviorLogService
go.mod / go.sum                                      # 新增 clickhouse-go/v2 + testcontainers/clickhouse
```

---

## Task 1: 新增 ClickHouse 基础设施

**Files:**
- Modify: `deploy/docker-compose.middleware.yml`
- Create: `deploy/clickhouse/behavior_events.sql`

- [ ] **Step 1: 向 docker-compose.middleware.yml 添加 ClickHouse 服务**

在 `services:` 块末尾追加：

```yaml
  clickhouse:
    image: clickhouse/clickhouse-server:23.8-alpine
    container_name: xbh-clickhouse
    restart: unless-stopped
    ports:
      - "8123:8123"
      - "9000:9000"
    volumes:
      - clickhouse_data:/var/lib/clickhouse
      - ./clickhouse:/docker-entrypoint-initdb.d
    environment:
      CLICKHOUSE_DB: xbh_analytics
      CLICKHOUSE_USER: default
      CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: 1
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
```

在 `volumes:` 块追加：

```yaml
  clickhouse_data:
```

- [ ] **Step 2: 创建 ClickHouse DDL 文件**

创建 `deploy/clickhouse/behavior_events.sql`：

```sql
CREATE DATABASE IF NOT EXISTS xbh_analytics;

CREATE TABLE IF NOT EXISTS xbh_analytics.behavior_events (
    event_id    Int64,
    event_time  DateTime64(3),
    user_id     Int64,
    action      LowCardinality(String),
    target_id   Int64,
    target_type LowCardinality(String),
    duration    Int32 DEFAULT 0,
    scene       String DEFAULT '',
    client_ip   String DEFAULT ''
) ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (user_id, action, event_time, event_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.user_action_daily
ENGINE = SummingMergeTree()
ORDER BY (user_id, action, target_type, date)
AS SELECT
    toDate(event_time) AS date,
    user_id, action, target_type,
    count() AS cnt
FROM xbh_analytics.behavior_events
GROUP BY date, user_id, action, target_type;

CREATE MATERIALIZED VIEW IF NOT EXISTS xbh_analytics.behavior_events_by_time
ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (event_time, user_id, event_id)
AS SELECT * FROM xbh_analytics.behavior_events;
```

- [ ] **Step 3: 验证 ClickHouse 容器启动**

Run: `cd deploy && docker compose -f docker-compose.middleware.yml up -d clickhouse && sleep 5 && docker exec xbh-clickhouse clickhouse-client --query "SHOW TABLES FROM xbh_analytics"`

Expected: 输出包含 `behavior_events`、`user_action_daily`、`behavior_events_by_time`

- [ ] **Step 4: Commit**

```bash
git add deploy/docker-compose.middleware.yml deploy/clickhouse/behavior_events.sql
git commit -m "infra: add ClickHouse service and behavior_events DDL

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 2: 添加 ClickHouse Go 依赖

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: 安装 clickhouse-go 驱动和 testcontainers ClickHouse 模块**

Run: `go get github.com/ClickHouse/clickhouse-go/v2 && go get github.com/testcontainers/testcontainers-go/modules/clickhouse`

Expected: go.mod 新增两个 require 条目

- [ ] **Step 2: 验证依赖可用**

Run: `go mod tidy`

Expected: 无错误

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add clickhouse-go/v2 and testcontainers clickhouse module

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 3: pkg/testutil/clickhouse.go — ClickHouse TestContainer 辅助

**Files:**
- Create: `pkg/testutil/clickhouse.go`

- [ ] **Step 1: 创建 ClickHouse testcontainer 辅助函数**

```go
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	chmodule "github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

type ClickHouseEnv struct {
	DB      *sql.DB
	DSN     string
	closeFn func()
}

func SetupClickHouseEnv(t *testing.T, initScripts ...string) *ClickHouseEnv {
	t.Helper()
	env, err := setupClickHouseEnv(initScripts...)
	require.NoError(t, err)
	return env
}

func SetupClickHouseEnvM(initScripts ...string) *ClickHouseEnv {
	env, err := setupClickHouseEnv(initScripts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupClickHouseEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupClickHouseEnv(initScripts ...string) (*ClickHouseEnv, error) {
	ctx := context.Background()

	opts := []testcontainers.ContainerCustomizer{
		chmodule.WithDatabase("xbh_analytics"),
		chmodule.WithUsername("default"),
		chmodule.WithPassword(""),
	}
	for _, script := range initScripts {
		opts = append(opts, chmodule.WithInitScripts(script))
	}

	container, err := chmodule.Run(ctx, "clickhouse/clickhouse-server:23.8-alpine", opts...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("clickhouse dsn: %w", err)
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("sql.Open clickhouse: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(container)
	}

	return &ClickHouseEnv{DB: db, DSN: dsn, closeFn: cleanup}, nil
}

func (e *ClickHouseEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./pkg/testutil/...`

Expected: 无错误

- [ ] **Step 3: Commit**

```bash
git add pkg/testutil/clickhouse.go
git commit -m "test: add ClickHouse testcontainer helper in pkg/testutil

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 4: pkg/event/behavior.go — 共享行为事件类型 (TDD)

**Files:**
- Create: `pkg/event/behavior.go`
- Create: `pkg/event/behavior_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBehaviorEvent_JSONRoundTrip(t *testing.T) {
	e := BehaviorEvent{
		EventID:    100001,
		EventTime:  1714300000000,
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Duration:   0,
		Scene:      "home",
		ClientIP:   "10.0.0.1",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var got BehaviorEvent
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestBehaviorEvent_Validate_Valid(t *testing.T) {
	e := BehaviorEvent{
		EventID:    1,
		UserID:     42,
		Action:     "like",
		TargetID:   100,
		TargetType: "post",
	}
	assert.NoError(t, e.Validate())
}

func TestBehaviorEvent_Validate_MissingUserID(t *testing.T) {
	e := BehaviorEvent{EventID: 1, Action: "like", TargetID: 100, TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "user_id")
}

func TestBehaviorEvent_Validate_MissingAction(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, TargetID: 100, TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "action")
}

func TestBehaviorEvent_Validate_MissingTargetID(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, Action: "like", TargetType: "post"}
	assert.ErrorContains(t, e.Validate(), "target_id")
}

func TestBehaviorEvent_Validate_MissingTargetType(t *testing.T) {
	e := BehaviorEvent{EventID: 1, UserID: 42, Action: "like", TargetID: 100}
	assert.ErrorContains(t, e.Validate(), "target_type")
}

func TestBehaviorEvent_EventIDString(t *testing.T) {
	e := BehaviorEvent{EventID: 123456789}
	assert.Equal(t, "123456789", e.EventIDString())
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./pkg/event/... -v -count=1`

Expected: FAIL — `package esx/pkg/event` 不存在

- [ ] **Step 3: 编写最小实现**

```go
package event

import (
	"fmt"
	"strconv"
)

type BehaviorEvent struct {
	EventID    int64  `json:"event_id"`
	EventTime  int64  `json:"event_time"`
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
	Duration   int32  `json:"duration"`
	Scene      string `json:"scene"`
	ClientIP   string `json:"client_ip"`
}

func (e *BehaviorEvent) Validate() error {
	if e.UserID <= 0 {
		return fmt.Errorf("user_id is required")
	}
	if e.Action == "" {
		return fmt.Errorf("action is required")
	}
	if e.TargetID <= 0 {
		return fmt.Errorf("target_id is required")
	}
	if e.TargetType == "" {
		return fmt.Errorf("target_type is required")
	}
	return nil
}

func (e *BehaviorEvent) EventIDString() string {
	return strconv.FormatInt(e.EventID, 10)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./pkg/event/... -v -count=1`

Expected: PASS — all 6 tests pass

- [ ] **Step 5: Commit**

```bash
git add pkg/event/behavior.go pkg/event/behavior_test.go
git commit -m "feat(event): add shared BehaviorEvent type with validation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 5: Bloom Filter 去重 (TDD)

**Files:**
- Create: `app/pipeline/behaviorlog/internal/dedup/bloom_dedup.go`
- Create: `app/pipeline/behaviorlog/internal/dedup/bloom_dedup_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package dedup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func setupRedis(t *testing.T) *redis.Redis {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}
	container, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)

	return redis.MustNewRedis(redis.RedisConf{
		Host: fmt.Sprintf("%s:%s", host, port.Port()),
		Type: redis.NodeType,
	})
}

func TestBloomDedup_NewEvent_NotDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	dup, err := d.IsDuplicate(context.Background(), "event-001")
	require.NoError(t, err)
	assert.False(t, dup)
}

func TestBloomDedup_SameEvent_IsDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	dup1, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.False(t, dup1)

	dup2, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.True(t, dup2)
}

func TestBloomDedup_DifferentEvents_NotDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	_, _ = d.IsDuplicate(context.Background(), "event-aaa")
	dup, err := d.IsDuplicate(context.Background(), "event-bbb")
	require.NoError(t, err)
	assert.False(t, dup)
}

func TestBloomDedup_KeyContainsDate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	key := d.keyForDate(time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "bf:behavior_events:20260429", key)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/pipeline/behaviorlog/internal/dedup/... -v -count=1 -timeout=120s`

Expected: FAIL — package 不存在

- [ ] **Step 3: 编写实现**

```go
package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/bloom"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	keyPrefix = "bf:behavior_events:"
	ttlHours  = 48
)

type BloomDedup struct {
	rds  *redis.Redis
	bits uint
}

func NewBloomDedup(rds *redis.Redis, bits uint) *BloomDedup {
	return &BloomDedup{rds: rds, bits: bits}
}

func (d *BloomDedup) keyForDate(t time.Time) string {
	return fmt.Sprintf("%s%s", keyPrefix, t.Format("20060102"))
}

func (d *BloomDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	now := time.Now()
	data := []byte(eventID)

	todayKey := d.keyForDate(now)
	todayFilter := bloom.New(d.rds, todayKey, d.bits)

	exists, err := todayFilter.Exists(data)
	if err != nil {
		return false, fmt.Errorf("bloom exists today: %w", err)
	}
	if exists {
		return true, nil
	}

	yesterday := now.AddDate(0, 0, -1)
	yesterdayKey := d.keyForDate(yesterday)
	yesterdayFilter := bloom.New(d.rds, yesterdayKey, d.bits)

	exists, err = yesterdayFilter.Exists(data)
	if err != nil {
		return false, fmt.Errorf("bloom exists yesterday: %w", err)
	}
	if exists {
		return true, nil
	}

	if err := todayFilter.Add(data); err != nil {
		return false, fmt.Errorf("bloom add: %w", err)
	}

	_, _ = d.rds.ExpireCtx(ctx, todayKey, int(ttlHours*3600))

	return false, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/pipeline/behaviorlog/internal/dedup/... -v -count=1 -timeout=120s`

Expected: PASS — all 4 tests pass

- [ ] **Step 5: Commit**

```bash
git add app/pipeline/behaviorlog/internal/dedup/
git commit -m "feat(pipeline): add Bloom Filter dedup with daily key rotation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 6: ClickHouse Store — 写入行为事件 (TDD)

**Files:**
- Create: `app/pipeline/behaviorlog/internal/store/clickhouse_store.go`
- Create: `app/pipeline/behaviorlog/internal/store/clickhouse_store_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package store

import (
	"context"
	"testing"
	"time"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCH(t *testing.T) *testutil.ClickHouseEnv {
	t.Helper()
	return testutil.SetupClickHouseEnv(t, testutil.ClickHouseSchemaPath("behavior_events.sql"))
}

func TestClickHouseStore_Insert_SingleEvent(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{
		EventID:    10001,
		EventTime:  time.Now().UnixMilli(),
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
		Duration:   0,
		Scene:      "home",
		ClientIP:   "10.0.0.1",
	}

	err := s.Insert(context.Background(), e)
	require.NoError(t, err)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 10001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestClickHouseStore_Insert_DuplicateEventID_Deduped(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{
		EventID:    20001,
		EventTime:  time.Now().UnixMilli(),
		UserID:     42,
		Action:     "like",
		TargetID:   999,
		TargetType: "post",
	}

	require.NoError(t, s.Insert(context.Background(), e))
	require.NoError(t, s.Insert(context.Background(), e))

	// OPTIMIZE 强制 merge 以触发 ReplacingMergeTree 去重
	_, err := chEnv.DB.ExecContext(context.Background(),
		"OPTIMIZE TABLE xbh_analytics.behavior_events FINAL")
	require.NoError(t, err)

	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events FINAL WHERE event_id = 20001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestClickHouseStore_Insert_InvalidEvent_ReturnsError(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	e := event.BehaviorEvent{}

	err := s.Insert(context.Background(), e)
	assert.Error(t, err)
}

func TestClickHouseStore_QueryByUser(t *testing.T) {
	chEnv := setupCH(t)
	defer chEnv.Close()

	s := NewClickHouseStore(chEnv.DB)
	now := time.Now().UnixMilli()

	events := []event.BehaviorEvent{
		{EventID: 30001, EventTime: now, UserID: 100, Action: "like", TargetID: 1, TargetType: "post"},
		{EventID: 30002, EventTime: now, UserID: 100, Action: "favorite", TargetID: 2, TargetType: "post"},
		{EventID: 30003, EventTime: now, UserID: 200, Action: "like", TargetID: 3, TargetType: "post"},
	}
	for _, e := range events {
		require.NoError(t, s.Insert(context.Background(), e))
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 100").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/pipeline/behaviorlog/internal/store/... -v -count=1 -timeout=180s`

Expected: FAIL — `store` package 不存在

- [ ] **Step 3: 在 pkg/testutil/clickhouse.go 中添加 SchemaPath 辅助函数**

在 `pkg/testutil/clickhouse.go` 文件末尾追加：

```go
func ClickHouseSchemaPath(filename string) string {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..")
	return filepath.Join(root, "deploy", "clickhouse", filename)
}
```

同时在 import 中添加 `"path/filepath"` 和 `"runtime"`（如尚未引入）。

- [ ] **Step 4: 编写 ClickHouseStore 实现**

```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"esx/pkg/event"
)

type BehaviorStore interface {
	Insert(ctx context.Context, e event.BehaviorEvent) error
}

type ClickHouseStore struct {
	db *sql.DB
}

func NewClickHouseStore(db *sql.DB) *ClickHouseStore {
	return &ClickHouseStore{db: db}
}

func (s *ClickHouseStore) Insert(ctx context.Context, e event.BehaviorEvent) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("validate behavior event: %w", err)
	}

	eventTime := time.UnixMilli(e.EventTime)
	if e.EventTime == 0 {
		eventTime = time.Now()
	}

	query := `INSERT INTO xbh_analytics.behavior_events
		(event_id, event_time, user_id, action, target_id, target_type, duration, scene, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		e.EventID, eventTime, e.UserID, e.Action,
		e.TargetID, e.TargetType, e.Duration, e.Scene, e.ClientIP)
	if err != nil {
		return fmt.Errorf("insert behavior_events: %w", err)
	}

	return nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./app/pipeline/behaviorlog/internal/store/... -v -count=1 -timeout=180s`

Expected: PASS — all 4 tests pass

- [ ] **Step 6: Commit**

```bash
git add app/pipeline/behaviorlog/internal/store/ pkg/testutil/clickhouse.go
git commit -m "feat(pipeline): add ClickHouseStore for behavior event persistence

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 7: 消费处理函数 (TDD)

**Files:**
- Create: `app/pipeline/behaviorlog/internal/consumer/behavior_log.go`
- Create: `app/pipeline/behaviorlog/internal/consumer/behavior_log_test.go`
- Modify: `pkg/mqx/topics.go`

- [ ] **Step 1: 在 pkg/mqx/topics.go 添加新消费者组**

在 `ConsumerGroup` 常量块末尾追加：

```go
GroupBehaviorLogService = "behavior-log-service-group"
```

- [ ] **Step 2: 编写失败测试**

```go
package consumer

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type mockStore struct {
	inserted []event.BehaviorEvent
	err      error
}

func (m *mockStore) Insert(ctx context.Context, e event.BehaviorEvent) error {
	if m.err != nil {
		return m.err
	}
	m.inserted = append(m.inserted, e)
	return nil
}

type mockDedup struct {
	seen map[string]bool
	err  error
}

func newMockDedup() *mockDedup {
	return &mockDedup{seen: make(map[string]bool)}
}

func (m *mockDedup) IsDuplicate(_ context.Context, eventID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.seen[eventID] {
		return true, nil
	}
	m.seen[eventID] = true
	return false, nil
}

func makeMsg(body string) *primitive.MessageExt {
	return &primitive.MessageExt{
		Message: primitive.Message{Body: []byte(body)},
		MsgId:   "test-msg",
	}
}

func TestConsumeBehavior_ValidEvent_Inserts(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
	assert.Equal(t, int64(42), store.inserted[0].UserID)
	assert.Equal(t, "like", store.inserted[0].Action)
}

func TestConsumeBehavior_MalformedJSON_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`bad-json`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.inserted)
}

func TestConsumeBehavior_ValidationFails_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"user_id":0,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.inserted)
}

func TestConsumeBehavior_DuplicateEvent_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	msg := makeMsg(`{"event_id":100,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`)

	result1 := consumeBehaviorMsg(context.Background(), store, dedup, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result1)
	assert.Len(t, store.inserted, 1)

	result2 := consumeBehaviorMsg(context.Background(), store, dedup, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result2)
	assert.Len(t, store.inserted, 1) // 仍为 1，重复被过滤
}

func TestConsumeBehavior_StoreError_ReturnsRetry(t *testing.T) {
	store := &mockStore{err: errors.New("clickhouse down")}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestConsumeBehavior_DedupError_FallsThrough(t *testing.T) {
	store := &mockStore{}
	dedup := &mockDedup{seen: make(map[string]bool), err: errors.New("redis down")}

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	// Bloom Filter 故障时放行，由 ClickHouse ReplacingMergeTree 兜底去重
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
}

func TestConsumeBehavior_ZeroEventID_GeneratesOne(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
	assert.NotZero(t, store.inserted[0].EventID)
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./app/pipeline/behaviorlog/internal/consumer/... -v -count=1`

Expected: FAIL — `consumeBehaviorMsg` undefined

- [ ] **Step 4: 编写消费处理函数实现**

```go
package consumer

import (
	"context"
	"encoding/json"
	"time"

	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"esx/pkg/util"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type Deduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
}

func consumeBehaviorMsg(ctx context.Context, s store.BehaviorStore, d Deduper, msg *primitive.MessageExt) consumer.ConsumeResult {
	var e event.BehaviorEvent
	if err := json.Unmarshal(msg.Body, &e); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: unmarshal failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}

	if e.EventID == 0 {
		id, err := util.NextID()
		if err != nil {
			logx.WithContext(ctx).Errorw("behavior-log: generate event_id failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		e.EventID = id
	}

	if e.EventTime == 0 {
		e.EventTime = time.Now().UnixMilli()
	}

	if err := e.Validate(); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: validation failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}

	dup, err := d.IsDuplicate(ctx, e.EventIDString())
	if err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: dedup check failed, falling through",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
	} else if dup {
		logx.WithContext(ctx).Infow("behavior-log: duplicate event skipped",
			logx.Field("event_id", e.EventID))
		return consumer.ConsumeSuccess
	}

	if err := s.Insert(ctx, e); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: insert failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("event_id", e.EventID),
			logx.Field("err", err.Error()))
		return consumer.ConsumeRetryLater
	}

	logx.WithContext(ctx).Infow("behavior-log: event recorded",
		logx.Field("event_id", e.EventID), logx.Field("user_id", e.UserID),
		logx.Field("action", e.Action))

	return consumer.ConsumeSuccess
}

func MakeBehaviorHandler(s store.BehaviorStore, d Deduper) func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	return func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			result := consumeBehaviorMsg(ctx, s, d, msg)
			if result == consumer.ConsumeRetryLater {
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./app/pipeline/behaviorlog/internal/consumer/... -v -count=1`

Expected: PASS — all 7 tests pass

注意: `TestConsumeBehavior_ZeroEventID_GeneratesOne` 需要 Snowflake 初始化。如果测试中 `util.NextID()` 返回错误（因未初始化），在测试文件的 `TestMain` 或 `init()` 中调用 `util.InitSnowflake(1, 1)`。如果需要，在测试文件顶部添加：

```go
func init() {
	_ = util.InitSnowflake(1, 1)
}
```

- [ ] **Step 6: Commit**

```bash
git add app/pipeline/behaviorlog/internal/consumer/ pkg/mqx/topics.go
git commit -m "feat(pipeline): implement behavior-log consumer handler with dedup

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 8: Config + ServiceContext + 入口文件

**Files:**
- Create: `app/pipeline/behaviorlog/internal/config/config.go`
- Create: `app/pipeline/behaviorlog/internal/svc/service_context.go`
- Create: `app/pipeline/behaviorlog/etc/behavior-log.yaml`
- Create: `app/pipeline/behaviorlog/main.go`

- [ ] **Step 1: 创建 Config 结构体**

```go
package config

import (
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	MQ             mqx.ConsumerConfig
	ClickHouseDSN  string
	Redis          redis.RedisConf
	BloomBits      uint   `json:",default=20971520"` // 20M bits, ~1M events at 1% FP
	WorkerID       int64  `json:",default=1"`
	DatacenterID   int64  `json:",default=1"`
}
```

- [ ] **Step 2: 创建 ServiceContext**

```go
package svc

import (
	"database/sql"
	"fmt"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"esx/app/pipeline/behaviorlog/internal/config"
	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config config.Config
	Store  store.BehaviorStore
	Dedup  *dedup.BloomDedup
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := sql.Open("clickhouse", c.ClickHouseDSN)
	if err != nil {
		panic(fmt.Sprintf("behavior-log: open clickhouse: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("behavior-log: ping clickhouse: %v", err))
	}

	rds := redis.MustNewRedis(c.Redis)

	return &ServiceContext{
		Config: c,
		Store:  store.NewClickHouseStore(db),
		Dedup:  dedup.NewBloomDedup(rds, c.BloomBits),
	}
}
```

- [ ] **Step 3: 创建配置文件**

创建 `app/pipeline/behaviorlog/etc/behavior-log.yaml`：

```yaml
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "behavior-log-service-group"
  Topic: ""
  Tag: "default"
  ConsumeOrder: false

ClickHouseDSN: "clickhouse://${CH_HOST:=127.0.0.1}:${CH_PORT:=9000}/xbh_analytics?dial_timeout=5s&compress=lz4"

Redis:
  Host: "${REDIS_HOST:=127.0.0.1:6379}"
  Type: node

BloomBits: 20971520
WorkerID: 1
DatacenterID: 1
```

- [ ] **Step 4: 创建 main.go 入口文件**

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"esx/app/pipeline/behaviorlog/internal/config"
	"esx/app/pipeline/behaviorlog/internal/consumer"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"esx/pkg/cleanupx"
	"esx/pkg/mqx"
	"esx/pkg/util"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/behavior-log.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if err := util.InitSnowflake(c.WorkerID, c.DatacenterID); err != nil {
		logx.Must(err)
	}

	svcCtx := svc.NewServiceContext(c)
	handler := consumer.MakeBehaviorHandler(svcCtx.Store, svcCtx.Dedup)

	mq, err := mqx.NewConsumer(c.MQ)
	if err != nil {
		logx.Must(err)
	}

	topics := []string{
		mqx.TopicLike, mqx.TopicUnlike,
		mqx.TopicFavorite, mqx.TopicUnfavorite,
		mqx.TopicCommentCreate,
		mqx.TopicUserFollow, mqx.TopicUserUnfollow,
	}
	for _, topic := range topics {
		if err := mq.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			logx.Must(fmt.Errorf("subscribe %s: %w", topic, err))
		}
	}

	if err := mq.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "behavior-log consumer", mq.Shutdown)

	fmt.Println("Behavior-log consumer started, subscribing: like/unlike/favorite/unfavorite/comment-create/user-follow/user-unfollow")
	select {}
}
```

- [ ] **Step 5: 验证编译通过**

Run: `go build ./app/pipeline/behaviorlog/...`

Expected: 编译成功，无错误

- [ ] **Step 6: Commit**

```bash
git add app/pipeline/behaviorlog/
git commit -m "feat(pipeline): add behavior-log consumer entry point and config

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 9: pkg/clickhousex — 可选通用封装 (TDD)

**Files:**
- Create: `pkg/clickhousex/client.go`
- Create: `pkg/clickhousex/client_test.go`

此任务提供一个薄封装层，供其他 Phase 的消费者复用 ClickHouse 连接管理。如果仅 behavior-log-consumer 使用，可跳过此任务。

- [ ] **Step 1: 编写失败测试**

```go
package clickhousex

import (
	"context"
	"testing"

	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_PingSucceeds(t *testing.T) {
	chEnv := testutil.SetupClickHouseEnv(t)
	defer chEnv.Close()

	client, err := NewClient(chEnv.DSN)
	require.NoError(t, err)
	defer client.Close()

	assert.NoError(t, client.Ping(context.Background()))
}

func TestNewClient_InvalidDSN_ReturnsError(t *testing.T) {
	_, err := NewClient("clickhouse://invalid:9999/nonexistent?dial_timeout=1s")
	assert.Error(t, err)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./pkg/clickhousex/... -v -count=1 -timeout=120s`

Expected: FAIL — package 不存在

- [ ] **Step 3: 编写实现**

```go
package clickhousex

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

type Client struct {
	db *sql.DB
}

func NewClient(dsn string) (*Client, error) {
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhousex open: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("clickhousex ping: %w", err)
	}

	return &Client{db: db}, nil
}

func (c *Client) DB() *sql.DB {
	return c.db
}

func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *Client) Close() error {
	return c.db.Close()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./pkg/clickhousex/... -v -count=1 -timeout=120s`

Expected: PASS — both tests pass

- [ ] **Step 5: Commit**

```bash
git add pkg/clickhousex/
git commit -m "feat(clickhousex): add ClickHouse client wrapper for shared use

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 10: 端到端集成测试

**Files:**
- Create: `app/pipeline/behaviorlog/internal/consumer/integration_test.go`

- [ ] **Step 1: 编写集成测试**

```go
//go:build integration

package consumer

import (
	"context"
	"os"
	"testing"
	"time"

	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"esx/pkg/testutil"
	"esx/pkg/util"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	chEnv    *testutil.ClickHouseEnv
	testEnv  *testutil.TestEnv
)

func TestMain(m *testing.M) {
	_ = util.InitSnowflake(1, 1)

	chEnv = testutil.SetupClickHouseEnvM(testutil.ClickHouseSchemaPath("behavior_events.sql"))
	defer chEnv.Close()

	// Redis for bloom filter — reuse TestEnv's Redis setup logic
	testEnv = testutil.SetupTestEnvM("xbh_test_behaviorlog", testutil.SchemaPath("xbh_user.sql"))
	defer testEnv.Close()

	os.Exit(m.Run())
}

func TestIntegration_FullPipeline_EventPersistedInClickHouse(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

	msg := &primitive.MessageExt{
		Message: primitive.Message{
			Body: []byte(`{
				"event_id": 99001,
				"event_time": 1714300000000,
				"user_id": 42,
				"action": "like",
				"target_id": 999,
				"target_type": "post",
				"scene": "home"
			}`),
		},
		MsgId: "integration-msg-1",
	}

	result, err := handler(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result)

	// 查询 ClickHouse 确认写入
	var count uint64
	err = chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99001").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_DuplicateEvent_FilteredByBloom(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

	body := []byte(`{
		"event_id": 99002,
		"event_time": 1714300000000,
		"user_id": 42,
		"action": "favorite",
		"target_id": 888,
		"target_type": "post"
	}`)

	msg1 := &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "dup-1"}
	msg2 := &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "dup-2"}

	result1, _ := handler(context.Background(), msg1)
	assert.Equal(t, consumer.ConsumeSuccess, result1)

	result2, _ := handler(context.Background(), msg2)
	assert.Equal(t, consumer.ConsumeSuccess, result2)

	// Bloom Filter 应阻止第二次写入
	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE event_id = 99002").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestIntegration_MultipleActions_AllPersisted(t *testing.T) {
	s := store.NewClickHouseStore(chEnv.DB)
	d := dedup.NewBloomDedup(testEnv.Redis, 1024)
	handler := MakeBehaviorHandler(s, d)

	now := time.Now().UnixMilli()
	actions := []struct {
		eventID  int64
		action   string
		targetID int64
	}{
		{99010, "like", 100},
		{99011, "favorite", 101},
		{99012, "comment", 102},
		{99013, "follow", 200},
	}

	for _, a := range actions {
		e := event.BehaviorEvent{
			EventID: a.eventID, EventTime: now,
			UserID: 50, Action: a.action,
			TargetID: a.targetID, TargetType: "post",
		}
		body, _ := json.Marshal(e)
		msg := &primitive.MessageExt{
			Message: primitive.Message{Body: body},
			MsgId:   fmt.Sprintf("multi-%d", a.eventID),
		}
		result, _ := handler(context.Background(), msg)
		assert.Equal(t, consumer.ConsumeSuccess, result)
	}

	var count uint64
	err := chEnv.DB.QueryRowContext(context.Background(),
		"SELECT count() FROM xbh_analytics.behavior_events WHERE user_id = 50 AND event_id >= 99010 AND event_id <= 99013").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), count)
}
```

注意：集成测试文件需要追加 import：

```go
import (
	"encoding/json"
	"fmt"
	// ... 其他 imports
)
```

- [ ] **Step 2: 运行集成测试**

Run: `go test ./app/pipeline/behaviorlog/internal/consumer/... -v -count=1 -timeout=300s -tags=integration -run TestIntegration`

Expected: PASS — all 3 integration tests pass

- [ ] **Step 3: 运行全量测试验证无回归**

Run: `go test ./app/pipeline/... -v -count=1 -timeout=300s`

Expected: PASS — 全部单元测试通过

Run: `go vet ./app/pipeline/... && go vet ./pkg/event/... && go vet ./pkg/clickhousex/...`

Expected: 无 vet 问题

- [ ] **Step 4: Commit**

```bash
git add app/pipeline/behaviorlog/internal/consumer/integration_test.go
git commit -m "test(pipeline): add end-to-end integration tests for behavior-log consumer

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## 后续阶段概览

本计划仅覆盖 Phase 1。以下阶段各自需要独立的详细实施计划：

### Phase 2: 内容管线消费者 (预计 1.5 周开发 + 0.5 周联调)
- **search-index-consumer**: 替换 `app/search/mq` 中的 `NoopIndexer` 为真实 ES 客户端 (upsert/delete)
- **embedding-consumer**: 新建消费者，调用 Embedding gRPC 服务写入 Milvus
- **content-cleanup-consumer**: 帖子删除时清理 Redis 特征缓存 + Feed 数据

### Phase 3: 内容特征消费者 (预计 1 周开发 + 0.5 周联调)
- **content-feature-consumer**: 规则引擎质量分计算 (QualityScorer 接口)
- **content-stat-consumer**: 攒批 Redis 互动计数 + 热度分 + 热榜 ZSET
- **content-stat-mysql-consumer**: 独立慢路径 MySQL 计数同步
- **热榜清理 cron**: `ZREMRANGEBYSCORE` + 成员数上限截断

### Phase 4: 用户特征 + 画像宽表 (预计 1 周开发 + 0.5 周联调)
- **user-feature-consumer**: 近期行为 LIST + 会话标签 SET + 30min ClickHouse 聚合刷新
- **user_profile_wide 表**: MySQL DDL + goctl model 生成
- **feature_version_registry 表**: 特征版本管理

### Phase 5: 漏斗管线 (预计 2 周开发 + 1 周联调)
- 多路召回 (ES + Milvus + Redis 并行 goroutine)
- 粗排 (质量过滤 + 时效衰减)
- 精排 (规则模型 → CTRModel 接口)
- 混排 (MMR 多样性 + 作者打散 + 已读去重)
- Search/Recommend RPC 对接
- 降级/熔断策略 (go-zero breaker + 本地缓存兜底)

### Phase 6: 全链路压测 + 灰度发布 (预计 1.5 周)
- 压测脚本编写
- 灰度发布方案
- 监控告警 (Prometheus + Grafana dashboard)

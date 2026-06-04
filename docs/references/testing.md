# Testing Patterns

## Table of Contents
- [Architecture Overview](#architecture-overview)
- [1. Unit Testing](#1-unit-testing)
- [2. Integration Testing](#2-integration-testing)
- [3. Contract Testing](#3-contract-testing)
- [4. Load Testing](#4-load-testing)
- [5. Go Benchmark](#5-go-benchmark)
- [6. pprof Profiling](#6-pprof-profiling)
- [7. Chaos Engineering](#7-chaos-engineering)
- [8. Test Organization & CI](#8-test-organization--ci)
- [Best Practices Summary](#best-practices-summary)

Comprehensive testing strategies for go-zero microservices: unit tests, integration tests, contract tests, performance testing, and profiling.

## Architecture Overview

```
Testing Pyramid:
┌─────────────────────┐
│    E2E Tests        │  ← Few, slow, high confidence
├─────────────────────┤
│  Integration Tests  │  ← Database, RPC, Redis
├─────────────────────┤
│    Unit Tests       │  ← Many, fast, isolated
└─────────────────────┘

Coverage Target: 80%+
```

---

## 1. Unit Testing

### ✅ Testing Logic Layer (Three-Layer Pattern)

Logic is the primary unit under test. Mock the ServiceContext dependencies:

```go
// internal/logic/get_user_logic_test.go
package logic

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "yourapp/internal/svc"
    "yourapp/internal/types"
)

// MockUserModel implements model.UserModel interface
type MockUserModel struct {
    mock.Mock
}

func (m *MockUserModel) FindOne(ctx context.Context, id int64) (*model.User, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*model.User), args.Error(1)
}

func TestGetUserLogic_Success(t *testing.T) {
    mockUser := &model.User{
        Id:       1001,
        Username: "testuser",
        Email:    "test@example.com",
    }

    mockModel := new(MockUserModel)
    mockModel.On("FindOne", mock.Anything, int64(1001)).Return(mockUser, nil)

    svcCtx := &svc.ServiceContext{
        UserModel: mockModel,
    }

    logic := NewGetUserLogic(context.Background(), svcCtx)
    resp, err := logic.GetUser(&types.GetUserRequest{Id: 1001})

    assert.NoError(t, err)
    assert.Equal(t, "testuser", resp.Username)
    mockModel.AssertExpectations(t)
}

func TestGetUserLogic_NotFound(t *testing.T) {
    mockModel := new(MockUserModel)
    mockModel.On("FindOne", mock.Anything, int64(9999)).Return(nil, model.ErrNotFound)

    svcCtx := &svc.ServiceContext{
        UserModel: mockModel,
    }

    logic := NewGetUserLogic(context.Background(), svcCtx)
    resp, err := logic.GetUser(&types.GetUserRequest{Id: 9999})

    assert.Error(t, err)
    assert.Nil(t, resp)
}
```

### ✅ Table-Driven Tests

```go
func TestCalculateDiscount(t *testing.T) {
    tests := []struct {
        name     string
        amount   float64
        vipLevel int
        want     float64
        wantErr  bool
    }{
        {"normal user no discount", 100, 0, 100, false},
        {"vip1 5% discount", 100, 1, 95, false},
        {"vip2 10% discount", 100, 2, 90, false},
        {"negative amount", -1, 0, 0, true},
        {"zero amount", 0, 0, 0, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateDiscount(tt.amount, tt.vipLevel)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.InDelta(t, tt.want, got, 0.01)
        })
    }
}
```

### ✅ Testing RPC Client Calls

```go
// Mock the RPC client
type MockUserRpc struct {
    mock.Mock
}

func (m *MockUserRpc) GetUser(ctx context.Context, in *userclient.GetUserRequest) (*userclient.GetUserResponse, error) {
    args := m.Called(ctx, in)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*userclient.GetUserResponse), args.Error(1)
}

func TestCreateOrderLogic_UserNotFound(t *testing.T) {
    mockUserRpc := new(MockUserRpc)
    mockUserRpc.On("GetUser", mock.Anything, mock.Anything).
        Return(nil, status.Error(codes.NotFound, "user not found"))

    svcCtx := &svc.ServiceContext{
        UserRpc: mockUserRpc,
    }

    logic := NewCreateOrderLogic(context.Background(), svcCtx)
    _, err := logic.CreateOrder(&types.CreateOrderRequest{UserId: 9999})

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "user not found")
}
```

---

## 2. Integration Testing

### ✅ Database Integration Test with TestMain

```go
// internal/model/user_model_test.go
package model

import (
    "context"
    "os"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testDB sqlx.SqlConn

func TestMain(m *testing.M) {
    // Use test database — set via environment variable
    dsn := os.Getenv("TEST_DATABASE_DSN")
    if dsn == "" {
        dsn = "root:password@tcp(localhost:3306)/testdb?parseTime=true"
    }

    testDB = sqlx.NewMysql(dsn)

    // Setup: create tables
    setupTestDB(testDB)

    code := m.Run()

    // Teardown: clean up
    teardownTestDB(testDB)
    os.Exit(code)
}

func setupTestDB(db sqlx.SqlConn) {
    db.Exec(`CREATE TABLE IF NOT EXISTS users (
        id BIGINT PRIMARY KEY,
        username VARCHAR(64) NOT NULL,
        email VARCHAR(128) NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )`)
}

func teardownTestDB(db sqlx.SqlConn) {
    db.Exec(`DROP TABLE IF EXISTS users`)
}

func TestUserModel_Insert_FindOne(t *testing.T) {
    model := NewUserModel(testDB)
    ctx := context.Background()

    // Insert
    user := &User{
        Id:       10001,
        Username: "integration_test_user",
        Email:    "integration@test.com",
    }
    _, err := model.Insert(ctx, user)
    assert.NoError(t, err)

    // FindOne
    found, err := model.FindOne(ctx, 10001)
    assert.NoError(t, err)
    assert.Equal(t, "integration_test_user", found.Username)

    // Cleanup
    _ = model.Delete(ctx, 10001)
}
```

### ✅ Redis Integration Test

```go
func TestCacheAside_Pattern(t *testing.T) {
    rds := redis.MustNewRedis(redis.RedisConf{
        Host: os.Getenv("TEST_REDIS_HOST"),
        Type: "node",
    })

    ctx := context.Background()
    key := "test:user:1001"

    // Clean up
    defer rds.DelCtx(ctx, key)

    // First call: cache miss → should query DB
    val, err := rds.GetCtx(ctx, key)
    assert.NoError(t, err)
    assert.Empty(t, val) // cache miss

    // Simulate setting cache after DB query
    err = rds.SetexCtx(ctx, key, `{"id":1001,"name":"test"}`, 60)
    assert.NoError(t, err)

    // Second call: cache hit
    val, err = rds.GetCtx(ctx, key)
    assert.NoError(t, err)
    assert.Contains(t, val, "test")
}
```

### ✅ HTTP Handler Integration Test

```go
import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestCreateUserHandler_Integration(t *testing.T) {
    // Build the full server with test config
    svcCtx := svc.NewServiceContext(testConfig)

    handler := handler.CreateUserHandler(svcCtx)

    body := `{"username":"newuser","email":"new@test.com","password":"SecurePass123!"}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), "newuser")
}
```

---

## 3. Contract Testing

### ✅ Consumer-Driven Contract Test (gRPC)

Verify that the consumer's expectations match the provider's actual API:

```go
// consumer_contract_test.go — runs in the consumer service
package contract

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    userpb "yourapp/pb/user"
)

// TestUserServiceContract validates that the user-rpc service
// still satisfies the contract expected by order-api (consumer).
func TestUserServiceContract(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping contract test in short mode")
    }

    conn, err := grpc.NewClient(
        "localhost:8081",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    assert.NoError(t, err)
    defer conn.Close()

    client := userpb.NewUserServiceClient(conn)

    t.Run("GetUser returns expected fields", func(t *testing.T) {
        resp, err := client.GetUser(context.Background(), &userpb.GetUserRequest{
            Id: 1, // Known seed data
        })
        assert.NoError(t, err)

        // Contract: these fields must exist and be non-empty
        assert.NotZero(t, resp.Id)
        assert.NotEmpty(t, resp.Username)
        assert.NotEmpty(t, resp.Email)
        // Contract: email must be a valid format
        assert.Contains(t, resp.Email, "@")
    })

    t.Run("GetUser with non-existent ID returns NotFound", func(t *testing.T) {
        _, err := client.GetUser(context.Background(), &userpb.GetUserRequest{
            Id: 999999999,
        })
        assert.Error(t, err)
        // Contract: non-existent user returns gRPC NotFound
        st, ok := status.FromError(err)
        assert.True(t, ok)
        assert.Equal(t, codes.NotFound, st.Code())
    })
}
```

### ✅ Proto-Based Contract Validation

```go
// Validate backward compatibility of proto files
// Run this in CI to catch breaking changes

// proto_compat_test.go
func TestProtoBackwardCompatibility(t *testing.T) {
    // Use buf (https://buf.build) for proto linting and breaking change detection
    // In CI pipeline:
    // buf breaking --against '.git#branch=main'
    //
    // Rules enforced:
    // - No removing fields
    // - No changing field numbers
    // - No changing field types
    // - No renaming RPCs
    t.Log("Run 'buf breaking --against .git#branch=main' in CI")
}
```

---

## 4. Load Testing

### ✅ k6 Load Test Script

```javascript
// loadtest/api_load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const latency = new Trend('api_latency');

export const options = {
    stages: [
        { duration: '30s', target: 50 },   // Ramp up
        { duration: '1m',  target: 100 },   // Stay at 100 VUs
        { duration: '30s', target: 200 },   // Peak load
        { duration: '30s', target: 0 },     // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<200', 'p(99)<500'],  // SLO: p95 < 200ms
        errors: ['rate<0.01'],                            // Error rate < 1%
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8888';

export default function () {
    // GET /api/v1/posts — list endpoint
    const listRes = http.get(`${BASE_URL}/api/v1/posts?pageSize=20`);
    check(listRes, {
        'list status 200': (r) => r.status === 200,
        'list has data': (r) => JSON.parse(r.body).data !== null,
    });
    errorRate.add(listRes.status !== 200);
    latency.add(listRes.timings.duration);

    sleep(0.5);

    // GET /api/v1/posts/:id — detail endpoint
    const detailRes = http.get(`${BASE_URL}/api/v1/posts/1`);
    check(detailRes, {
        'detail status 200': (r) => r.status === 200,
    });
    errorRate.add(detailRes.status !== 200);
    latency.add(detailRes.timings.duration);

    sleep(0.5);
}

// Run: k6 run --env BASE_URL=http://localhost:8888 loadtest/api_load_test.js
```

### ✅ vegeta Load Test

```bash
# Install: go install github.com/tsenart/vegeta@latest

# Constant rate test: 100 RPS for 30 seconds
echo "GET http://localhost:8888/api/v1/posts?pageSize=20" | \
  vegeta attack -rate=100 -duration=30s | \
  vegeta report

# Ramping test with multiple targets
cat <<'EOF' > targets.txt
GET http://localhost:8888/api/v1/posts?pageSize=20
GET http://localhost:8888/api/v1/posts/1
GET http://localhost:8888/api/v1/tags
EOF

vegeta attack -targets=targets.txt -rate=200 -duration=1m | \
  vegeta report -type=text

# Generate latency histogram
vegeta attack -targets=targets.txt -rate=100 -duration=30s | \
  vegeta report -type='hist[0,5ms,10ms,25ms,50ms,100ms,250ms,500ms,1s]'

# Plot results (HTML)
vegeta attack -targets=targets.txt -rate=100 -duration=30s | \
  vegeta plot > results.html
```

---

## 5. Go Benchmark

### ✅ Benchmark Test Patterns

```go
// internal/logic/benchmark_test.go
package logic

import (
    "context"
    "testing"
)

// BenchmarkCalculateDiscount measures pure logic performance
func BenchmarkCalculateDiscount(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _, _ = CalculateDiscount(199.99, 2)
    }
}

// BenchmarkSerializeResponse measures JSON serialization
func BenchmarkSerializeResponse(b *testing.B) {
    resp := &types.PostListResponse{
        Posts: generateTestPosts(100), // Helper: generate 100 posts
    }

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _, _ = json.Marshal(resp)
    }
}

// BenchmarkCursorEncode measures cursor pagination encoding
func BenchmarkCursorEncode(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _ = cursor.Encode(int64(i), time.Now())
    }
}

// Benchmark with different input sizes using sub-benchmarks
func BenchmarkBatchInsert(b *testing.B) {
    sizes := []int{10, 100, 1000}
    for _, size := range sizes {
        b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
            items := generateTestItems(size)
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _ = batchInsertItems(context.Background(), items)
            }
        })
    }
}
```

### ✅ Running Benchmarks

```bash
# Run all benchmarks in a package
go test -bench=. -benchmem ./internal/logic/...

# Run specific benchmark with CPU profile
go test -bench=BenchmarkCalculateDiscount -benchmem -cpuprofile=cpu.prof ./internal/logic/

# Compare benchmarks between changes (install benchstat)
# go install golang.org/x/perf/cmd/benchstat@latest
go test -bench=. -benchmem -count=10 ./internal/logic/ > old.txt
# ... make changes ...
go test -bench=. -benchmem -count=10 ./internal/logic/ > new.txt
benchstat old.txt new.txt
```

### ✅ Benchmark Output Interpretation

```
BenchmarkCalculateDiscount-8    50000000    23.4 ns/op    0 B/op    0 allocs/op
│                          │    │           │             │          └── allocations per op
│                          │    │           │             └── bytes allocated per op
│                          │    │           └── nanoseconds per operation
│                          │    └── iterations run
│                          └── GOMAXPROCS (CPU cores)
└── benchmark name
```

---

## 6. pprof Profiling

### ✅ Enable pprof in go-zero Service

```go
// main.go — add pprof endpoints for debugging
import (
    _ "net/http/pprof" // Register pprof handlers
    "net/http"
)

func main() {
    var c config.Config
    conf.MustLoad(*configFile, &c)

    // Start pprof server on a separate port (NEVER expose in production without auth)
    go func() {
        pprofAddr := os.Getenv("PPROF_ADDR")
        if pprofAddr == "" {
            pprofAddr = "localhost:6060" // Only bind to localhost
        }
        logx.Infof("pprof listening on %s", pprofAddr)
        if err := http.ListenAndServe(pprofAddr, nil); err != nil {
            logx.Errorf("pprof server error: %v", err)
        }
    }()

    server := rest.MustNewServer(c.RestConf)
    defer server.Stop()
    // ...
}
```

### ✅ CPU Profiling

```bash
# Capture 30-second CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Common pprof commands:
# (pprof) top 20       — top 20 CPU consumers
# (pprof) list FuncName — show source code with CPU time
# (pprof) web          — open interactive graph in browser
# (pprof) flamegraph   — generate flame graph

# One-liner: fetch profile and open web UI
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

### ✅ Memory (Heap) Profiling

```bash
# Current heap snapshot
go tool pprof http://localhost:6060/debug/pprof/heap

# Allocs profile (all allocations since start)
go tool pprof http://localhost:6060/debug/pprof/allocs

# Common analysis:
# (pprof) top 20 -cum   — top allocators by cumulative size
# (pprof) list FuncName  — show allocations in source code

# Compare two heap snapshots to find leaks
go tool pprof -base heap_before.prof heap_after.prof
```

### ✅ Goroutine Profiling

```bash
# Check for goroutine leaks
go tool pprof http://localhost:6060/debug/pprof/goroutine

# Quick goroutine dump (text format)
curl http://localhost:6060/debug/pprof/goroutine?debug=2

# Block profiling (contention analysis)
# Requires: runtime.SetBlockProfileRate(1) in init()
go tool pprof http://localhost:6060/debug/pprof/block

# Mutex contention profiling
# Requires: runtime.SetMutexProfileFraction(1) in init()
go tool pprof http://localhost:6060/debug/pprof/mutex
```

### ✅ Programmatic Profiling in Tests

```go
import (
    "os"
    "runtime/pprof"
    "testing"
)

func TestHeavyOperation_WithProfile(t *testing.T) {
    // CPU profile
    cpuFile, _ := os.Create("cpu_test.prof")
    defer cpuFile.Close()
    pprof.StartCPUProfile(cpuFile)
    defer pprof.StopCPUProfile()

    // Run the operation under test
    for i := 0; i < 1000; i++ {
        heavyOperation()
    }

    // Memory profile
    memFile, _ := os.Create("mem_test.prof")
    defer memFile.Close()
    pprof.WriteHeapProfile(memFile)

    // Analyze: go tool pprof -http=:8080 cpu_test.prof
}
```

---

## 7. Chaos Engineering

### ✅ Fault Injection Patterns

```go
// internal/middleware/fault_injection.go
// Only enable in staging/testing — NEVER in production

type FaultInjectionMiddleware struct {
    enabled    bool
    errorRate  float64 // 0.0 to 1.0
    delayMs    int     // Added latency in milliseconds
}

func NewFaultInjectionMiddleware() *FaultInjectionMiddleware {
    return &FaultInjectionMiddleware{
        enabled:   os.Getenv("FAULT_INJECTION_ENABLED") == "true",
        errorRate: parseFloat(os.Getenv("FAULT_INJECTION_ERROR_RATE"), 0),
        delayMs:   parseInt(os.Getenv("FAULT_INJECTION_DELAY_MS"), 0),
    }
}

func (m *FaultInjectionMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if !m.enabled {
            next(w, r)
            return
        }

        // Inject random latency
        if m.delayMs > 0 {
            jitter := time.Duration(rand.Intn(m.delayMs)) * time.Millisecond
            time.Sleep(jitter)
        }

        // Inject random errors
        if m.errorRate > 0 && rand.Float64() < m.errorRate {
            httpx.WriteJsonCtx(r.Context(), w, http.StatusInternalServerError,
                map[string]string{"error": "injected fault"})
            return
        }

        next(w, r)
    }
}
```

### ✅ Network Partition Simulation (Docker Compose)

```yaml
# docker-compose.chaos.yaml
# Simulate network partition between services
services:
  user-api:
    networks:
      - partition-a

  user-rpc:
    networks:
      - partition-b  # Isolated from user-api

  # Restore connectivity:
  # docker network connect partition-a user-rpc

networks:
  partition-a:
  partition-b:
```

### ✅ Resource Exhaustion Test

```bash
# Limit container resources for chaos testing
docker run --cpus=0.5 --memory=128m your-service:test

# Stress test with concurrent connections
# Use k6 or vegeta to flood the resource-limited service
k6 run --vus 200 --duration 60s loadtest/api_load_test.js
```

### ✅ Dependency Failure Test

```go
// Test how service behaves when dependencies are down
func TestServiceWithRedisDown(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping chaos test in short mode")
    }

    // Point Redis to a non-existent host
    svcCtx := &svc.ServiceContext{
        Config: config.Config{
            Redis: redis.RedisConf{
                Host: "localhost:19999", // Non-existent
                Type: "node",
            },
        },
    }

    logic := NewGetUserLogic(context.Background(), svcCtx)
    resp, err := logic.GetUser(&types.GetUserRequest{Id: 1})

    // Service should degrade gracefully, not panic
    // Expect: cache miss → fallback to DB, or return cached error
    if err != nil {
        assert.NotContains(t, err.Error(), "panic")
    }
}
```

---

## 8. Test Organization & CI

### ✅ Recommended Directory Structure

```
service/
├── internal/
│   ├── logic/
│   │   ├── get_user_logic.go
│   │   ├── get_user_logic_test.go        # Unit tests
│   │   └── benchmark_test.go             # Benchmarks
│   └── model/
│       ├── user_model.go
│       └── user_model_test.go            # Integration tests (TestMain setup)
├── test/
│   ├── contract/
│   │   └── user_contract_test.go         # Contract tests
│   ├── chaos/
│   │   └── fault_injection_test.go       # Chaos tests
│   └── loadtest/
│       ├── api_load_test.js              # k6 scripts
│       └── targets.txt                   # vegeta targets
└── Makefile
```

### ✅ Makefile Test Targets

```makefile
.PHONY: test test-unit test-integration test-contract test-load test-bench test-cover

# Run unit tests only (fast, no external deps)
test-unit:
	go test -short -race -count=1 ./internal/logic/...

# Run integration tests (requires DB, Redis)
test-integration:
	go test -race -count=1 -run Integration ./internal/model/...

# Run contract tests (requires services running)
test-contract:
	go test -race -count=1 ./test/contract/...

# Run load tests
test-load:
	k6 run --env BASE_URL=http://localhost:8888 test/loadtest/api_load_test.js

# Run benchmarks
test-bench:
	go test -bench=. -benchmem ./internal/logic/...

# Run all tests with coverage
test-cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -1
	@echo "Open HTML report: go tool cover -html=coverage.out"

# Full test suite
test: test-unit test-integration
	@echo "All tests passed"
```

### ✅ CI Pipeline Integration

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]

jobs:
  unit-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test -short -race -coverprofile=coverage.out ./...
      - run: go tool cover -func=coverage.out | tail -1

  integration-test:
    runs-on: ubuntu-latest
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: testpass
          MYSQL_DATABASE: testdb
        ports: ['3306:3306']
        options: >-
          --health-cmd="mysqladmin ping -h localhost"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=5
      redis:
        image: redis:7-alpine
        ports: ['6379:6379']
    env:
      TEST_DATABASE_DSN: "root:testpass@tcp(localhost:3306)/testdb?parseTime=true"
      TEST_REDIS_HOST: "localhost:6379"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test -race -count=1 ./internal/model/...

  benchmark:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test -bench=. -benchmem -count=5 ./... > bench.txt
      - uses: actions/upload-artifact@v4
        with:
          name: benchmarks
          path: bench.txt
```

---

## Best Practices Summary

### ✅ DO:
- Test logic layer through ServiceContext — mock models and RPC clients
- Use table-driven tests for all multi-case scenarios
- Run `-race` flag in all test commands to detect data races
- Use `TestMain` for integration test setup/teardown
- Separate test types: `-short` for unit, tag/skip for integration and contract
- Track benchmark changes with `benchstat` across commits
- Use pprof only on localhost — NEVER expose to public network
- Run chaos tests in staging environment, not production
- Maintain 80%+ coverage on logic and model layers

### ❌ DON'T:
- Mock everything — integration tests catch what mocks miss
- Skip `-race` flag — data races are hard to detect otherwise
- Use production database for integration tests — always use isolated test DB
- Commit pprof profiles — they contain runtime information
- Enable fault injection middleware in production builds
- Rely solely on unit tests — the testing pyramid needs all levels
- Ignore benchmark regressions — track and investigate performance changes

For database testing patterns, see [Database Patterns](./database-patterns.md).
For resilience testing (circuit breaker, rate limiting), see [Resilience Patterns](./resilience-patterns.md).
For deployment CI/CD integration, see [Deployment Patterns](./deployment-patterns.md).

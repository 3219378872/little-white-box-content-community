# Concurrency & Data Safety Patterns

## Table of Contents
- [Decision Matrix: Which Pattern to Use](#decision-matrix-which-pattern-to-use)
- [1. CAS — Conditional Atomic Update](#1-cas--conditional-atomic-update)
- [2. Optimistic Locking](#2-optimistic-locking)
- [3. Pessimistic Locking](#3-pessimistic-locking)
- [4. Distributed Lock (Redis)](#4-distributed-lock-redis)
- [5. Idempotency](#5-idempotency)
- [6. Distributed Transactions — Saga Pattern](#6-distributed-transactions--saga-pattern)
- [7. Saga Compensation Recovery](#7-saga-compensation-recovery)
- [8. Distributed Lock Expiration Strategy](#8-distributed-lock-expiration-strategy)
- [Best Practices Summary](#best-practices-summary)

Patterns for preventing data corruption, lost updates, and race conditions in go-zero microservices.

## Decision Matrix: Which Pattern to Use

```
Is the operation a single atomic SQL statement?
  YES → Use CAS (Conditional Atomic Update)
  NO  → Is there high contention (many writers on same row)?
          YES → Use Pessimistic Locking (SELECT FOR UPDATE)
          NO  → Use Optimistic Locking (version field)

Is the critical section cross-instance (multiple service replicas)?
  YES → Use Distributed Lock (Redis)
  NO  → Is it cross-service (multiple microservices)?
          YES → Use Distributed Transaction (Saga/TCC)
          NO  → Use Database Transaction (TransactCtx)
```

---

## 1. CAS — Conditional Atomic Update

The simplest and most performant approach. Combine the check and update into a single SQL statement.

### When to Use
- Counter increments/decrements
- Status transitions (state machines)
- Balance operations
- Any update where the condition can be expressed in SQL

### ✅ Atomic Counter

```go
func (m *customPostModel) IncrementViewCount(ctx context.Context, postId int64) error {
    query := `UPDATE posts SET view_count = view_count + 1 WHERE id = ?`
    _, err := m.ExecNoCacheCtx(ctx, query, postId)
    return err
}
```

### ✅ State Machine Transition

```go
func (m *customOrderModel) UpdateStatus(ctx context.Context, orderId int64, fromStatus, toStatus string) error {
    query := `UPDATE orders SET status = ?, updated_at = NOW() WHERE id = ? AND status = ?`
    result, err := m.ExecNoCacheCtx(ctx, query, toStatus, orderId, fromStatus)
    if err != nil {
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return fmt.Errorf("order %d status transition %s -> %s failed: current status mismatch", orderId, fromStatus, toStatus)
    }

    return nil
}
```

### ✅ Balance Deduction (Prevent Negative)

```go
func (m *customAccountModel) Deduct(ctx context.Context, userId int64, amount float64) error {
    query := `UPDATE accounts SET balance = balance - ? WHERE user_id = ? AND balance >= ?`
    result, err := m.ExecNoCacheCtx(ctx, query, amount, userId, amount)
    if err != nil {
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return errors.New("insufficient balance")
    }

    return nil
}
```

---

## 2. Optimistic Locking

Use a `version` (or `updated_at`) column to detect concurrent modifications. Best for low-contention scenarios.

### Schema Requirement

```sql
ALTER TABLE users ADD COLUMN version INT NOT NULL DEFAULT 1;
```

### ✅ Model Interface

```go
type UsersModel interface {
    usersModel
    UpdateWithVersion(ctx context.Context, data *Users) error
}
```

### ✅ Implementation

```go
func (m *customUsersModel) UpdateWithVersion(ctx context.Context, data *Users) error {
    query := `UPDATE users SET name = ?, email = ?, age = ?, version = version + 1
              WHERE id = ? AND version = ?`
    result, err := m.ExecNoCacheCtx(ctx, query, data.Name, data.Email, data.Age, data.Id, data.Version)
    if err != nil {
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return ErrOptimisticLock // concurrent modification detected
    }

    return nil
}
```

### ✅ Logic Layer with Retry

```go
var ErrOptimisticLock = errors.New("optimistic lock conflict")

func (l *UpdateUserLogic) UpdateUser(req *types.UpdateUserRequest) error {
    const maxRetries = 3

    for attempt := 0; attempt < maxRetries; attempt++ {
        // 1. Read current state
        user, err := l.svcCtx.UsersModel.FindOne(l.ctx, req.Id)
        if err != nil {
            return err
        }

        // 2. Apply changes
        user.Name = req.Name
        user.Age = int64(req.Age)

        // 3. Write with version check
        err = l.svcCtx.UsersModel.UpdateWithVersion(l.ctx, user)
        if err == nil {
            return nil // success
        }

        if !errors.Is(err, ErrOptimisticLock) {
            return err // non-retryable error
        }

        l.Logger.Infof("optimistic lock conflict on user %d, retry %d/%d", req.Id, attempt+1, maxRetries)
    }

    return errors.New("update failed after max retries due to concurrent modifications")
}
```

---

## 3. Pessimistic Locking

Lock the row in the database during a transaction. Best for high-contention scenarios.

### ✅ SELECT ... FOR UPDATE

```go
func (l *TransferLogic) Transfer(fromId, toId int64, amount float64) error {
    return l.svcCtx.DB.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
        // Lock rows in consistent order to prevent deadlock (always lock smaller ID first)
        firstId, secondId := fromId, toId
        if fromId > toId {
            firstId, secondId = toId, fromId
        }

        // 1. Lock both accounts (SELECT FOR UPDATE)
        var first, second Account
        err := session.QueryRowCtx(ctx, &first,
            `SELECT id, balance FROM accounts WHERE id = ? FOR UPDATE`, firstId)
        if err != nil {
            return fmt.Errorf("failed to lock account %d: %w", firstId, err)
        }

        err = session.QueryRowCtx(ctx, &second,
            `SELECT id, balance FROM accounts WHERE id = ? FOR UPDATE`, secondId)
        if err != nil {
            return fmt.Errorf("failed to lock account %d: %w", secondId, err)
        }

        // 2. Business validation
        fromAccount := &first
        if firstId != fromId {
            fromAccount = &second
        }
        if fromAccount.Balance < amount {
            return errors.New("insufficient balance")
        }

        // 3. Execute transfer (safe — rows are locked)
        _, err = session.ExecCtx(ctx, `UPDATE accounts SET balance = balance - ? WHERE id = ?`, amount, fromId)
        if err != nil {
            return err
        }

        _, err = session.ExecCtx(ctx, `UPDATE accounts SET balance = balance + ? WHERE id = ?`, amount, toId)
        if err != nil {
            return err
        }

        return nil
    })
}
```

### Deadlock Prevention Rules

1. **Always lock rows in a consistent order** (e.g., by ascending ID)
2. **Keep transactions short** — minimize time between lock acquisition and commit
3. **Set lock wait timeout** — avoid indefinite blocking

```sql
-- MySQL: set lock wait timeout (seconds)
SET innodb_lock_wait_timeout = 5;
```

---

## 4. Distributed Lock (Redis)

For protecting critical sections across multiple service instances.

### ✅ Redis Lock with go-zero

```go
import "github.com/zeromicro/go-zero/core/stores/redis"

func (l *OrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    // Lock key per user to prevent duplicate order creation
    lockKey := fmt.Sprintf("lock:order:user:%d", req.UserId)

    lock := redis.NewRedisLock(l.svcCtx.Redis, lockKey)
    lock.SetExpire(10) // lock expires in 10 seconds (prevent deadlock on crash)

    acquired, err := lock.AcquireCtx(l.ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to acquire lock: %w", err)
    }
    if !acquired {
        return nil, errors.New("operation in progress, please try again later")
    }
    defer func() {
        if _, err := lock.ReleaseCtx(l.ctx); err != nil {
            l.Logger.Errorf("failed to release lock %s: %v", lockKey, err)
        }
    }()

    // Critical section — only one instance can execute this for the same user
    return l.doCreateOrder(req)
}
```

### When to Use vs Database Locks

| Scenario | Use Database Lock | Use Redis Lock |
|----------|------------------|----------------|
| Single DB update | ✅ | Overkill |
| Cross-table transaction | ✅ | Optional layer |
| Cross-service operation | ❌ | ✅ |
| Rate limit per user | ❌ | ✅ |
| Prevent duplicate requests | ❌ | ✅ |

---

## 5. Idempotency

Ensure that repeating the same request produces the same result. Critical for payments, orders, and any non-repeatable operation.

### Schema

```sql
CREATE TABLE idempotency_keys (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `idempotency_key` varchar(64) NOT NULL,
    `response_code` int NOT NULL DEFAULT 0,
    `response_body` text,
    `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_idempotency_key` (`idempotency_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### ✅ Idempotency Middleware

```go
func (m *IdempotencyMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Only apply to non-safe methods (POST, PUT, PATCH)
        if r.Method == http.MethodGet || r.Method == http.MethodDelete {
            next.ServeHTTP(w, r)
            return
        }

        idempotencyKey := r.Header.Get("Idempotency-Key")
        if idempotencyKey == "" {
            next.ServeHTTP(w, r)
            return
        }

        // Check if request was already processed
        cached, err := m.svcCtx.IdempotencyModel.FindByKey(r.Context(), idempotencyKey)
        if err == nil {
            // Return cached response
            w.WriteHeader(cached.ResponseCode)
            w.Write([]byte(cached.ResponseBody))
            return
        }

        // Capture response
        recorder := httptest.NewRecorder()
        next.ServeHTTP(recorder, r)

        // Store response for idempotency
        _ = m.svcCtx.IdempotencyModel.Insert(r.Context(), &IdempotencyKey{
            IdempotencyKey: idempotencyKey,
            ResponseCode:   recorder.Code,
            ResponseBody:   recorder.Body.String(),
        })

        // Write actual response
        for k, v := range recorder.Header() {
            w.Header()[k] = v
        }
        w.WriteHeader(recorder.Code)
        w.Write(recorder.Body.Bytes())
    }
}
```

### ✅ Database-Level Idempotency (Simpler)

For cases where a unique business key provides natural idempotency:

```go
func (l *CreatePaymentLogic) CreatePayment(req *types.CreatePaymentRequest) error {
    // Use order_id + payment_type as natural idempotency key
    // MySQL: INSERT IGNORE; PostgreSQL: INSERT ... ON CONFLICT DO NOTHING
    query := `INSERT IGNORE INTO payments(order_id, payment_type, amount, status)
              VALUES(?, ?, ?, 'pending')`
    // PostgreSQL equivalent:
    // query := `INSERT INTO payments(order_id, payment_type, amount, status)
    //           VALUES($1, $2, $3, 'pending') ON CONFLICT (order_id, payment_type) DO NOTHING`
    result, err := l.svcCtx.DB.ExecCtx(l.ctx, query, req.OrderId, req.PaymentType, req.Amount)
    if err != nil {
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        // Payment already exists — idempotent response
        l.Logger.Infof("duplicate payment request for order %d, returning existing", req.OrderId)
        return nil
    }

    return nil
}
```

---

## 6. Distributed Transactions — Saga Pattern

For operations spanning multiple microservices where ACID transactions are impossible.

### Orchestration Saga (Recommended)

A central coordinator manages the transaction steps and compensations.

```go
// saga/saga.go — Generic Saga orchestrator
type SagaStep struct {
    Name       string
    Execute    func(ctx context.Context) error
    Compensate func(ctx context.Context) error
}

func ExecuteSaga(ctx context.Context, steps []SagaStep) error {
    var completedSteps []SagaStep

    for _, step := range steps {
        if err := step.Execute(ctx); err != nil {
            // Compensate in reverse order
            for i := len(completedSteps) - 1; i >= 0; i-- {
                if compErr := completedSteps[i].Compensate(ctx); compErr != nil {
                    logx.Errorf("saga compensation failed for step %s: %v", completedSteps[i].Name, compErr)
                    // Log for manual intervention — compensation must not be silently lost
                }
            }
            return fmt.Errorf("saga failed at step %s: %w", step.Name, err)
        }
        completedSteps = append(completedSteps, step)
    }

    return nil
}
```

### ✅ Example: Cross-Service Order Creation

```go
func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    var orderId int64

    steps := []SagaStep{
        {
            Name: "create_order",
            Execute: func(ctx context.Context) error {
                resp, err := l.svcCtx.OrderRpc.CreateOrder(ctx, &order.CreateOrderRequest{
                    UserId: req.UserId,
                    Amount: req.Amount,
                })
                if err != nil {
                    return err
                }
                orderId = resp.OrderId
                return nil
            },
            Compensate: func(ctx context.Context) error {
                _, err := l.svcCtx.OrderRpc.CancelOrder(ctx, &order.CancelOrderRequest{
                    OrderId: orderId,
                })
                return err
            },
        },
        {
            Name: "deduct_inventory",
            Execute: func(ctx context.Context) error {
                _, err := l.svcCtx.InventoryRpc.DeductStock(ctx, &inventory.DeductStockRequest{
                    ProductId: req.ProductId,
                    Quantity:  req.Quantity,
                })
                return err
            },
            Compensate: func(ctx context.Context) error {
                _, err := l.svcCtx.InventoryRpc.RestoreStock(ctx, &inventory.RestoreStockRequest{
                    ProductId: req.ProductId,
                    Quantity:  req.Quantity,
                })
                return err
            },
        },
        {
            Name: "deduct_balance",
            Execute: func(ctx context.Context) error {
                _, err := l.svcCtx.PaymentRpc.Deduct(ctx, &payment.DeductRequest{
                    UserId: req.UserId,
                    Amount: req.Amount,
                })
                return err
            },
            Compensate: func(ctx context.Context) error {
                _, err := l.svcCtx.PaymentRpc.Refund(ctx, &payment.RefundRequest{
                    UserId: req.UserId,
                    Amount: req.Amount,
                })
                return err
            },
        },
    }

    if err := ExecuteSaga(l.ctx, steps); err != nil {
        l.Logger.Errorf("order saga failed: %v", err)
        return nil, err
    }

    return &types.CreateOrderResponse{OrderId: orderId}, nil
}
```

### DTM Integration (Production-Grade)

For production use, consider [DTM](https://github.com/dtm-labs/dtm) — a distributed transaction framework with native go-zero support:

```go
import "github.com/dtm-labs/dtmgrpc"

func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) error {
    gid := dtmgrpc.MustGenGid(dtmServer)
    saga := dtmgrpc.NewSagaGrpc(dtmServer, gid).
        Add(orderServer+"/order.Order/Create", orderServer+"/order.Order/CreateRevert", &orderReq).
        Add(inventoryServer+"/inventory.Inventory/Deduct", inventoryServer+"/inventory.Inventory/DeductRevert", &inventoryReq).
        Add(paymentServer+"/payment.Payment/Pay", paymentServer+"/payment.Payment/PayRevert", &paymentReq)

    return saga.Submit()
}
```

---

## 7. Saga Compensation Recovery

When a Saga compensation step itself fails, the system enters an inconsistent state. This section covers recovery strategies.

### ✅ Compensation Retry with Dead Letter

```go
type SagaCompensationRecord struct {
    SagaId        string
    StepName      string
    Payload       []byte
    RetryCount    int
    MaxRetries    int
    NextRetryAt   time.Time
    Status        string // "pending", "retrying", "failed", "completed"
    LastError     string
    CreatedAt     time.Time
}

// PersistFailedCompensation stores failed compensation for async retry
func PersistFailedCompensation(ctx context.Context, db sqlx.SqlConn, record *SagaCompensationRecord) error {
    query := `INSERT INTO saga_compensation_records(saga_id, step_name, payload, retry_count, max_retries, next_retry_at, status, last_error)
              VALUES(?, ?, ?, 0, ?, ?, 'pending', ?)`
    _, err := db.ExecCtx(ctx, query, record.SagaId, record.StepName, record.Payload,
        record.MaxRetries, record.NextRetryAt, record.LastError)
    return err
}
```

### ✅ Enhanced ExecuteSaga with Persistent Retry

```go
func ExecuteSaga(ctx context.Context, sagaId string, steps []SagaStep, db sqlx.SqlConn) error {
    var completedSteps []SagaStep

    for _, step := range steps {
        if err := step.Execute(ctx); err != nil {
            // Compensate in reverse order
            for i := len(completedSteps) - 1; i >= 0; i-- {
                if compErr := completedSteps[i].Compensate(ctx); compErr != nil {
                    logx.Errorf("saga %s: compensation failed for step %s: %v", sagaId, completedSteps[i].Name, compErr)

                    // Persist for async retry instead of silent logging
                    payload, _ := json.Marshal(completedSteps[i].CompensationData)
                    _ = PersistFailedCompensation(ctx, db, &SagaCompensationRecord{
                        SagaId:      sagaId,
                        StepName:    completedSteps[i].Name,
                        Payload:     payload,
                        MaxRetries:  5,
                        NextRetryAt: time.Now().Add(30 * time.Second),
                        LastError:   compErr.Error(),
                    })
                }
            }
            return fmt.Errorf("saga %s failed at step %s: %w", sagaId, step.Name, err)
        }
        completedSteps = append(completedSteps, step)
    }
    return nil
}
```

### ✅ Background Compensation Worker

```go
// CompensationWorker retries failed compensations with exponential backoff
type CompensationWorker struct {
    db       sqlx.SqlConn
    handlers map[string]func(ctx context.Context, payload []byte) error
    interval time.Duration
}

func (w *CompensationWorker) Start(ctx context.Context) {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.processPendingCompensations(ctx)
        }
    }
}

func (w *CompensationWorker) processPendingCompensations(ctx context.Context) {
    query := `SELECT saga_id, step_name, payload, retry_count, max_retries
              FROM saga_compensation_records
              WHERE status IN ('pending', 'retrying')
                AND next_retry_at <= NOW()
              LIMIT 50
              FOR UPDATE SKIP LOCKED`

    var records []SagaCompensationRecord
    if err := w.db.QueryRowsCtx(ctx, &records, query); err != nil {
        logx.Errorf("compensation worker: query failed: %v", err)
        return
    }

    for _, record := range records {
        handler, ok := w.handlers[record.StepName]
        if !ok {
            logx.Errorf("compensation worker: no handler for step %s", record.StepName)
            continue
        }

        if err := handler(ctx, record.Payload); err != nil {
            record.RetryCount++
            if record.RetryCount >= record.MaxRetries {
                // Escalate to dead letter — requires manual intervention
                w.db.ExecCtx(ctx, `UPDATE saga_compensation_records
                    SET status = 'failed', retry_count = ?, last_error = ? WHERE saga_id = ? AND step_name = ?`,
                    record.RetryCount, err.Error(), record.SagaId, record.StepName)
                logx.Errorf("saga %s step %s: compensation exhausted after %d retries, manual intervention required",
                    record.SagaId, record.StepName, record.MaxRetries)
            } else {
                // Exponential backoff: 30s, 60s, 120s, 240s, 480s
                backoff := time.Duration(math.Pow(2, float64(record.RetryCount))) * 30 * time.Second
                w.db.ExecCtx(ctx, `UPDATE saga_compensation_records
                    SET status = 'retrying', retry_count = ?, next_retry_at = ?, last_error = ? WHERE saga_id = ? AND step_name = ?`,
                    record.RetryCount, time.Now().Add(backoff), err.Error(), record.SagaId, record.StepName)
            }
        } else {
            w.db.ExecCtx(ctx, `UPDATE saga_compensation_records SET status = 'completed' WHERE saga_id = ? AND step_name = ?`,
                record.SagaId, record.StepName)
        }
    }
}
```

### Compensation Table Schema

```sql
CREATE TABLE saga_compensation_records (
    saga_id       VARCHAR(64) NOT NULL,
    step_name     VARCHAR(64) NOT NULL,
    payload       JSON,
    retry_count   INT NOT NULL DEFAULT 0,
    max_retries   INT NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMP NOT NULL,
    status        ENUM('pending', 'retrying', 'completed', 'failed') NOT NULL DEFAULT 'pending',
    last_error    TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (saga_id, step_name),
    INDEX idx_status_retry (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

---

## 8. Distributed Lock Expiration Strategy

Choosing the right lock expiration time is critical: too short causes premature release mid-operation; too long causes prolonged blocking on holder crash.

### ✅ Decision Guide

```
Lock Expiration = Expected Operation Time × 3 + Safety Buffer

Examples:
┌─────────────────────────────┬──────────────────┬────────────────┐
│ Operation                   │ Expected Time    │ Lock TTL       │
├─────────────────────────────┼──────────────────┼────────────────┤
│ Inventory deduction         │ 50ms             │ 500ms          │
│ Payment processing          │ 2s               │ 10s            │
│ Report generation           │ 30s              │ 120s           │
│ Batch data sync             │ 5min             │ 20min          │
└─────────────────────────────┴──────────────────┴────────────────┘
```

### ✅ Lock with Auto-Renewal (Watchdog Pattern)

For long-running operations where the duration is unpredictable, use a watchdog to extend the lock while the operation is still running:

```go
type WatchdogLock struct {
    redis    *redis.Redis
    key      string
    value    string
    ttl      time.Duration
    stopCh   chan struct{}
}

func NewWatchdogLock(rds *redis.Redis, key string, ttl time.Duration) *WatchdogLock {
    return &WatchdogLock{
        redis:  rds,
        key:    key,
        value:  fmt.Sprintf("%d:%s", os.Getpid(), uuid.NewString()),
        ttl:    ttl,
        stopCh: make(chan struct{}),
    }
}

func (l *WatchdogLock) Lock(ctx context.Context) (bool, error) {
    ok, err := l.redis.SetnxExCtx(ctx, l.key, l.value, int(l.ttl.Seconds()))
    if err != nil || !ok {
        return false, err
    }

    // Start watchdog: renew lock at 1/3 TTL intervals
    go l.watchdog(ctx)
    return true, nil
}

func (l *WatchdogLock) watchdog(ctx context.Context) {
    renewInterval := l.ttl / 3
    ticker := time.NewTicker(renewInterval)
    defer ticker.Stop()

    for {
        select {
        case <-l.stopCh:
            return
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Only renew if we still own the lock (Lua script for atomicity)
            script := `if redis.call("GET", KEYS[1]) == ARGV[1] then
                return redis.call("EXPIRE", KEYS[1], ARGV[2])
            else
                return 0
            end`
            val, err := l.redis.EvalCtx(ctx, script, []string{l.key}, l.value, int(l.ttl.Seconds()))
            if err != nil || val.(int64) == 0 {
                logx.Errorf("watchdog: lock renewal failed for key %s", l.key)
                return
            }
        }
    }
}

func (l *WatchdogLock) Unlock(ctx context.Context) error {
    close(l.stopCh) // Stop watchdog first

    // Release lock only if we own it
    script := `if redis.call("GET", KEYS[1]) == ARGV[1] then
        return redis.call("DEL", KEYS[1])
    else
        return 0
    end`
    _, err := l.redis.EvalCtx(ctx, script, []string{l.key}, l.value)
    return err
}
```

### ✅ Usage Example

```go
func (l *DeductInventoryLogic) DeductInventory(req *types.DeductRequest) error {
    lock := NewWatchdogLock(l.svcCtx.Redis, fmt.Sprintf("lock:inventory:%d", req.ProductId), 10*time.Second)

    ok, err := lock.Lock(l.ctx)
    if err != nil {
        return fmt.Errorf("lock acquire error: %w", err)
    }
    if !ok {
        return fmt.Errorf("inventory is being updated by another process")
    }
    defer lock.Unlock(l.ctx)

    // Perform inventory deduction — even if it takes longer than 10s,
    // the watchdog will renew the lock automatically
    return l.doDeduct(req)
}
```

### ❌ Anti-Pattern: Fixed TTL Without Renewal

```go
// BAD: If deduct takes > 5s, another process can acquire the same lock
rds.SetnxExCtx(ctx, "lock:inventory:1", "owner1", 5)
// ... long operation ...
rds.DelCtx(ctx, "lock:inventory:1") // May delete another owner's lock!
```

---

## Best Practices Summary

### ✅ DO:
- Prefer CAS (single atomic SQL) over read-modify-write — it's simpler and faster
- Add `version` column to tables that require concurrent updates
- Lock rows in consistent order (ascending ID) to prevent deadlocks
- Set lock timeout to avoid indefinite blocking
- Use Redis distributed locks for cross-instance critical sections
- Implement idempotency for all payment/order creation endpoints
- Use Saga pattern for cross-service transactions
- Log all compensation failures for manual recovery

### ❌ DON'T:
- Use read-modify-write (FindOne → modify → Update) without locking
- Hold database locks across RPC calls (transaction scope too wide)
- Use distributed locks for single-database operations (overkill)
- Retry non-idempotent operations without deduplication
- Ignore compensation failures in Saga — they need manual resolution
- Lock rows in random order — causes deadlocks

For database CRUD operations, see [Database Patterns](./database-patterns.md).
For system-level resilience, see [Resilience Patterns](./resilience-patterns.md).

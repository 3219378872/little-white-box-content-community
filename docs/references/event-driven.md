# Event-Driven Patterns for go-zero

## Table of Contents
- [Overview](#overview)
- [1. Message Queue Integration](#1-message-queue-integration)
- [2. CQRS — Command Query Responsibility Segregation](#2-cqrs--command-query-responsibility-segregation)
- [3. Dead Letter Queue (DLQ)](#3-dead-letter-queue-dlq)
- [4. Transactional Outbox Pattern](#4-transactional-outbox-pattern)
- [5. Event Sourcing Lite](#5-event-sourcing-lite)
- [Decision Matrix](#decision-matrix)
- [Checklist](#checklist)
- [Cross-References](#cross-references)

## Overview

This guide covers event-driven architecture patterns for go-zero microservices:
- Message queue integration (Kafka, RabbitMQ, NATS)
- CQRS (Command Query Responsibility Segregation)
- Dead letter queues and error handling
- Transactional Outbox pattern

---

## 1. Message Queue Integration

### go-zero Queue Framework

go-zero provides a built-in queue framework (`go-queue`) that supports Kafka and other backends.

### Kafka Producer

```go
// pkg/mq/producer.go
import (
    "encoding/json"

    "github.com/zeromicro/go-queue/kq"
    "github.com/zeromicro/go-zero/core/logx"
)

type EventProducer struct {
    pusher *kq.Pusher
}

func NewEventProducer(brokers []string, topic string) *EventProducer {
    pusher := kq.NewPusher(brokers, topic)
    return &EventProducer{pusher: pusher}
}

func (p *EventProducer) Publish(ctx context.Context, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    if err := p.pusher.PushCtx(ctx, string(data)); err != nil {
        logx.WithContext(ctx).Errorf("failed to publish event: %v", err)
        return fmt.Errorf("failed to publish event: %w", err)
    }

    return nil
}
```

### Kafka Consumer

```go
// internal/mq/consumer.go
import (
    "github.com/zeromicro/go-queue/kq"
    "github.com/zeromicro/go-zero/core/service"
)

func NewConsumers(c config.Config, svcCtx *svc.ServiceContext) []service.Service {
    return []service.Service{
        kq.MustNewQueue(c.Kafka.OrderCreated, kq.WithHandle(func(k, v string) error {
            return handleOrderCreated(svcCtx, k, v)
        })),
        kq.MustNewQueue(c.Kafka.PaymentCompleted, kq.WithHandle(func(k, v string) error {
            return handlePaymentCompleted(svcCtx, k, v)
        })),
    }
}

func handleOrderCreated(svcCtx *svc.ServiceContext, key, value string) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal([]byte(value), &event); err != nil {
        logx.Errorf("failed to unmarshal OrderCreatedEvent: %v", err)
        // Return nil to skip — malformed messages should not be retried
        return nil
    }

    // Process event
    ctx := context.Background()
    l := logic.NewProcessOrderLogic(ctx, svcCtx)
    return l.ProcessOrder(&event)
}
```

### Configuration

```yaml
# etc/service.yaml
Kafka:
  OrderCreated:
    Brokers:
      - ${KAFKA_BROKER_1}
      - ${KAFKA_BROKER_2}
    Group: order-service
    Topic: order.created
    Offset: first     # "first" or "last"
    Consumers: 4      # Consumer goroutines
    Processors: 8     # Processing goroutines

  PaymentCompleted:
    Brokers:
      - ${KAFKA_BROKER_1}
    Group: order-service
    Topic: payment.completed
    Offset: first
    Consumers: 2
    Processors: 4
```

### Event Schema Definition

```go
// pkg/events/events.go
package events

import "time"

// Base event structure
type BaseEvent struct {
    EventId   string    `json:"event_id"`    // Unique event ID for idempotency
    EventType string    `json:"event_type"`  // e.g., "order.created"
    Source    string    `json:"source"`       // e.g., "order-service"
    Timestamp time.Time `json:"timestamp"`
    TraceId   string    `json:"trace_id"`    // For distributed tracing
}

type OrderCreatedEvent struct {
    BaseEvent
    OrderId  int64   `json:"order_id"`
    UserId   int64   `json:"user_id"`
    Amount   float64 `json:"amount"`
    Items    []Item  `json:"items"`
}

type PaymentCompletedEvent struct {
    BaseEvent
    PaymentId int64   `json:"payment_id"`
    OrderId   int64   `json:"order_id"`
    Amount    float64 `json:"amount"`
    Method    string  `json:"method"`
}
```

### RabbitMQ Integration

```go
// pkg/mq/rabbitmq.go
import amqp "github.com/rabbitmq/amqp091-go"

type RabbitProducer struct {
    conn    *amqp.Connection
    channel *amqp.Channel
}

func NewRabbitProducer(url string) (*RabbitProducer, error) {
    conn, err := amqp.Dial(url)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
    }

    ch, err := conn.Channel()
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("failed to open channel: %w", err)
    }

    return &RabbitProducer{conn: conn, channel: ch}, nil
}

func (p *RabbitProducer) Publish(ctx context.Context, exchange, routingKey string, event any) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    return p.channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
        ContentType:  "application/json",
        Body:         data,
        DeliveryMode: amqp.Persistent,
        MessageId:    uuid.New().String(),
        Timestamp:    time.Now(),
    })
}

func (p *RabbitProducer) Close() {
    p.channel.Close()
    p.conn.Close()
}
```

---

## 2. CQRS — Command Query Responsibility Segregation

### Architecture

```
Writes (Commands)                   Reads (Queries)
    │                                     │
    ▼                                     ▼
[Command Handler]                  [Query Handler]
    │                                     │
    ▼                                     ▼
[Write DB (MySQL)]  ──Event──>  [Read DB (Elasticsearch/Redis)]
```

### Command Side (Write)

```go
// internal/logic/command/create_post_logic.go
func (l *CreatePostLogic) CreatePost(req *types.CreatePostRequest) (*types.CreatePostResponse, error) {
    // Write to primary database
    post := &model.Post{
        Id:      l.svcCtx.Snowflake.Generate(),
        Title:   req.Title,
        Content: req.Content,
        UserId:  req.UserId,
    }

    _, err := l.svcCtx.PostModel.Insert(l.ctx, post)
    if err != nil {
        return nil, err
    }

    // Publish event for read-side sync
    event := events.PostCreatedEvent{
        BaseEvent: events.BaseEvent{
            EventId:   uuid.New().String(),
            EventType: "post.created",
            Source:    "content-service",
            Timestamp: time.Now(),
        },
        PostId:  post.Id,
        Title:   post.Title,
        Content: post.Content,
        UserId:  post.UserId,
    }

    if err := l.svcCtx.EventProducer.Publish(l.ctx, event); err != nil {
        // Log but don't fail — eventual consistency
        logx.WithContext(l.ctx).Errorf("failed to publish post.created event: %v", err)
    }

    return &types.CreatePostResponse{Id: post.Id}, nil
}
```

### Query Side (Read)

```go
// internal/logic/query/search_posts_logic.go
func (l *SearchPostsLogic) SearchPosts(req *types.SearchPostsRequest) (*types.SearchPostsResponse, error) {
    // Query from read-optimized store (Elasticsearch)
    result, err := l.svcCtx.ESClient.Search(l.ctx, "posts", map[string]any{
        "query": map[string]any{
            "multi_match": map[string]any{
                "query":  req.Keyword,
                "fields": []string{"title^2", "content"},
            },
        },
        "from": (req.Page - 1) * req.PageSize,
        "size": req.PageSize,
    })
    if err != nil {
        return nil, err
    }

    return &types.SearchPostsResponse{
        Total: result.Total,
        Items: convertESHits(result.Hits),
    }, nil
}
```

### Event Consumer for Read-Side Sync

```go
// internal/mq/post_sync_consumer.go
func handlePostCreated(svcCtx *svc.ServiceContext, key, value string) error {
    var event events.PostCreatedEvent
    if err := json.Unmarshal([]byte(value), &event); err != nil {
        logx.Errorf("failed to unmarshal PostCreatedEvent: %v", err)
        return nil // Skip malformed messages
    }

    // Idempotency check — skip if already processed
    processed, err := svcCtx.Redis.ExistsCtx(context.Background(), "event:processed:"+event.EventId)
    if err == nil && processed {
        return nil
    }

    // Index to Elasticsearch
    err = svcCtx.ESClient.Index(context.Background(), "posts", event.PostId, map[string]any{
        "id":         event.PostId,
        "title":      event.Title,
        "content":    event.Content,
        "user_id":    event.UserId,
        "created_at": event.Timestamp,
    })
    if err != nil {
        return fmt.Errorf("failed to index post to ES: %w", err)
    }

    // Mark event as processed (TTL 7 days)
    _ = svcCtx.Redis.SetexCtx(context.Background(), "event:processed:"+event.EventId, "1", 604800)
    return nil
}
```

---

## 3. Dead Letter Queue (DLQ)

### Why DLQ

Messages that fail processing repeatedly should be moved to a dead letter queue instead of blocking the main consumer or being silently dropped.

### DLQ with Retry Counter

```go
// internal/mq/dlq_handler.go
type RetryableConsumer struct {
    maxRetries int
    dlqPusher  *kq.Pusher
    handler    func(ctx context.Context, key, value string) error
    redis      *redis.Redis
}

func NewRetryableConsumer(maxRetries int, dlqBrokers []string, dlqTopic string,
    rds *redis.Redis, handler func(ctx context.Context, key, value string) error) *RetryableConsumer {
    return &RetryableConsumer{
        maxRetries: maxRetries,
        dlqPusher:  kq.NewPusher(dlqBrokers, dlqTopic),
        handler:    handler,
        redis:      rds,
    }
}

func (c *RetryableConsumer) Handle(key, value string) error {
    ctx := context.Background()

    // Track retry count in Redis
    retryKey := fmt.Sprintf("mq:retry:%s", key)
    count, _ := c.redis.IncrCtx(ctx, retryKey)
    if count == 1 {
        _ = c.redis.ExpireCtx(ctx, retryKey, 86400) // 24h TTL
    }

    err := c.handler(ctx, key, value)
    if err == nil {
        _ = c.redis.DelCtx(ctx, retryKey)
        return nil
    }

    if count >= int64(c.maxRetries) {
        // Move to DLQ
        dlqMessage, _ := json.Marshal(map[string]any{
            "original_key":   key,
            "original_value": value,
            "error":          err.Error(),
            "retry_count":    count,
            "failed_at":      time.Now().Format(time.RFC3339),
        })

        if dlqErr := c.dlqPusher.Push(string(dlqMessage)); dlqErr != nil {
            logx.Errorf("failed to push to DLQ: %v (original error: %v)", dlqErr, err)
        }

        _ = c.redis.DelCtx(ctx, retryKey)
        logx.Errorf("message moved to DLQ after %d retries: key=%s, error=%v", count, key, err)
        return nil // Acknowledge the original message
    }

    logx.Errorf("message processing failed (attempt %d/%d): key=%s, error=%v", count, c.maxRetries, key, err)
    return err // Trigger retry
}
```

### DLQ Configuration

```yaml
# etc/service.yaml
Kafka:
  OrderCreated:
    Brokers:
      - ${KAFKA_BROKER_1}
    Group: order-service
    Topic: order.created
    Consumers: 4
    Processors: 8

  # Dead Letter Queue
  DLQ:
    Brokers:
      - ${KAFKA_BROKER_1}
    Group: dlq-processor
    Topic: order.created.dlq
    Consumers: 1
    Processors: 1

DLQ:
  MaxRetries: 3
```

### DLQ Monitor & Alerting

```go
// internal/mq/dlq_monitor.go
func handleDLQMessage(svcCtx *svc.ServiceContext, key, value string) error {
    var dlqMsg DLQMessage
    if err := json.Unmarshal([]byte(value), &dlqMsg); err != nil {
        logx.Errorf("malformed DLQ message: %v", err)
        return nil
    }

    // Log for observability
    logx.WithFields(
        logx.Field("original_key", dlqMsg.OriginalKey),
        logx.Field("error", dlqMsg.Error),
        logx.Field("retry_count", dlqMsg.RetryCount),
        logx.Field("failed_at", dlqMsg.FailedAt),
    ).Error("DLQ message received — manual intervention may be required")

    // Store in database for admin review
    _, err := svcCtx.DLQModel.Insert(context.Background(), &model.DLQEntry{
        OriginalKey:   dlqMsg.OriginalKey,
        OriginalValue: dlqMsg.OriginalValue,
        Error:         dlqMsg.Error,
        RetryCount:    dlqMsg.RetryCount,
        Status:        "pending_review",
    })
    if err != nil {
        logx.Errorf("failed to store DLQ entry: %v", err)
    }

    // Increment Prometheus counter for alerting
    dlqMessageTotal.Inc()
    return nil
}
```

---

## 4. Transactional Outbox Pattern

### Why Outbox

The dual-write problem: writing to DB and publishing to MQ are two separate operations. If one succeeds and the other fails, data becomes inconsistent.

```
❌ Dual-Write Problem:
1. INSERT INTO orders → ✅ Success
2. Publish to Kafka    → ❌ Failure (message lost!)

✅ Outbox Pattern:
1. BEGIN TRANSACTION
   INSERT INTO orders
   INSERT INTO outbox (event data)
   COMMIT
2. Background worker reads outbox → Publishes to Kafka → Marks as published
```

### Outbox Table

```sql
CREATE TABLE outbox (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    aggregate_type VARCHAR(64) NOT NULL,   -- e.g., "order"
    aggregate_id BIGINT NOT NULL,          -- e.g., order_id
    event_type VARCHAR(64) NOT NULL,       -- e.g., "order.created"
    payload TEXT NOT NULL,                  -- JSON event data
    status TINYINT DEFAULT 0,              -- 0=pending, 1=published, 2=failed
    retry_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMP NULL,
    INDEX idx_status_created (status, created_at)
);
```

### Writing to Outbox in Transaction

```go
// internal/logic/command/create_order_logic.go
func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    var orderId int64

    err := l.svcCtx.DB.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
        // 1. Insert order
        order := &model.Order{
            Id:     l.svcCtx.Snowflake.Generate(),
            UserId: req.UserId,
            Amount: req.Amount,
            Status: "pending",
        }
        _, err := session.ExecCtx(ctx,
            "INSERT INTO orders(id, user_id, amount, status) VALUES(?, ?, ?, ?)",
            order.Id, order.UserId, order.Amount, order.Status)
        if err != nil {
            return err
        }
        orderId = order.Id

        // 2. Insert outbox event (same transaction!)
        event := events.OrderCreatedEvent{
            BaseEvent: events.BaseEvent{
                EventId:   uuid.New().String(),
                EventType: "order.created",
                Source:    "order-service",
                Timestamp: time.Now(),
            },
            OrderId: order.Id,
            UserId:  order.UserId,
            Amount:  order.Amount,
        }
        payload, err := json.Marshal(event)
        if err != nil {
            return err
        }

        _, err = session.ExecCtx(ctx,
            "INSERT INTO outbox(aggregate_type, aggregate_id, event_type, payload) VALUES(?, ?, ?, ?)",
            "order", order.Id, "order.created", string(payload))
        return err
    })

    if err != nil {
        return nil, err
    }

    return &types.CreateOrderResponse{Id: orderId}, nil
}
```

### Outbox Publisher Worker

```go
// internal/worker/outbox_publisher.go
type OutboxPublisher struct {
    db        sqlx.SqlConn
    producer  *EventProducer
    batchSize int
    interval  time.Duration
}

func NewOutboxPublisher(db sqlx.SqlConn, producer *EventProducer) *OutboxPublisher {
    return &OutboxPublisher{
        db:        db,
        producer:  producer,
        batchSize: 100,
        interval:  time.Second,
    }
}

func (w *OutboxPublisher) Start(ctx context.Context) {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.processBatch(ctx)
        }
    }
}

func (w *OutboxPublisher) processBatch(ctx context.Context) {
    // Fetch pending events
    var entries []OutboxEntry
    query := `SELECT id, event_type, payload FROM outbox
              WHERE status = 0 AND retry_count < 5
              ORDER BY created_at ASC LIMIT ?`
    err := w.db.QueryRowsCtx(ctx, &entries, query, w.batchSize)
    if err != nil {
        logx.Errorf("failed to fetch outbox entries: %v", err)
        return
    }

    for _, entry := range entries {
        // Publish to message queue
        err := w.producer.PublishRaw(ctx, entry.EventType, entry.Payload)
        if err != nil {
            // Mark as retry
            _, _ = w.db.ExecCtx(ctx,
                "UPDATE outbox SET retry_count = retry_count + 1, status = CASE WHEN retry_count >= 4 THEN 2 ELSE 0 END WHERE id = ?",
                entry.Id)
            logx.Errorf("failed to publish outbox entry %d: %v", entry.Id, err)
            continue
        }

        // Mark as published
        _, err = w.db.ExecCtx(ctx,
            "UPDATE outbox SET status = 1, published_at = NOW() WHERE id = ?",
            entry.Id)
        if err != nil {
            logx.Errorf("failed to mark outbox entry %d as published: %v", entry.Id, err)
        }
    }
}
```

### Outbox Cleanup Job

```go
// Clean up published entries older than 7 days
func (w *OutboxPublisher) Cleanup(ctx context.Context) {
    _, err := w.db.ExecCtx(ctx,
        "DELETE FROM outbox WHERE status = 1 AND published_at < DATE_SUB(NOW(), INTERVAL 7 DAY)")
    if err != nil {
        logx.Errorf("failed to cleanup outbox: %v", err)
    }
}
```

---

## 5. Event Sourcing Lite

For services that need an event log without full event sourcing:

### Event Store

```sql
CREATE TABLE event_store (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSON NOT NULL,
    version INT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_aggregate_version (aggregate_type, aggregate_id, version),
    INDEX idx_aggregate (aggregate_type, aggregate_id)
);
```

### Append Event with Optimistic Concurrency

```go
func (m *EventStoreModel) AppendEvent(ctx context.Context, aggregateType string, aggregateId int64,
    eventType string, payload string, expectedVersion int) error {
    _, err := m.conn.ExecCtx(ctx,
        `INSERT INTO event_store(aggregate_type, aggregate_id, event_type, payload, version)
         VALUES(?, ?, ?, ?, ?)`,
        aggregateType, aggregateId, eventType, payload, expectedVersion+1)
    if err != nil {
        // Duplicate key = version conflict (concurrent write detected)
        if strings.Contains(err.Error(), "Duplicate entry") {
            return ErrConcurrentModification
        }
        return err
    }
    return nil
}
```

---

## Decision Matrix

| Scenario | Pattern | Why |
|----------|---------|-----|
| Simple notifications | Direct Kafka publish | Low complexity, eventual consistency OK |
| DB + MQ consistency | Transactional Outbox | Guarantees both succeed or neither |
| Search/analytics separate from writes | CQRS | Different read/write optimization |
| Failed message handling | DLQ + retry | Prevents message loss and blocking |
| Audit trail needed | Event Store | Immutable history of all changes |
| Cross-service transactions | Saga + events | See [Concurrency Patterns](./concurrency-patterns.md#6-distributed-transactions--saga-pattern) |

---

## Checklist

### ✅ DO:
- Define event schemas with unique event IDs
- Include trace IDs in events for distributed tracing
- Use idempotent consumers (check event_id before processing)
- Implement DLQ for failed messages
- Use Transactional Outbox for DB+MQ consistency
- Clean up published outbox entries periodically
- Set appropriate consumer/processor counts for load
- Use persistent message delivery mode

### ❌ DON'T:
- Publish events directly in business logic without outbox (dual-write risk)
- Retry malformed messages (skip them, log to DLQ)
- Use synchronous RPC where async events are more appropriate
- Ignore consumer lag — monitor it with Prometheus
- Store large payloads in events (use references/IDs instead)

---

## Cross-References

- For Saga distributed transactions, see [Concurrency Patterns](./concurrency-patterns.md#6-distributed-transactions--saga-pattern)
- For idempotency on consumers, see [Concurrency Patterns](./concurrency-patterns.md#5-idempotency)
- For tracing across async boundaries, see [Observability Patterns](./observability-patterns.md)
- For rate limiting producers, see [Resilience Patterns](./resilience-patterns.md)

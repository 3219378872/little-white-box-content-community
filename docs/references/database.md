# Database Patterns

## Table of Contents
- [SQL Database with go-zero](#sql-database-with-gozero)
- [Basic SQL Operations Pattern](#basic-sql-operations-pattern)
- [CRUD Operations Pattern](#crud-operations-pattern)
- [Custom Query Pattern](#custom-query-pattern)
- [sql.Null* 类型与动态 SQL 模式](#sqlnull-类型与动态-sql-模式)
- [Transaction Pattern](#transaction-pattern)
- [Caching Pattern](#caching-pattern)
- [Connection Pooling Pattern](#connection-pooling-pattern)
- [Error Handling Pattern](#error-handling-pattern)
- [MongoDB Pattern](#mongodb-pattern)
- [Best Practices Summary](#best-practices-summary)
- [When to Use Each Database Type](#when-to-use-each-database-type)

## SQL Database with go-zero

go-zero provides `sqlx` and `sqlc` packages for SQL operations with built-in connection pooling, caching, and resilience.

## Basic SQL Operations Pattern

### ✅ Model Generation from SQL

```bash
# Generate model from existing database
goctl model mysql datasource \
  -url="user:pass@tcp(localhost:3306)/database" \
  -table="users" \
  -dir="./model"

# Generate model from SQL DDL file
goctl model mysql ddl \
  -src="./schema.sql" \
  -dir="./model"
```

### Example SQL Schema

```sql
CREATE TABLE `users` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `email` varchar(255) NOT NULL UNIQUE,
  `age` int NOT NULL DEFAULT 0,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### Generated Model Structure

```go
// model/usersmodel.go
package model

import (
    "context"
    "database/sql"
    "github.com/zeromicro/go-zero/core/stores/cache"
    "github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ UsersModel = (*customUsersModel)(nil)

type (
    // Interface for Users model operations
    UsersModel interface {
        usersModel
        // Add custom methods here
    }

    customUsersModel struct {
        *defaultUsersModel
    }

    // Generated struct
    Users struct {
        Id        int64          `db:"id"`
        Name      string         `db:"name"`
        Email     string         `db:"email"`
        Age       int64          `db:"age"`
        CreatedAt sql.NullTime   `db:"created_at"`
        UpdatedAt sql.NullTime   `db:"updated_at"`
    }
)

// NewUsersModel returns a model for Users
func NewUsersModel(conn sqlx.SqlConn, c cache.CacheConf) UsersModel {
    return &customUsersModel{
        defaultUsersModel: newUsersModel(conn, c),
    }
}

// Generated methods (in usersmodel_gen.go):
// - Insert(ctx context.Context, data *Users) (sql.Result, error)
// - FindOne(ctx context.Context, id int64) (*Users, error)
// - FindOneByEmail(ctx context.Context, email string) (*Users, error)
// - Update(ctx context.Context, data *Users) error
// - Delete(ctx context.Context, id int64) error
```

## CRUD Operations Pattern

### ✅ Insert

```go
func (l *CreateUserLogic) CreateUser(req *types.CreateUserRequest) (*types.CreateUserResponse, error) {
    user := &model.Users{
        Name:  req.Name,
        Email: req.Email,
        Age:   int64(req.Age),
    }

    result, err := l.svcCtx.UsersModel.Insert(l.ctx, user)
    if err != nil {
        l.Logger.Errorf("failed to insert user: %v", err)
        return nil, err
    }

    userId, err := result.LastInsertId()
    if err != nil {
        return nil, err
    }

    return &types.CreateUserResponse{
        Id: userId,
    }, nil
}
```

### ✅ Find by Primary Key

```go
func (l *GetUserLogic) GetUser(req *types.GetUserRequest) (*types.GetUserResponse, error) {
    user, err := l.svcCtx.UsersModel.FindOne(l.ctx, req.Id)
    if err != nil {
        if errors.Is(err, model.ErrNotFound) {
            return nil, errors.New("user not found")
        }
        return nil, err
    }

    return &types.GetUserResponse{
        Id:    user.Id,
        Name:  user.Name,
        Email: user.Email,
        Age:   int(user.Age),
    }, nil
}
```

### ✅ Find by Unique Index

```go
func (l *GetUserByEmailLogic) GetUserByEmail(email string) (*model.Users, error) {
    user, err := l.svcCtx.UsersModel.FindOneByEmail(l.ctx, email)
    if err != nil {
        if errors.Is(err, model.ErrNotFound) {
            return nil, errors.New("user not found")
        }
        return nil, err
    }
    return user, nil
}
```

### ✅ Update (Direct SQL — Concurrency Safe)

Use direct SQL `UPDATE ... SET` to avoid read-modify-write race conditions:

```go
func (l *UpdateUserLogic) UpdateUser(req *types.UpdateUserRequest) error {
    query := `UPDATE users SET name = ?, age = ?, updated_at = NOW() WHERE id = ?`
    result, err := l.svcCtx.DB.ExecCtx(l.ctx, query, req.Name, req.Age, req.Id)
    if err != nil {
        l.Logger.Errorf("failed to update user: %v", err)
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return errors.New("user not found")
    }

    return nil
}
```

### ✅ Update with Optimistic Locking (High-Concurrency Safe)

Use a `version` field to detect concurrent modifications:

```go
func (l *UpdateUserLogic) UpdateUser(req *types.UpdateUserRequest) error {
    query := `UPDATE users SET name = ?, age = ?, version = version + 1, updated_at = NOW()
              WHERE id = ? AND version = ?`
    result, err := l.svcCtx.DB.ExecCtx(l.ctx, query, req.Name, req.Age, req.Id, req.Version)
    if err != nil {
        l.Logger.Errorf("failed to update user: %v", err)
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return errors.New("concurrent modification detected, please retry")
    }

    return nil
}
```

### ❌ Anti-Pattern: Read-Modify-Write Without Locking

> **WARNING**: The pattern below causes **Lost Update** under concurrent access.
> Two requests may read the same row, modify different fields, and overwrite each other.

```go
// ❌ DO NOT USE in concurrent scenarios
func (l *UpdateUserLogic) UpdateUser(req *types.UpdateUserRequest) error {
    user, err := l.svcCtx.UsersModel.FindOne(l.ctx, req.Id)  // T1 reads
    if err != nil {
        return err
    }
    user.Name = req.Name                                        // T1 modifies in memory
    err = l.svcCtx.UsersModel.Update(l.ctx, user)              // T1 writes back — may overwrite T2's changes
    if err != nil {
        return err
    }
    return nil
}
// If you must read-then-modify, use pessimistic locking (SELECT ... FOR UPDATE) within a transaction.
```

### ✅ Delete

```go
func (l *DeleteUserLogic) DeleteUser(req *types.DeleteUserRequest) error {
    err := l.svcCtx.UsersModel.Delete(l.ctx, req.Id)
    if err != nil {
        l.Logger.Errorf("failed to delete user: %v", err)
        return err
    }
    return nil
}
```

## Custom Query Pattern

### ✅ Add Custom Methods to Model

```go
// model/usersmodel.go
type (
    UsersModel interface {
        usersModel
        // Custom methods
        FindByAgeRange(ctx context.Context, minAge, maxAge int64) ([]*Users, error)
        FindActiveUsers(ctx context.Context, limit int64) ([]*Users, error)
        CountByAge(ctx context.Context, age int64) (int64, error)
    }

    customUsersModel struct {
        *defaultUsersModel
    }
)

func (m *customUsersModel) FindByAgeRange(ctx context.Context, minAge, maxAge int64) ([]*Users, error) {
    query := `SELECT * FROM users WHERE age BETWEEN ? AND ? ORDER BY created_at DESC`
    var users []*Users
    err := m.QueryRowsNoCacheCtx(ctx, &users, query, minAge, maxAge)
    if err != nil {
        return nil, err
    }
    return users, nil
}

func (m *customUsersModel) FindActiveUsers(ctx context.Context, limit int64) ([]*Users, error) {
    query := `SELECT * FROM users WHERE updated_at > DATE_SUB(NOW(), INTERVAL 30 DAY) LIMIT ?`
    var users []*Users
    err := m.QueryRowsNoCacheCtx(ctx, &users, query, limit)
    if err != nil {
        return nil, err
    }
    return users, nil
}

func (m *customUsersModel) CountByAge(ctx context.Context, age int64) (int64, error) {
    query := `SELECT COUNT(*) FROM users WHERE age = ?`
    var count int64
    err := m.QueryRowNoCacheCtx(ctx, &count, query, age)
    return count, err
}
```

### ✅ Pagination Pattern

```go
func (m *customUsersModel) FindWithPagination(ctx context.Context, page, pageSize int64) ([]*Users, int64, error) {
    // Get total count
    var total int64
    countQuery := `SELECT COUNT(*) FROM users`
    err := m.QueryRowNoCacheCtx(ctx, &total, countQuery)
    if err != nil {
        return nil, 0, err
    }

    // Get paginated results
    offset := (page - 1) * pageSize
    query := `SELECT * FROM users ORDER BY id DESC LIMIT ? OFFSET ?`
    var users []*Users
    err = m.QueryRowsNoCacheCtx(ctx, &users, query, pageSize, offset)
    if err != nil {
        return nil, 0, err
    }

    return users, total, nil
}
```

## sql.Null* 类型与动态 SQL 模式

数据库中的 `NULL` 字段在 Go 中映射为 `sql.NullString`、`sql.NullInt64`、`sql.NullTime` 等类型。goctl 生成的 model struct 会自动使用这些类型。正确处理它们是避免空值 bug 的关键。

### sql.Null* 类型基础

```go
// sql.NullString 结构
type NullString struct {
    String string // 实际值
    Valid  bool   // true = 有值, false = NULL
}
```

### ✅ 正确构造 sql.Null*

```go
// 有值
images := sql.NullString{String: "img1.jpg,img2.jpg", Valid: true}
parentId := sql.NullInt64{Int64: 10, Valid: true}
birthday := sql.NullTime{Time: time.Now(), Valid: true}

// NULL（无值）
images := sql.NullString{Valid: false}
parentId := sql.NullInt64{Valid: false}
birthday := sql.NullTime{Valid: false}
```

### ✅ 正确读取 sql.Null*

```go
// 必须先检查 Valid
if user.Phone.Valid {
    phone = user.Phone.String
} else {
    phone = "" // 或其他默认值
}
```

### ❌ 反模式：不检查 Valid 直接访问

```go
// ❌ 当 Valid=false 时，String 为零值 ""，无法区分「NULL」和「空字符串」
phone := user.Phone.String
```

### ✅ 辅助转换函数

将这些函数放在 logic 层的公共文件（如 `convert.go`）或 util 包中，统一转换逻辑：

```go
// toNullString 空字符串映射为 NULL，非空映射为有效值
func toNullString(s string) sql.NullString {
    return sql.NullString{String: s, Valid: s != ""}
}

// fromNullString NULL 映射为空字符串
func fromNullString(ns sql.NullString) string {
    if ns.Valid {
        return ns.String
    }
    return ""
}

// toNullInt64 根据 hasValue 决定是否为 NULL
func toNullInt64(v int64, hasValue bool) sql.NullInt64 {
    return sql.NullInt64{Int64: v, Valid: hasValue}
}

// fromNullInt64 NULL 映射为 0
func fromNullInt64(ni sql.NullInt64) int64 {
    if ni.Valid {
        return ni.Int64
    }
    return 0
}
```

### ✅ gRPC/API 响应转换模式

```go
// sql.NullString → proto string（空值返回 ""）
resp.Phone = fromNullString(user.Phone)

// sql.NullString（逗号分隔）→ proto repeated string
func nullStringToSlice(ns sql.NullString) []string {
    if !ns.Valid || ns.String == "" {
        return []string{}
    }
    return strings.Split(ns.String, ",")
}
resp.Images = nullStringToSlice(post.Images)

// proto repeated string → sql.NullString（写入时合并）
func sliceToNullString(ss []string) sql.NullString {
    return sql.NullString{
        String: strings.Join(ss, ","),
        Valid:  len(ss) > 0,
    }
}
images := sliceToNullString(in.Images)
```

### ✅ 动态 SQL 更新模式（PATCH 风格）

当只需更新用户传入的字段时，使用 `strings.Builder` 动态拼接 SET 子句，避免为每种字段组合硬编码 SQL：

```go
// model 层：UpdatePostPartial 只更新非零值字段
func (m *customPostModel) UpdatePostPartial(ctx context.Context, id int64, fields map[string]interface{}) error {
    if len(fields) == 0 {
        return nil // 无需更新
    }

    var setClauses []string
    var args []interface{}
    for col, val := range fields {
        setClauses = append(setClauses, fmt.Sprintf("`%s`=?", col))
        args = append(args, val)
    }
    args = append(args, id)

    postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
    _, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
        query := fmt.Sprintf("update %s set %s where `id`=?", m.table, strings.Join(setClauses, ", "))
        return conn.ExecCtx(ctx, query, args...)
    }, postIdKey)
    return err
}
```

Logic 层调用方式：

```go
func (l *UpdatePostLogic) UpdatePost(in *pb.UpdatePostReq) (*pb.UpdatePostResp, error) {
    fields := make(map[string]interface{})

    if in.Title != "" {
        fields["title"] = in.Title
    }
    if in.Content != "" {
        fields["content"] = in.Content
    }
    // sql.NullString 字段：根据业务语义决定是否更新
    if len(in.Images) > 0 {
        fields["images"] = sql.NullString{String: strings.Join(in.Images, ","), Valid: true}
    }
    if in.Status > 0 {
        fields["status"] = in.Status
    }

    if err := l.svcCtx.PostModel.UpdatePostPartial(l.ctx, in.PostId, fields); err != nil {
        return nil, fmt.Errorf("更新帖子失败: %w", err)
    }
    return &pb.UpdatePostResp{}, nil
}
```

### ✅ 动态 SQL 更新模式（强类型版本）

如果不想用 `map[string]interface{}`，可以用结构体 + 指针字段实现类型安全的 PATCH：

```go
// 更新请求结构体，nil 表示不更新该字段
type PostUpdateFields struct {
    Title   *string
    Content *string
    Images  *sql.NullString
    Status  *int64
}

func (m *customPostModel) UpdatePostFields(ctx context.Context, id int64, f PostUpdateFields) error {
    var setClauses []string
    var args []interface{}

    if f.Title != nil {
        setClauses = append(setClauses, "`title`=?")
        args = append(args, *f.Title)
    }
    if f.Content != nil {
        setClauses = append(setClauses, "`content`=?")
        args = append(args, *f.Content)
    }
    if f.Images != nil {
        setClauses = append(setClauses, "`images`=?")
        args = append(args, *f.Images)
    }
    if f.Status != nil {
        setClauses = append(setClauses, "`status`=?")
        args = append(args, *f.Status)
    }

    if len(setClauses) == 0 {
        return nil
    }
    args = append(args, id)

    postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
    _, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
        query := fmt.Sprintf("update %s set %s where `id`=?", m.table, strings.Join(setClauses, ", "))
        return conn.ExecCtx(ctx, query, args...)
    }, postIdKey)
    return err
}
```

### ✅ 动态 SQL 查询模式（条件过滤）

列表查询中，多个筛选条件可能为空，使用动态 WHERE 子句：

```go
func (m *customPostModel) FindPostsWithFilters(
    ctx context.Context,
    authorId *int64,
    status *int64,
    keyword string,
    page, pageSize int,
) ([]*Post, int64, error) {
    var conditions []string
    var args []interface{}

    if authorId != nil {
        conditions = append(conditions, "`author_id`=?")
        args = append(args, *authorId)
    }
    if status != nil {
        conditions = append(conditions, "`status`=?")
        args = append(args, *status)
    }
    if keyword != "" {
        conditions = append(conditions, "(`title` LIKE ? OR `content` LIKE ?)")
        like := "%" + keyword + "%"
        args = append(args, like, like)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = "WHERE " + strings.Join(conditions, " AND ")
    }

    // 总数
    var total int64
    countQuery := fmt.Sprintf("select count(*) from %s %s", m.table, whereClause)
    err := m.QueryRowNoCacheCtx(ctx, &total, countQuery, args...)
    if err != nil {
        return nil, 0, err
    }

    // 分页数据
    offset := (page - 1) * pageSize
    dataArgs := append(args, offset, pageSize)
    query := fmt.Sprintf("select %s from %s %s order by `created_at` desc limit ?,?", postRows, m.table, whereClause)
    var posts []*Post
    err = m.QueryRowsNoCacheCtx(ctx, &posts, query, dataArgs...)
    if err != nil {
        return nil, 0, err
    }

    return posts, total, nil
}
```

### ❌ 动态 SQL 反模式

```go
// ❌ 为每种字段组合手写一条 SQL
func UpdateTitle(ctx, id, title)   { exec("update posts set title=? ...") }
func UpdateContent(ctx, id, content) { exec("update posts set content=? ...") }
func UpdateTitleAndContent(ctx, id, title, content) { exec("update posts set title=?, content=? ...") }
// 字段组合爆炸，不可维护

// ❌ 拼接 SQL 时不用参数化（SQL 注入风险）
query := fmt.Sprintf("update posts set title='%s' where id=%d", title, id)

// ❌ 用 sql.NullString 但传给 WHERE 子句做等值比较时忽略 NULL 语义
// WHERE phone = ? 当 phone 为 sql.NullString{Valid: false} 时
// 数据库收到 NULL，但 NULL = NULL 在 SQL 中是 false！
// ✅ 应使用 WHERE phone IS NULL
```

## Transaction Pattern

### Transaction Isolation Levels

MySQL default is `REPEATABLE READ`. Choose based on your use case:

| Level | Dirty Read | Non-Repeatable Read | Phantom Read | Use Case |
|-------|-----------|-------------------|-------------|----------|
| READ UNCOMMITTED | Yes | Yes | Yes | Never in production |
| READ COMMITTED | No | Yes | Yes | Most CRUD operations |
| REPEATABLE READ | No | No | Yes (InnoDB prevents) | MySQL default, good for most cases |
| SERIALIZABLE | No | No | No | Financial calculations (slow) |

```go
// Set isolation level per transaction (when needed)
_, err := session.ExecCtx(ctx, "SET TRANSACTION ISOLATION LEVEL READ COMMITTED")
```

> **Note**: For concurrent write safety, see [Concurrency Patterns](./concurrency-patterns.md) — isolation levels alone do NOT prevent Lost Update in read-modify-write scenarios.

### ✅ Simple Transaction

```go
func (l *TransferLogic) Transfer(from, to int64, amount float64) error {
    // Start transaction
    err := l.svcCtx.DB.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
        // Debit from account
        debitQuery := `UPDATE accounts SET balance = balance - ? WHERE id = ? AND balance >= ?`
        result, err := session.ExecCtx(ctx, debitQuery, amount, from, amount)
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

        // Credit to account
        creditQuery := `UPDATE accounts SET balance = balance + ? WHERE id = ?`
        _, err = session.ExecCtx(ctx, creditQuery, amount, to)
        if err != nil {
            return err
        }

        // Record transaction
        recordQuery := `INSERT INTO transactions(from_id, to_id, amount) VALUES(?, ?, ?)`
        _, err = session.ExecCtx(ctx, recordQuery, from, to, amount)
        return err
    })

    return err
}
```

### ✅ Complex Transaction with Multiple Models

```go
func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    var orderId int64

    err := l.svcCtx.DB.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
        // 1. Create order
        orderQuery := `INSERT INTO orders(user_id, total_amount, status) VALUES(?, ?, ?)`
        result, err := session.ExecCtx(ctx, orderQuery, req.UserId, req.TotalAmount, "pending")
        if err != nil {
            return fmt.Errorf("failed to create order: %w", err)
        }

        orderId, err = result.LastInsertId()
        if err != nil {
            return err
        }

        // 2. Create order items
        itemQuery := `INSERT INTO order_items(order_id, product_id, quantity, price) VALUES(?, ?, ?, ?)`
        for _, item := range req.Items {
            _, err = session.ExecCtx(ctx, itemQuery, orderId, item.ProductId, item.Quantity, item.Price)
            if err != nil {
                return fmt.Errorf("failed to create order item: %w", err)
            }
        }

        // 3. Update inventory
        inventoryQuery := `UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?`
        for _, item := range req.Items {
            result, err = session.ExecCtx(ctx, inventoryQuery, item.Quantity, item.ProductId, item.Quantity)
            if err != nil {
                return fmt.Errorf("failed to update inventory: %w", err)
            }

            affected, err := result.RowsAffected()
            if err != nil {
                return fmt.Errorf("failed to check rows affected: %w", err)
            }
            if affected == 0 {
                return fmt.Errorf("insufficient stock for product %d", item.ProductId)
            }
        }

        return nil
    })

    if err != nil {
        l.Logger.Errorf("transaction failed: %v", err)
        return nil, err
    }

    return &types.CreateOrderResponse{
        OrderId: orderId,
    }, nil
}
```

## Caching Pattern

### ✅ Cache Configuration

```yaml
# Configuration file
Cache:
  - Host: localhost:6379
    Type: node
    Pass: ""  # Redis password (optional)
  # For Redis cluster
  # - Host: localhost:6379,localhost:6380,localhost:6381
  #   Type: cluster
```

```go
// Configuration struct
type Config struct {
    rest.RestConf
    DataSource string
    Cache      cache.CacheConf
}
```

### ✅ Model with Cache

When you use `NewUsersModel(conn, c.Cache)`, caching is automatic for:
- `FindOne` - Cached by primary key
- `FindOneByXxx` - Cached by unique index
- `Update` / `Delete` - Automatically invalidates cache

```go
// Service context with cache
func NewServiceContext(c config.Config) *ServiceContext {
    conn := sqlx.NewMysql(c.DataSource)

    return &ServiceContext{
        Config:     c,
        UsersModel: model.NewUsersModel(conn, c.Cache),  // ✅ Cache enabled
    }
}
```

### ✅ Custom Cache Keys

```go
func (m *customUsersModel) FindByEmailWithCache(ctx context.Context, email string) (*Users, error) {
    // Custom cache key
    cacheKey := fmt.Sprintf("user:email:%s", email)

    var user Users
    err := m.QueryRowCtx(ctx, &user, cacheKey, func(ctx context.Context, conn sqlx.SqlConn, v interface{}) error {
        query := `SELECT * FROM users WHERE email = ? LIMIT 1`
        return conn.QueryRowCtx(ctx, v, query, email)
    })

    if err != nil {
        return nil, err
    }

    return &user, nil
}
```

### ✅ Manual Cache Operations

```go
// Get from cache
var user Users
err := m.CachedConn.GetCacheCtx(ctx, "user:123", &user)

// Set cache with expiration
err := m.CachedConn.SetCacheCtx(ctx, "user:123", user, time.Hour)

// Delete cache
err := m.CachedConn.DelCacheCtx(ctx, "user:123")

// Delete multiple cache keys
err := m.CachedConn.DelCacheCtx(ctx, "user:123", "user:email:test@test.com")
```

## Connection Pooling Pattern

### ✅ Default Pool Configuration

go-zero uses sensible defaults:
```go
// Default connection pool settings
MaxIdleConns: 64
MaxOpenConns: 64
ConnMaxLifetime: time.Minute
```

### ✅ Custom Pool Configuration

```go
func NewServiceContext(c config.Config) *ServiceContext {
    // Create connection with custom settings
    conn := sqlx.NewMysql(c.DataSource)

    // Customize pool (if needed)
    db, err := conn.RawDB()
    if err == nil {
        db.SetMaxIdleConns(100)
        db.SetMaxOpenConns(100)
        db.SetConnMaxLifetime(time.Minute * 5)
    }

    return &ServiceContext{
        Config:     c,
        UsersModel: model.NewUsersModel(conn, c.Cache),
    }
}
```

## Error Handling Pattern

### ✅ Handle Common Errors

```go
import (
    "github.com/zeromicro/go-zero/core/stores/sqlc"
)

func (l *GetUserLogic) GetUser(req *types.GetUserRequest) (*types.GetUserResponse, error) {
    user, err := l.svcCtx.UsersModel.FindOne(l.ctx, req.Id)
    if err != nil {
        // Check for not found
        if errors.Is(err, sqlc.ErrNotFound) {
            return nil, errors.New("user not found")
        }

        // Check for database errors
        if errors.Is(err, sql.ErrConnDone) {
            l.Logger.Error("database connection error")
            return nil, errors.New("database connection error")
        }

        // Generic error
        l.Logger.Errorf("failed to find user: %v", err)
        return nil, err
    }

    return &types.GetUserResponse{
        Id:    user.Id,
        Name:  user.Name,
        Email: user.Email,
    }, nil
}
```

### ✅ Handle Duplicate Key Errors

```go
import (
    "github.com/go-sql-driver/mysql"
)

func (l *CreateUserLogic) CreateUser(req *types.CreateUserRequest) (*types.CreateUserResponse, error) {
    user := &model.Users{
        Name:  req.Name,
        Email: req.Email,
    }

    result, err := l.svcCtx.UsersModel.Insert(l.ctx, user)
    if err != nil {
        // Check for duplicate key error
        if mysqlErr, ok := err.(*mysql.MySQLError); ok {
            if mysqlErr.Number == 1062 { // Duplicate entry
                return nil, errors.New("email already exists")
            }
        }
        return nil, err
    }

    userId, err := result.LastInsertId()
    if err != nil {
        return nil, err
    }
    return &types.CreateUserResponse{Id: userId}, nil
}
```

## MongoDB Pattern

### ✅ MongoDB Configuration

```yaml
Mongo:
  Host: localhost:27017
  Type: mongo  # or "mongos" for sharded cluster
  User: username
  Pass: password
  Db: mydb
```

```go
type Config struct {
    rest.RestConf
    Mongo struct {
        Host string
        Type string
        User string `json:",optional"`
        Pass string `json:",optional"`
        Db   string
    }
}
```

### ✅ MongoDB Model

```go
// model/usermodel.go
package model

import (
    "context"
    "github.com/zeromicro/go-zero/core/stores/mon"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
    ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
    Name     string             `bson:"name" json:"name"`
    Email    string             `bson:"email" json:"email"`
    Age      int                `bson:"age" json:"age"`
    CreateAt int64              `bson:"create_at" json:"create_at"`
    UpdateAt int64              `bson:"update_at" json:"update_at"`
}

type UserModel interface {
    Insert(ctx context.Context, user *User) error
    FindOne(ctx context.Context, id string) (*User, error)
    FindOneByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
}

type defaultUserModel struct {
    conn *mon.Model
}

func NewUserModel(url, db, collection string) UserModel {
    return &defaultUserModel{
        conn: mon.MustNewModel(url, db, collection),
    }
}

func (m *defaultUserModel) Insert(ctx context.Context, user *User) error {
    user.ID = primitive.NewObjectID()
    _, err := m.conn.InsertOne(ctx, user)
    return err
}

func (m *defaultUserModel) FindOne(ctx context.Context, id string) (*User, error) {
    oid, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var user User
    err = m.conn.FindOne(ctx, &user, bson.M{"_id": oid})
    return &user, err
}

func (m *defaultUserModel) FindOneByEmail(ctx context.Context, email string) (*User, error) {
    var user User
    err := m.conn.FindOne(ctx, &user, bson.M{"email": email})
    return &user, err
}

func (m *defaultUserModel) Update(ctx context.Context, user *User) error {
    oid, err := primitive.ObjectIDFromHex(user.ID.Hex())
    if err != nil {
        return err
    }

    _, err = m.conn.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": user})
    return err
}

func (m *defaultUserModel) Delete(ctx context.Context, id string) error {
    oid, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = m.conn.DeleteOne(ctx, bson.M{"_id": oid})
    return err
}
```

## Best Practices Summary

### ✅ DO:
- Use `goctl` to generate models from database schema
- Always pass `context.Context` to database operations
- Use transactions for operations that must be atomic
- Enable caching for read-heavy models
- Handle `sqlc.ErrNotFound` explicitly
- Use connection pooling (automatic by default)
- Add custom methods to model interface
- Log database errors with context
- Use parameterized queries (automatic with go-zero)
- Validate data before database operations
- Use optimistic locking (`version` field) for concurrent updates
- Use `SELECT ... FOR UPDATE` for high-contention writes within transactions
- Use direct SQL `UPDATE ... SET` instead of read-modify-write when possible
- Always check `RowsAffected()` error — never discard with `_`
- Implement idempotency keys for critical write operations (payments, orders)
- 使用辅助函数统一处理 `sql.Null*` 类型转换（`toNullString`/`fromNullString`）
- 用 `strings.Builder` / `[]string` 动态拼接 SET/WHERE 子句，而非为每种字段组合硬编码 SQL

### ❌ DON'T:
- Execute raw SQL without parameterization
- Ignore errors from database operations
- Use `_` to discard errors (including `LastInsertId()` and `RowsAffected()`)
- Create database connections in handlers/logic
- Keep transactions open longer than necessary
- Query in loops (use batch operations)
- Store sensitive data unencrypted
- Use `SELECT *` in production code (be explicit)
- Cache write-heavy data unnecessarily
- Forget to close result sets/cursors
- Use read-modify-write (FindOne → modify → Update) without locking — causes Lost Update
- Retry non-idempotent operations without deduplication
- 直接访问 `sql.NullString.String` 而不检查 `.Valid`（空值和零值混淆）
- 在 WHERE 子句中用 `= ?` 比较 `sql.Null*`（NULL = NULL 在 SQL 中为 false，应用 `IS NULL`）

## When to Use Each Database Type

### MySQL/PostgreSQL (SQL):
- Structured data with relationships
- ACID transactions required
- Complex queries with JOINs
- Strong consistency needed
- Traditional CRUD operations

### MongoDB:
- Flexible schema
- Horizontal scaling
- Document-oriented data
- High write throughput
- Hierarchical data

### Redis (Cache):
- Session storage
- Rate limiting
- Real-time leaderboards
- Pub/sub messaging
- Hot data caching

For Redis-specific patterns, see [Resilience Patterns](./resilience-patterns.md).

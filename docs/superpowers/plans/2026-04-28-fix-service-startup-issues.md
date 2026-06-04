# Fix Service Startup Issues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 4 config/code issues preventing Message RPC and Feed RPC from starting, plus 2 cosmetic warnings.

**Architecture:** Four independent single-file fixes. No new dependencies, no proto changes, no test changes.

**Tech Stack:** go-zero v1.10.1, Go 1.26.1

---

### Task 1: Add `conf.UseEnv()` to Message RPC

**Files:**
- Modify: `app/message/rpc/message.go:30`

- [ ] **Step 1: Add `conf.UseEnv()` to `conf.MustLoad`**

```go
// Line 30: change
conf.MustLoad(*configFile, &c)
// to
conf.MustLoad(*configFile, &c, conf.UseEnv())
```

- [ ] **Step 2: Verify build**

Run: `go build ./app/message/rpc/`
Expected: exit 0, no output

- [ ] **Step 3: Commit**

```bash
git add app/message/rpc/message.go
git commit -m "fix(message): add conf.UseEnv() to load MQ and DB env vars"
```

---

### Task 2: Change Feed RPC port from 9091 to 9093

**Files:**
- Modify: `app/feed/rpc/etc/feed.yaml:2`

- [ ] **Step 1: Change port**

```yaml
# Line 2: change
ListenOn: 0.0.0.0:9091
# to
ListenOn: 0.0.0.0:9093
```

- [ ] **Step 2: Verify build**

Run: `go build ./app/feed/rpc/`
Expected: exit 0, no output

- [ ] **Step 3: Commit**

```bash
git add app/feed/rpc/etc/feed.yaml
git commit -m "fix(feed): change port 9091 to 9093 to avoid Milvus conflict"
```

---

### Task 3: Add `DB_MESSAGE` env var

**Files:**
- Modify: `scripts/env.sh`

- [ ] **Step 1: Add export line after existing DB_FEED line**

```bash
# After line 6: export DB_FEED='...'
export DB_MESSAGE='root:Xbh@MySQL2024!@tcp(127.0.0.1:3306)/xbh_message?parseTime=true&loc=UTC'
```

- [ ] **Step 2: Verify shell syntax**

Run: `bash -n scripts/env.sh`
Expected: exit 0, no output

- [ ] **Step 3: Commit**

```bash
git add scripts/env.sh
git commit -m "fix(scripts): add missing DB_MESSAGE env var"
```

---

### Task 4: Add log level to Content RPC config

**Files:**
- Modify: `app/content/rpc/etc/content.yaml`

- [ ] **Step 1: Add Log section**

```yaml
# Append after line 20:
Log:
  Level: info
```

- [ ] **Step 2: Verify build** (config-only change, no code impact)

Run: `go build ./app/content/rpc/`
Expected: exit 0, no output

- [ ] **Step 3: Commit**

```bash
git add app/content/rpc/etc/content.yaml
git commit -m "fix(content): set explicit log level to silence warning"
```

---

## Verification

After all 4 tasks:

```bash
source scripts/env.sh
export DB_MESSAGE='root:Xbh@MySQL2024!@tcp(127.0.0.1:3306)/xbh_message?parseTime=true&loc=UTC'

# Build all
go build ./...
cd app/gateway && go build . && cd -
cd app/user/rpc && go build . && cd -

# Tests
go test ./... -race -count=1
```

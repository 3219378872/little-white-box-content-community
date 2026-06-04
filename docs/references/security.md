# Security Patterns for go-zero

## Table of Contents
- [Overview](#overview)
- [1. mTLS — Service-to-Service Encryption](#1-mtls--service-to-service-encryption)
- [2. RBAC — Role-Based Access Control](#2-rbac--role-based-access-control)
- [3. OAuth 2.0 / OIDC Integration](#3-oauth-20--oidc-integration)
- [4. CORS Configuration](#4-cors-configuration)
- [5. Security Response Headers](#5-security-response-headers)
- [6. Secret Management & Rotation](#6-secret-management--rotation)
- [7. Audit Logging](#7-audit-logging)
- [8. Input Validation & Sanitization](#8-input-validation--sanitization)
- [Security Checklist](#security-checklist)
- [Cross-References](#cross-references)

## Overview

This guide covers enterprise-level security patterns for go-zero microservices:
- mTLS for service-to-service encryption
- RBAC middleware for access control
- OAuth 2.0 / OIDC integration
- CORS configuration
- Security response headers
- Secret rotation with Vault
- Audit logging

---

## 1. mTLS — Service-to-Service Encryption

### Why

Plaintext gRPC between microservices allows network-level eavesdropping and man-in-the-middle attacks. mTLS ensures both parties verify each other's identity.

### RPC Server with TLS

```yaml
# etc/user.yaml
Name: user.rpc
ListenOn: 0.0.0.0:9000

# mTLS configuration
CertFile: /certs/server.crt
KeyFile: /certs/server.key
# Require client certificates (mutual TLS)
```

```go
// main.go — RPC server with TLS
import (
    "github.com/zeromicro/go-zero/zrpc"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "crypto/tls"
    "crypto/x509"
)

func main() {
    var c config.Config
    conf.MustLoad(*configFile, &c)

    // Load server certificate and key
    serverCert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
    if err != nil {
        log.Fatal(err)
    }

    // Load CA certificate for client verification
    caCert, err := os.ReadFile(c.CACertFile)
    if err != nil {
        log.Fatal(err)
    }
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{serverCert},
        ClientAuth:   tls.RequireAndVerifyClientCert,
        ClientCAs:    caCertPool,
        MinVersion:   tls.VersionTLS13, // Enforce TLS 1.3
    }

    s := zrpc.MustNewServer(c.RpcServerConf, func(srv *grpc.Server) {
        // Register services
    })
    // Apply TLS credentials
    s.AddOptions(grpc.Creds(credentials.NewTLS(tlsConfig)))
    defer s.Stop()
    s.Start()
}
```

### RPC Client with mTLS

```go
// ServiceContext — RPC client with mTLS
func NewServiceContext(c config.Config) *ServiceContext {
    clientCert, err := tls.LoadX509KeyPair(c.ClientCertFile, c.ClientKeyFile)
    if err != nil {
        log.Fatal(err)
    }

    caCert, err := os.ReadFile(c.CACertFile)
    if err != nil {
        log.Fatal(err)
    }
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{clientCert},
        RootCAs:      caCertPool,
        MinVersion:   tls.VersionTLS13,
    }

    conn := zrpc.MustNewClient(c.UserRpc,
        zrpc.WithDialOption(grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))),
    )

    return &ServiceContext{
        Config:  c,
        UserRpc: userclient.NewUser(conn),
    }
}
```

### Certificate Management Best Practices

- Use short-lived certificates (90 days max) with auto-renewal
- Store private keys in Secret Manager (Vault, K8s Secrets with encryption at rest)
- Use separate CA for internal services vs external-facing
- Rotate certificates without downtime using graceful reload

---

## 2. RBAC — Role-Based Access Control

### Role & Permission Model

```sql
CREATE TABLE roles (
    id BIGINT PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE,
    description VARCHAR(256),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE permissions (
    id BIGINT PRIMARY KEY,
    resource VARCHAR(128) NOT NULL,    -- e.g., "posts", "users"
    action VARCHAR(32) NOT NULL,       -- e.g., "read", "write", "delete"
    UNIQUE KEY uk_resource_action (resource, action)
);

CREATE TABLE role_permissions (
    role_id BIGINT NOT NULL,
    permission_id BIGINT NOT NULL,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    PRIMARY KEY (user_id, role_id)
);
```

### RBAC Middleware

```go
// internal/middleware/rbac_middleware.go
package middleware

import (
    "context"
    "net/http"
    "strings"

    "github.com/zeromicro/go-zero/rest/httpx"
)

type RBACMiddleware struct {
    permissionChecker PermissionChecker
}

type PermissionChecker interface {
    HasPermission(ctx context.Context, userId int64, resource, action string) (bool, error)
}

func NewRBACMiddleware(checker PermissionChecker) *RBACMiddleware {
    return &RBACMiddleware{permissionChecker: checker}
}

// RequirePermission returns a middleware that checks for a specific permission
func (m *RBACMiddleware) RequirePermission(resource, action string) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            userId, ok := r.Context().Value("userId").(int64)
            if !ok {
                httpx.ErrorCtx(r.Context(), w, ErrUnauthorized)
                return
            }

            allowed, err := m.permissionChecker.HasPermission(r.Context(), userId, resource, action)
            if err != nil {
                httpx.ErrorCtx(r.Context(), w, ErrInternalServer)
                return
            }
            if !allowed {
                httpx.ErrorCtx(r.Context(), w, ErrForbidden)
                return
            }

            next.ServeHTTP(w, r)
        }
    }
}
```

### Permission Checker with Cache

```go
// internal/svc/permission_checker.go
type CachedPermissionChecker struct {
    userRoleModel    model.UserRoleModel
    rolePermModel    model.RolePermissionModel
    cache            cache.Cache
    cacheTTL         time.Duration
}

func (c *CachedPermissionChecker) HasPermission(ctx context.Context, userId int64, resource, action string) (bool, error) {
    cacheKey := fmt.Sprintf("perm:%d:%s:%s", userId, resource, action)

    // Check cache first
    var allowed bool
    if err := c.cache.GetCtx(ctx, cacheKey, &allowed); err == nil {
        return allowed, nil
    }

    // Query database
    roles, err := c.userRoleModel.FindByUserId(ctx, userId)
    if err != nil {
        return false, err
    }

    for _, role := range roles {
        perms, err := c.rolePermModel.FindByRoleId(ctx, role.RoleId)
        if err != nil {
            return false, err
        }
        for _, perm := range perms {
            if perm.Resource == resource && perm.Action == action {
                _ = c.cache.SetWithExpireCtx(ctx, cacheKey, true, int(c.cacheTTL.Seconds()))
                return true, nil
            }
        }
    }

    _ = c.cache.SetWithExpireCtx(ctx, cacheKey, false, int(c.cacheTTL.Seconds()))
    return false, nil
}
```

### Registering RBAC in Routes

```go
// internal/handler/routes.go
func RegisterHandlers(server *rest.Server, ctx *svc.ServiceContext) {
    rbac := middleware.NewRBACMiddleware(ctx.PermissionChecker)

    server.AddRoutes(
        rest.WithMiddlewares(
            []rest.Middleware{rbac.RequirePermission("posts", "write")},
            []rest.Route{{
                Method:  http.MethodPost,
                Path:    "/api/v1/posts",
                Handler: posts.CreatePostHandler(ctx),
            }}...,
        ),
    )
}
```

---

## 3. OAuth 2.0 / OIDC Integration

### Configuration

```yaml
# etc/gateway.yaml
OAuth:
  ClientId: ${OAUTH_CLIENT_ID}
  ClientSecret: ${OAUTH_CLIENT_SECRET}
  RedirectURL: "https://api.example.com/auth/callback"
  AuthURL: "https://accounts.google.com/o/oauth2/auth"
  TokenURL: "https://oauth2.googleapis.com/token"
  UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo"
  Scopes:
    - openid
    - email
    - profile
```

```go
// internal/config/config.go
type Config struct {
    rest.RestConf

    OAuth struct {
        ClientId     string
        ClientSecret string
        RedirectURL  string
        AuthURL      string
        TokenURL     string
        UserInfoURL  string
        Scopes       []string
    }
}
```

### OAuth Login Flow

```go
// internal/logic/auth/oauth_login_logic.go
import "golang.org/x/oauth2"

func (l *OAuthLoginLogic) OAuthLogin() (*types.OAuthLoginResponse, error) {
    oauthConfig := &oauth2.Config{
        ClientID:     l.svcCtx.Config.OAuth.ClientId,
        ClientSecret: l.svcCtx.Config.OAuth.ClientSecret,
        RedirectURL:  l.svcCtx.Config.OAuth.RedirectURL,
        Scopes:       l.svcCtx.Config.OAuth.Scopes,
        Endpoint: oauth2.Endpoint{
            AuthURL:  l.svcCtx.Config.OAuth.AuthURL,
            TokenURL: l.svcCtx.Config.OAuth.TokenURL,
        },
    }

    // Generate state token to prevent CSRF
    state, err := generateSecureRandom(32)
    if err != nil {
        return nil, err
    }

    // Store state in Redis with TTL
    err = l.svcCtx.Redis.SetexCtx(l.ctx, "oauth:state:"+state, "1", 600)
    if err != nil {
        return nil, err
    }

    url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
    return &types.OAuthLoginResponse{RedirectURL: url}, nil
}
```

### OAuth Callback Handler

```go
// internal/logic/auth/oauth_callback_logic.go
func (l *OAuthCallbackLogic) OAuthCallback(req *types.OAuthCallbackRequest) (*types.LoginResponse, error) {
    // Verify state to prevent CSRF
    exists, err := l.svcCtx.Redis.ExistsCtx(l.ctx, "oauth:state:"+req.State)
    if err != nil || !exists {
        return nil, ErrInvalidState
    }
    // Delete state after use (one-time use)
    _ = l.svcCtx.Redis.DelCtx(l.ctx, "oauth:state:"+req.State)

    // Exchange code for token
    oauthConfig := l.buildOAuthConfig()
    token, err := oauthConfig.Exchange(l.ctx, req.Code)
    if err != nil {
        return nil, fmt.Errorf("oauth token exchange failed: %w", err)
    }

    // Fetch user info
    client := oauthConfig.Client(l.ctx, token)
    resp, err := client.Get(l.svcCtx.Config.OAuth.UserInfoURL)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch user info: %w", err)
    }
    defer resp.Body.Close()

    var userInfo OIDCUserInfo
    if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
        return nil, err
    }

    // Find or create user
    user, err := l.findOrCreateUser(userInfo)
    if err != nil {
        return nil, err
    }

    // Issue JWT
    jwtToken, err := l.generateJWT(user.Id)
    if err != nil {
        return nil, err
    }

    return &types.LoginResponse{
        AccessToken: jwtToken,
        ExpiresIn:   l.svcCtx.Config.Auth.AccessExpire,
    }, nil
}
```

---

## 4. CORS Configuration

### ✅ Correct Pattern — Explicit CORS Middleware

```go
// internal/middleware/cors_middleware.go
package middleware

import "net/http"

type CORSMiddleware struct {
    allowedOrigins map[string]bool
}

func NewCORSMiddleware(origins []string) *CORSMiddleware {
    m := &CORSMiddleware{allowedOrigins: make(map[string]bool)}
    for _, o := range origins {
        m.allowedOrigins[o] = true
    }
    return m
}

func (m *CORSMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")

        // Only allow configured origins — NEVER use "*" with credentials
        if m.allowedOrigins[origin] {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Vary", "Origin")
        }

        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-Idempotency-Key")
        w.Header().Set("Access-Control-Allow-Credentials", "true")
        w.Header().Set("Access-Control-Max-Age", "86400")

        // Handle preflight
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        next.ServeHTTP(w, r)
    }
}
```

### Configuration

```yaml
# etc/gateway.yaml
CORS:
  AllowedOrigins:
    - "https://app.example.com"
    - "https://admin.example.com"
  # NEVER add "*" when using credentials
```

### ❌ Common Mistakes

```go
// DON'T: Allow all origins with credentials
w.Header().Set("Access-Control-Allow-Origin", "*")          // ❌ Wildcard
w.Header().Set("Access-Control-Allow-Credentials", "true")  // ❌ Incompatible with *

// DON'T: Reflect origin blindly
w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))  // ❌ No validation
```

---

## 5. Security Response Headers

### ✅ Security Headers Middleware

```go
// internal/middleware/security_headers_middleware.go
package middleware

import "net/http"

type SecurityHeadersMiddleware struct{}

func NewSecurityHeadersMiddleware() *SecurityHeadersMiddleware {
    return &SecurityHeadersMiddleware{}
}

func (m *SecurityHeadersMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Prevent MIME type sniffing
        w.Header().Set("X-Content-Type-Options", "nosniff")

        // Prevent clickjacking
        w.Header().Set("X-Frame-Options", "DENY")

        // XSS protection (legacy browsers)
        w.Header().Set("X-XSS-Protection", "1; mode=block")

        // Enforce HTTPS
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

        // Content Security Policy (adjust per service)
        w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

        // Prevent referrer leaks
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

        // Restrict browser features
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

        next.ServeHTTP(w, r)
    }
}
```

### Register Security Headers Globally

```go
// main.go
server := rest.MustNewServer(c.RestConf)
defer server.Stop()

// Apply to ALL routes
secHeaders := middleware.NewSecurityHeadersMiddleware()
server.Use(secHeaders.Handle)
```

---

## 6. Secret Management & Rotation

### Environment Variable Pattern (Minimum)

```yaml
# etc/service.yaml — NEVER hardcode secrets
Auth:
  AccessSecret: ${AUTH_ACCESS_SECRET}
  AccessExpire: 86400

MySQL:
  DataSource: ${MYSQL_DSN}

Redis:
  Host: ${REDIS_HOST}
  Pass: ${REDIS_PASSWORD}
```

### HashiCorp Vault Integration

```go
// pkg/vault/client.go
import (
    vault "github.com/hashicorp/vault/api"
)

type VaultClient struct {
    client *vault.Client
    path   string
}

func NewVaultClient(addr, token, path string) (*VaultClient, error) {
    config := vault.DefaultConfig()
    config.Address = addr

    client, err := vault.NewClient(config)
    if err != nil {
        return nil, fmt.Errorf("failed to create vault client: %w", err)
    }
    client.SetToken(token)

    return &VaultClient{client: client, path: path}, nil
}

func (v *VaultClient) GetSecret(key string) (string, error) {
    secret, err := v.client.Logical().Read(v.path)
    if err != nil {
        return "", fmt.Errorf("failed to read secret: %w", err)
    }
    if secret == nil || secret.Data == nil {
        return "", fmt.Errorf("secret not found at path: %s", v.path)
    }

    data, ok := secret.Data["data"].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("unexpected secret format")
    }

    val, ok := data[key].(string)
    if !ok {
        return "", fmt.Errorf("key %s not found", key)
    }
    return val, nil
}
```

### JWT Secret Rotation

```go
// Support dual keys during rotation
type JWTConfig struct {
    CurrentSecret  string // Active signing key
    PreviousSecret string // Still accepted for verification during rotation window
    RotationWindow int64  // Seconds to accept old key after rotation
}

func (l *AuthLogic) VerifyToken(tokenStr string) (*Claims, error) {
    // Try current key first
    claims, err := parseJWT(tokenStr, l.svcCtx.Config.Auth.CurrentSecret)
    if err == nil {
        return claims, nil
    }

    // Fall back to previous key during rotation window
    if l.svcCtx.Config.Auth.PreviousSecret != "" {
        claims, err = parseJWT(tokenStr, l.svcCtx.Config.Auth.PreviousSecret)
        if err == nil {
            return claims, nil
        }
    }

    return nil, ErrInvalidToken
}
```

---

## 7. Audit Logging

### Audit Log Model

```sql
CREATE TABLE audit_logs (
    id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL,        -- e.g., "user.create", "post.delete"
    resource_type VARCHAR(64) NOT NULL,  -- e.g., "user", "post"
    resource_id BIGINT,
    ip_address VARCHAR(45),
    user_agent VARCHAR(256),
    request_body TEXT,                   -- Sanitized (no passwords/tokens)
    response_code INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_action (action),
    INDEX idx_created_at (created_at)
);
```

### Audit Middleware

```go
// internal/middleware/audit_middleware.go
package middleware

import (
    "bytes"
    "io"
    "net/http"

    "github.com/zeromicro/go-zero/core/logx"
)

type AuditMiddleware struct {
    auditLogger AuditLogger
}

type AuditLogger interface {
    Log(ctx context.Context, entry AuditEntry) error
}

type AuditEntry struct {
    UserId       int64
    Action       string
    ResourceType string
    ResourceId   int64
    IPAddress    string
    UserAgent    string
    RequestBody  string
    ResponseCode int
}

func NewAuditMiddleware(logger AuditLogger) *AuditMiddleware {
    return &AuditMiddleware{auditLogger: logger}
}

func (m *AuditMiddleware) Handle(action, resourceType string) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // Capture request body (sanitized)
            body, _ := io.ReadAll(r.Body)
            r.Body = io.NopCloser(bytes.NewBuffer(body))
            sanitizedBody := sanitizeRequestBody(string(body))

            // Wrap response writer to capture status code
            rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

            next.ServeHTTP(rw, r)

            // Log audit entry asynchronously
            userId, _ := r.Context().Value("userId").(int64)
            entry := AuditEntry{
                UserId:       userId,
                Action:       action,
                ResourceType: resourceType,
                IPAddress:    getClientIP(r),
                UserAgent:    r.UserAgent(),
                RequestBody:  sanitizedBody,
                ResponseCode: rw.statusCode,
            }

            go func() {
                if err := m.auditLogger.Log(r.Context(), entry); err != nil {
                    logx.Errorf("failed to write audit log: %v", err)
                }
            }()
        }
    }
}

// sanitizeRequestBody removes sensitive fields before logging
func sanitizeRequestBody(body string) string {
    // Remove password, token, secret fields
    sensitiveFields := []string{"password", "token", "secret", "credit_card"}
    result := body
    for _, field := range sensitiveFields {
        // Replace value with "[REDACTED]"
        re := regexp.MustCompile(`"` + field + `"\s*:\s*"[^"]*"`)
        result = re.ReplaceAllString(result, `"`+field+`":"[REDACTED]"`)
    }
    return result
}

func getClientIP(r *http.Request) string {
    // Check X-Forwarded-For first (behind reverse proxy)
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        // Take the first IP (client IP)
        parts := strings.SplitN(xff, ",", 2)
        return strings.TrimSpace(parts[0])
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

### Response Writer Wrapper

```go
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}
```

---

## 8. Input Validation & Sanitization

### ✅ Request Validation

```go
// Validate at the boundary — in the logic layer, before any business operations
func (l *CreateUserLogic) CreateUser(req *types.CreateUserRequest) (*types.CreateUserResponse, error) {
    // Length limits prevent abuse
    if len(req.Name) > 100 {
        return nil, ErrNameTooLong
    }

    // Email format validation
    if !isValidEmail(req.Email) {
        return nil, ErrInvalidEmail
    }

    // SQL injection prevention — always use parameterized queries
    // ✅ Correct:
    result, err := l.svcCtx.UserModel.Insert(l.ctx, &model.User{Name: req.Name})
    // ❌ NEVER:
    // query := fmt.Sprintf("INSERT INTO users(name) VALUES('%s')", req.Name)
}
```

### Rate Limiting per User

```go
// internal/middleware/rate_limit_middleware.go
type UserRateLimitMiddleware struct {
    redis   *redis.Redis
    rate    int    // requests per window
    window  int    // window in seconds
}

func (m *UserRateLimitMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userId, _ := r.Context().Value("userId").(int64)
        key := fmt.Sprintf("ratelimit:%d:%s", userId, r.URL.Path)

        count, err := m.redis.IncrCtx(r.Context(), key)
        if err != nil {
            // Fail open — allow request if Redis is down
            next.ServeHTTP(w, r)
            return
        }

        if count == 1 {
            _ = m.redis.ExpireCtx(r.Context(), key, m.window)
        }

        if count > int64(m.rate) {
            w.Header().Set("Retry-After", strconv.Itoa(m.window))
            httpx.ErrorCtx(r.Context(), w, ErrTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    }
}
```

---

## Security Checklist

### ✅ DO:
- Use mTLS for all inter-service gRPC communication
- Implement RBAC with cached permission checks
- Validate OAuth state parameter to prevent CSRF
- Set explicit CORS allowed origins (never wildcard with credentials)
- Add security headers to all HTTP responses
- Use environment variables or Vault for all secrets
- Support dual-key JWT verification during rotation
- Log all sensitive operations to audit table
- Sanitize request bodies before audit logging
- Use parameterized queries exclusively
- Rate limit per user/IP on all endpoints

### ❌ DON'T:
- Hardcode secrets in config files or source code
- Use `*` for CORS `Access-Control-Allow-Origin` with credentials
- Trust `X-Forwarded-For` without validating trusted proxy list
- Log passwords, tokens, or credit card numbers
- Store plaintext passwords (use bcrypt with cost >= 12)
- Skip TLS certificate verification in production
- Implement custom cryptographic algorithms
- Return detailed error messages to clients in production

---

## Cross-References

- For JWT authentication middleware, see [REST API Patterns](./rest-api-patterns.md#middleware-pattern)
- For Redis-based distributed locks, see [Concurrency Patterns](./concurrency-patterns.md#4-distributed-locks)
- For structured logging with PII masking, see [Observability Patterns](./observability-patterns.md)
- For resilience (rate limiting, circuit breaker), see [Resilience Patterns](./resilience-patterns.md)

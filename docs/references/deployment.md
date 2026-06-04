# Deployment Patterns for go-zero

## Overview

This guide covers production deployment patterns for go-zero microservices:
- Canary / Blue-Green deployment strategies
- Database migration with zero downtime
- Helm Charts for Kubernetes
- HPA / VPA auto-scaling
- GitOps workflow with ArgoCD

---

## 1. Deployment Strategies

### Blue-Green Deployment

Two identical environments; switch traffic atomically.

```
                   ┌─────────────────┐
                   │  Load Balancer  │
                   └────────┬────────┘
                            │
              ┌─────────────┼─────────────┐
              │ (active)    │ (standby)   │
              ▼             │             ▼
        ┌───────────┐      │       ┌───────────┐
        │  Blue v1  │      │       │ Green v2  │
        │ (3 pods)  │      │       │ (3 pods)  │
        └───────────┘      │       └───────────┘
                            │
                    Switch after validation
```

#### Kubernetes Service Switching

```yaml
# service.yaml — switch by updating selector
apiVersion: v1
kind: Service
metadata:
  name: user-api
spec:
  selector:
    app: user-api
    version: green  # Change to "blue" to rollback
  ports:
    - port: 8080
      targetPort: 8080
```

#### Blue-Green Script

```bash
#!/bin/bash
# deploy-blue-green.sh
SERVICE="user-api"
NEW_VERSION="$1"
CURRENT=$(kubectl get svc $SERVICE -o jsonpath='{.spec.selector.version}')

if [ "$CURRENT" = "blue" ]; then
    TARGET="green"
else
    TARGET="blue"
fi

echo "Deploying $NEW_VERSION to $TARGET environment..."

# Deploy new version to standby
kubectl set image deployment/${SERVICE}-${TARGET} ${SERVICE}=${IMAGE}:${NEW_VERSION}
kubectl rollout status deployment/${SERVICE}-${TARGET} --timeout=300s

# Run health checks
echo "Running health checks..."
kubectl exec -it $(kubectl get pod -l app=$SERVICE,version=$TARGET -o jsonpath='{.items[0].metadata.name}') \
    -- wget -q -O- http://localhost:8080/health

# Switch traffic
echo "Switching traffic to $TARGET..."
kubectl patch svc $SERVICE -p "{\"spec\":{\"selector\":{\"version\":\"$TARGET\"}}}"

echo "Deployment complete. Rollback: kubectl patch svc $SERVICE -p '{\"spec\":{\"selector\":{\"version\":\"$CURRENT\"}}}'"
```

### Canary Deployment

Route a small percentage of traffic to the new version.

#### Using Nginx Ingress Annotations

```yaml
# canary-ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: user-api-canary
  annotations:
    nginx.ingress.kubernetes.io/canary: "true"
    nginx.ingress.kubernetes.io/canary-weight: "10"  # 10% traffic
spec:
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /api/
            pathType: Prefix
            backend:
              service:
                name: user-api-canary
                port:
                  number: 8080
```

#### Canary Promotion Steps

```bash
# 1. Deploy canary (10% traffic)
kubectl apply -f canary-ingress.yaml  # canary-weight: "10"

# 2. Monitor for 15 minutes
#    - Check error rates in Prometheus
#    - Check P99 latency
#    - Check business metrics

# 3. Increase traffic (50%)
kubectl annotate ingress user-api-canary \
    nginx.ingress.kubernetes.io/canary-weight="50" --overwrite

# 4. Monitor for another 15 minutes

# 5. Full promotion (update main deployment, remove canary)
kubectl set image deployment/user-api user-api=${IMAGE}:${NEW_VERSION}
kubectl rollout status deployment/user-api
kubectl delete -f canary-ingress.yaml

# OR Rollback
kubectl delete -f canary-ingress.yaml
kubectl rollout undo deployment/user-api-canary
```

### Rolling Update (Default K8s)

```yaml
# deployment.yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0     # Never reduce capacity below desired
      maxSurge: 1           # Add 1 extra pod during update
  template:
    spec:
      terminationGracePeriodSeconds: 60
      containers:
        - name: user-api
          lifecycle:
            preStop:
              exec:
                command: ["/bin/sh", "-c", "sleep 10"]  # Drain connections
```

---

## 2. Database Migration

### Zero-Downtime Migration Rules

1. **Never** drop columns or tables in the same release that removes code references
2. **Always** make migrations backwards-compatible
3. **Expand then contract**: add first, remove later (2-phase)

### Migration Tool: golang-migrate

```bash
# Install
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Create migration
migrate create -ext sql -dir migrations -seq add_phone_to_users

# Run migrations
migrate -path migrations -database "mysql://${MYSQL_DSN}" up

# Rollback last migration
migrate -path migrations -database "mysql://${MYSQL_DSN}" down 1
```

### Safe Migration Examples

#### Adding a Column (Safe)

```sql
-- migrations/000001_add_phone_to_users.up.sql
ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT '' AFTER email;

-- migrations/000001_add_phone_to_users.down.sql
ALTER TABLE users DROP COLUMN phone;
```

#### Renaming a Column (2-Phase)

```sql
-- Phase 1: Add new column, keep old (deploy code that writes to both)
-- migrations/000002_add_mobile_column.up.sql
ALTER TABLE users ADD COLUMN mobile VARCHAR(20) DEFAULT '' AFTER phone;
UPDATE users SET mobile = phone WHERE phone != '';

-- Phase 2: (next release) Drop old column after all code uses new one
-- migrations/000003_drop_phone_column.up.sql
ALTER TABLE users DROP COLUMN phone;
```

#### Adding Index without Locking (MySQL 8.0+)

```sql
-- For large tables, use ALGORITHM=INPLACE to avoid table lock
ALTER TABLE posts ADD INDEX idx_user_created (user_id, created_at), ALGORITHM=INPLACE, LOCK=NONE;
```

### Migration in CI/CD

```yaml
# .github/workflows/deploy.yml (excerpt)
- name: Run Database Migrations
  run: |
    migrate -path migrations \
      -database "mysql://${MYSQL_DSN}" \
      up
  env:
    MYSQL_DSN: ${{ secrets.MYSQL_DSN }}

- name: Deploy Application
  run: |
    kubectl set image deployment/user-api \
      user-api=${{ env.IMAGE }}:${{ github.sha }}
```

---

## 3. Helm Charts

### Chart Structure

```
helm/
└── user-api/
    ├── Chart.yaml
    ├── values.yaml
    ├── values-staging.yaml
    ├── values-production.yaml
    └── templates/
        ├── deployment.yaml
        ├── service.yaml
        ├── ingress.yaml
        ├── configmap.yaml
        ├── hpa.yaml
        └── _helpers.tpl
```

### Chart.yaml

```yaml
apiVersion: v2
name: user-api
description: User management microservice
type: application
version: 1.0.0
appVersion: "1.0.0"
```

### values.yaml (Default)

```yaml
replicaCount: 2

image:
  repository: registry.example.com/user-api
  tag: "latest"  # Overridden per environment
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: api.example.com
      paths:
        - path: /api/v1/users
          pathType: Prefix

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  targetMemoryUtilization: 80

config:
  name: user-api
  host: 0.0.0.0
  port: 8080
  logLevel: info

probes:
  startup:
    path: /health
    failureThreshold: 30
    periodSeconds: 2
  liveness:
    path: /health
    initialDelaySeconds: 5
    periodSeconds: 10
  readiness:
    path: /health
    initialDelaySeconds: 3
    periodSeconds: 5
```

### values-production.yaml

```yaml
replicaCount: 3

image:
  tag: "v1.2.3"  # Pinned version, NEVER "latest"

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: "1"
    memory: 1Gi

autoscaling:
  minReplicas: 3
  maxReplicas: 20

config:
  logLevel: error
```

### Deployment Template

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "user-api.fullname" . }}
  labels:
    {{- include "user-api.labels" . | nindent 4 }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "user-api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "user-api.selectorLabels" . | nindent 8 }}
    spec:
      terminationGracePeriodSeconds: 60
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - containerPort: {{ .Values.service.port }}
          envFrom:
            - secretRef:
                name: {{ include "user-api.fullname" . }}-secrets
          volumeMounts:
            - name: config
              mountPath: /app/etc
          startupProbe:
            httpGet:
              path: {{ .Values.probes.startup.path }}
              port: {{ .Values.service.port }}
            failureThreshold: {{ .Values.probes.startup.failureThreshold }}
            periodSeconds: {{ .Values.probes.startup.periodSeconds }}
          livenessProbe:
            httpGet:
              path: {{ .Values.probes.liveness.path }}
              port: {{ .Values.service.port }}
            initialDelaySeconds: {{ .Values.probes.liveness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
          readinessProbe:
            httpGet:
              path: {{ .Values.probes.readiness.path }}
              port: {{ .Values.service.port }}
            initialDelaySeconds: {{ .Values.probes.readiness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
      volumes:
        - name: config
          configMap:
            name: {{ include "user-api.fullname" . }}-config
```

### Deploy Commands

```bash
# Install
helm install user-api ./helm/user-api -f helm/user-api/values-production.yaml

# Upgrade
helm upgrade user-api ./helm/user-api -f helm/user-api/values-production.yaml \
    --set image.tag=v1.2.4

# Rollback
helm rollback user-api 1

# Dry run (preview changes)
helm upgrade user-api ./helm/user-api -f helm/user-api/values-production.yaml \
    --set image.tag=v1.2.4 --dry-run --debug
```

---

## 4. HPA / VPA Auto-Scaling

### Horizontal Pod Autoscaler (HPA)

```yaml
# templates/hpa.yaml
{{- if .Values.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "user-api.fullname" . }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "user-api.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetCPUUtilization }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetMemoryUtilization }}
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 2
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300  # 5 min cool-down before scaling down
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
{{- end }}
```

### Custom Metrics HPA (Prometheus-based)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: user-api-custom
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: user-api
  minReplicas: 2
  maxReplicas: 20
  metrics:
    # Scale on request rate
    - type: Pods
      pods:
        metric:
          name: http_requests_per_second
        target:
          type: AverageValue
          averageValue: "100"
    # Scale on queue depth
    - type: External
      external:
        metric:
          name: kafka_consumer_lag
          selector:
            matchLabels:
              topic: order.created
        target:
          type: AverageValue
          averageValue: "1000"
```

### Vertical Pod Autoscaler (VPA)

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: user-api-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: user-api
  updatePolicy:
    updateMode: "Auto"  # "Off" for recommendations only
  resourcePolicy:
    containerPolicies:
      - containerName: user-api
        minAllowed:
          cpu: 50m
          memory: 64Mi
        maxAllowed:
          cpu: "2"
          memory: 2Gi
```

> **Note**: Do NOT use HPA and VPA on the same resource metric (e.g., both scaling on CPU). Use HPA for CPU/memory and VPA for right-sizing during off-peak.

---

## 5. GitOps with ArgoCD

### Application Definition

```yaml
# argocd/user-api.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: user-api
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/infra-manifests.git
    targetRevision: main
    path: helm/user-api
    helm:
      valueFiles:
        - values-production.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: production
  syncPolicy:
    automated:
      prune: true        # Remove resources not in git
      selfHeal: true     # Fix manual changes (drift)
    syncOptions:
      - CreateNamespace=true
    retry:
      limit: 3
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
```

### GitOps Workflow

```
Developer → Push code → CI builds image → Update image tag in infra repo → ArgoCD syncs

1. git push (feature branch) → CI pipeline
2. CI: build → test → push image:sha256
3. CI: update helm/user-api/values-production.yaml → image.tag: sha256
4. ArgoCD detects change → syncs to cluster
5. ArgoCD monitors health → auto-rollback on failure
```

### CI Pipeline Example (GitHub Actions)

```yaml
# .github/workflows/deploy.yml
name: Build and Deploy

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build and Push Image
        run: |
          docker build -t $REGISTRY/user-api:${{ github.sha }} .
          docker push $REGISTRY/user-api:${{ github.sha }}

      - name: Update Infra Repo
        run: |
          git clone https://github.com/your-org/infra-manifests.git
          cd infra-manifests
          sed -i "s|tag:.*|tag: \"${{ github.sha }}\"|" helm/user-api/values-production.yaml
          git add .
          git commit -m "deploy: user-api ${{ github.sha }}"
          git push
```

---

## 6. go-zero Graceful Shutdown

go-zero handles graceful shutdown automatically, but ensure your code cooperates:

```go
// main.go
func main() {
    var c config.Config
    conf.MustLoad(*configFile, &c)

    server := rest.MustNewServer(c.RestConf)
    defer server.Stop() // Triggers graceful shutdown

    ctx := svc.NewServiceContext(c)
    handler.RegisterHandlers(server, ctx)

    // Start background workers with cancellation
    workerCtx, cancel := context.WithCancel(context.Background())
    go startOutboxPublisher(workerCtx, ctx)
    go startConsumers(workerCtx, ctx)

    // Cleanup on shutdown
    proc.AddShutdownListener(func() {
        cancel()                    // Stop background workers
        ctx.DB.Close()              // Close DB connections
        ctx.Redis.Close()           // Close Redis
        logx.Info("shutdown complete")
    })

    fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
    server.Start()
}
```

---

## Deployment Checklist

### ✅ DO:
- Use pinned image tags (sha or semver), NEVER `:latest` in production
- Run DB migrations before deploying new code
- Make migrations backwards-compatible (expand-then-contract)
- Set `terminationGracePeriodSeconds` >= connection drain time
- Configure startup, liveness, AND readiness probes
- Use HPA with scale-down stabilization (5+ minutes)
- Store secrets in K8s Secrets / Vault, not ConfigMaps
- Use GitOps for production deployments (auditable, reversible)
- Test Helm charts with `--dry-run` before applying

### ❌ DON'T:
- Deploy code and DB migration simultaneously (race condition)
- Use `kubectl apply` manually in production (use GitOps)
- Drop columns in the same release that removes code references
- Set HPA and VPA on the same metric (conflicts)
- Skip health checks during canary promotion
- Use `maxUnavailable: 100%` in rolling updates

---

## Cross-References

- For health check endpoint implementation, see [Observability Patterns](./observability-patterns.md)
- For K8s config templates, see [goctl Commands](./goctl-commands.md)
- For graceful shutdown patterns, see [Resilience Patterns](./resilience-patterns.md)
- For secret management, see [Security Patterns](./security-patterns.md#6-secret-management--rotation)

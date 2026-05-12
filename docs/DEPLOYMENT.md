# Gateway-A Deployment Runbook

## Overview

This runbook provides step-by-step instructions for deploying Gateway-A to Kubernetes.

**Last Updated:** 2026-05-12  
**Version:** 1.0.0

---

## Prerequisites

Before deploying, ensure you have:

1. **Kubernetes Cluster** - v1.25+ (tested on v1.28)
2. **kubectl** configured with cluster access
3. **Docker Registry** - for pushing container images
4. **Redis** - Redis 7+ instance (or use Helm chart)
5. **PostgreSQL** - PostgreSQL 15+ database (or use Helm chart)
6. **TLS Certificate** - valid certificate for your domain

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                    Gateway-A                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │   │
│  │  │   Ingress   │─▶│   Service   │─▶│   Pods      │  │   │
│  │  │   Nginx     │  │ (Load Bal.) │  │ (Replicas)  │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                      │              │                        │
│                      ▼              ▼                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │  Redis      │  │ PostgreSQL  │  │   LLM API   │          │
│  │  (Cache)    │  │ (Database)  │  │  (External) │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

---

## Quick Start Deployment

### Step 1: Build and Push Container Image

```bash
# Build Docker image
docker build -t your-registry/gateway-a:latest .

# Push to registry
docker push your-registry/gateway-a:latest
```

### Step 2: Create Namespace

```bash
kubectl create namespace gateway-a
```

### Step 3: Create Secrets

```bash
# Redis credentials
kubectl create secret generic gateway-redis \
  --namespace=gateway-a \
  --from-literal=host=redis.default.svc.cluster.local \
  --from-literal=port=6379 \
  --from-literal=password=changeme \
  --from-literal=database=0

# PostgreSQL credentials
kubectl create secret generic gateway-postgres \
  --namespace=gateway-a \
  --from-literal=host=postgres.default.svc.cluster.local \
  --from-literal=port=5432 \
  --from-literal=username=dbuser \
  --from-literal=password=changeme \
  --from-literal=database=gateway_a

# API keys (optional - for upstream LLM providers)
kubectl create secret generic gateway-llm-keys \
  --namespace=gateway-a \
  --from-literal=openai-api-key=your-api-key \
  --from-literal=anthropic-api-key=your-api-key
```

### Step 4: Deploy Gateway-A

```bash
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml
```

### Step 5: Verify Deployment

```bash
# Check pod status
kubectl get pods -n gateway-a

# Check logs
kubectl logs -f deployment/gateway-a -n gateway-a

# Test health endpoint
kubectl port-forward svc/gateway-a 8080:80 -n gateway-a
curl http://localhost:8080/health
```

---

## Detailed Configuration

### 1. Deployment Manifest (`k8s/deployment.yaml`)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-a
  namespace: gateway-a
  labels:
    app: gateway-a
spec:
  replicas: 3
  selector:
    matchLabels:
      app: gateway-a
  template:
    metadata:
      labels:
        app: gateway-a
    spec:
      containers:
      - name: gateway-a
        image: your-registry/gateway-a:latest
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: "/etc/gateway/config.yaml"
        - name: LOG_LEVEL
          value: "info"
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: config
          mountPath: /etc/gateway
      volumes:
      - name: config
        configMap:
          name: gateway-a-config
```

### 2. Service Manifest (`k8s/service.yaml`)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-a
  namespace: gateway-a
  labels:
    app: gateway-a
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: gateway-a
```

### 3. Ingress Manifest (`k8s/ingress.yaml`)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gateway-a
  namespace: gateway-a
  annotations:
    nginx.ingress.kubernetes.io/proxy-connect-timeout: "60"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "300"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - api.your-domain.com
    secretName: gateway-a-tls
  rules:
  - host: api.your-domain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: gateway-a
            port:
              number: 80
```

### 4. ConfigMap (`k8s/configmap.yaml`)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-a-config
  namespace: gateway-a
data:
  config.yaml: |
    redis:
      host: ${REDIS_HOST}
      port: ${REDIS_PORT}
      password: ${REDIS_PASSWORD}
      database: 0

    database:
      host: ${POSTGRES_HOST}
      port: "5432"
      username: ${POSTGRES_USERNAME}
      password: ${POSTGRES_PASSWORD}
      dbname: gateway_a

    logging:
      level: info
      format: json

    server:
      host: "0.0.0.0"
      port: 8080

    rate_limit:
      enabled: true
      rate: 10
      bucket_size: 100
      window: 1m

    circuit_breaker:
      enabled: true
      failure_threshold: 50
      reset_timeout: 30s
```

---

## Redis Deployment (Optional)

If you don't have an existing Redis instance, deploy with Helm:

```bash
# Add Redis Helm repo
helm repo add redis https://repo.redis.io/helm/charts
helm repo update

# Install Redis
helm install redis redis/redis \
  --namespace=default \
  --set auth.enabled=true \
  --set auth.password=changeme \
  --set architecture=standalone
```

---

## PostgreSQL Deployment (Optional)

```bash
# Add PostgreSQL Helm repo
helm repo add bitnami https://charts.bitnami.com/bitnami

# Install PostgreSQL
helm install postgres bitnami/postgresql \
  --namespace=default \
  --set auth.postgresPassword=changeme \
  --set auth.database=gateway_a
```

---

## Configuration Options

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CONFIG_PATH` | Path to config file | `/etc/gateway/config.yaml` |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `REDIS_HOST` | Redis host address | - |
| `REDIS_PORT` | Redis port | `6379` |
| `REDIS_PASSWORD` | Redis password | - |

### Rate Limiting Configuration

```yaml
rate_limit:
  enabled: true
  rate: 10          # tokens per second
  bucket_size: 100  # max tokens
  window: 1m        # sliding window duration
```

### Circuit Breaker Configuration

```yaml
circuit_breaker:
  enabled: true
  failure_threshold: 50   # percentage
  reset_timeout: 30s
  half_open_max_requests: 3
```

---

## Monitoring

### Prometheus Metrics

Gateway-A exposes Prometheus metrics at `/metrics`:

```yaml
# prometheus.yaml
apiVersion: v1
kind: ServiceMonitor
metadata:
  name: gateway-a
  namespace: gateway-a
  labels:
    app: gateway-a
spec:
  selector:
    matchLabels:
      app: gateway-a
  endpoints:
  - port: http
    path: /metrics
    interval: 15s
```

### Key Metrics

| Metric | Description |
|--------|-------------|
| `gateway_a_requests_total` | Total requests |
| `gateway_a_llm_requests_total` | LLM requests |
| `gateway_a_llm_tokens_total` | Tokens processed |
| `gateway_a_queue_jobs_enqueued_total` | Jobs enqueued |
| `gateway_a_rate_limit_exceeded_total` | Rate limit events |
| `gateway_a_circuit_breaker_state` | Circuit breaker state |

### Grafana Dashboard

Import the dashboard JSON at `monitoring/dashboard.json` to Grafana.

---

## High Availability

### Horizontal Pod Autoscaler (HPA)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: gateway-a
  namespace: gateway-a
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: gateway-a
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### Pod Disruption Budget (PDB)

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: gateway-a
  namespace: gateway-a
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: gateway-a
```

---

## Scaling Guidelines

### CPU/Memory Recommendations

| Workload | Min Replicas | CPU Request | CPU Limit | Memory Request | Memory Limit |
|----------|--------------|-------------|-----------|----------------|--------------|
| Low Traffic | 2 | 100m | 500m | 256Mi | 512Mi |
| Medium Traffic | 3 | 250m | 1000m | 512Mi | 1Gi |
| High Traffic | 5+ | 500m | 2000m | 1Gi | 2Gi |

### Queue Worker Scaling

For high job throughput, increase worker pool:

```yaml
queue:
  pool_size: 8  # Default is 4
```

---

## Troubleshooting

### Common Issues

1. **Pod won't start**
   ```bash
   kubectl describe pod <pod-name> -n gateway-a
   kubectl logs <pod-name> -n gateway-a
   ```

2. **Redis connection failed**
   ```bash
   # Check Redis service
   kubectl get svc redis -n default
   # Test connectivity from pod
   kubectl exec -it <pod-name> -n gateway-a -- redis-cli -h redis ping
   ```

3. **Database connection failed**
   ```bash
   # Check PostgreSQL service
   kubectl get svc postgres -n default
   # Test connectivity
   kubectl exec -it <pod-name> -n gateway-a -- psql -h postgres -U dbuser -d gateway_a
   ```

4. **Rate limiting too aggressive**
   - Increase `rate` or `bucket_size` in config
   - Add exceptions for specific clients

5. **Circuit breaker constantly open**
   - Check upstream service health
   - Increase `reset_timeout`
   - Review `failure_threshold`

---

## Rollout and Rollback

### Rolling Update

```bash
# Update image
kubectl set image deployment/gateway-a gateway-a=your-registry/gateway-a:v2 -n gateway-a

# Check rollout status
kubectl rollout status deployment/gateway-a -n gateway-a
```

### Rollback

```bash
# Undo to previous version
kubectl rollout undo deployment/gateway-a -n gateway-a

# Check history
kubectl rollout history deployment/gateway-a -n gateway-a
```

---

## Disaster Recovery

### Backup Procedures

```bash
# Backup Kubernetes resources
kubectl get all -n gateway-a -o yaml > backup-$(date +%Y%m%d).yaml

# Backup PostgreSQL
PGPASSWORD=changeme pg_dump -h postgres -U dbuser -d gateway_a > pg_backup_$(date +%Y%m%d).sql
```

### Recovery Procedures

```bash
# Restore Kubernetes resources
kubectl apply -f backup-$(date +%Y%m%d).yaml

# Restore PostgreSQL
PGPASSWORD=changeme psql -h postgres -U dbuser -d gateway_a < pg_backup_$(date +%Y%m%d).sql
```

---

## Security Best Practices

1. **Use TLS** for all external traffic
2. **Rotate secrets** regularly
3. **Enable Pod Security Policies**
4. **Use network policies** to restrict access
5. **Enable audit logging** for compliance
6. **Use RBAC** for least privilege access

---

## Maintenance

### Regular Tasks

- [ ] Check logs for errors weekly
- [ ] Review metrics dashboards daily
- [ ] Rotate API keys quarterly
- [ ] Update container images for security patches
- [ ] Review and adjust rate limits based on usage

### Upgrade Procedure

```bash
# 1. Backup current state
kubectl get all -n gateway-a -o yaml > backup-pre-upgrade.yaml

# 2. Update image version
kubectl set image deployment/gateway-a gateway-a=your-registry/gateway-a:v2.0.0 -n gateway-a

# 3. Monitor rollout
kubectl rollout status deployment/gateway-a -n gateway-a

# 4. Verify health
kubectl get pods -n gateway-a

# 5. Check metrics
curl http://gateway-a.gateway-a.svc.cluster.local:8080/metrics
```

---

## Support

For issues and support:

- **GitHub Issues**: https://github.com/gateway-a/issues
- **Documentation**: https://gateway-a.example.com/docs
- **Email**: support@gateway-a.example.com

---

## Changelog

- **v1.0.0** (2026-05-12) - Initial deployment runbook

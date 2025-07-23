# AgentSmith-HUB Helm Chart

This Helm chart deploys AgentSmith-HUB, a Security Data Pipeline Platform with integrated real-time threat detection engine, on Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure

## Quick Start

### 1. Add the Helm repository (if using a repository)

```bash
helm repo add agentsmith-hub https://your-repo-url
helm repo update
```

### 2. Install the chart

```bash
# Install with default values
helm install agentsmith-hub ./helm/agentsmith-hub

# Install with custom values
helm install agentsmith-hub ./helm/agentsmith-hub -f values-custom.yaml

# Install in a specific namespace
kubectl create namespace agentsmith-hub
helm install agentsmith-hub ./helm/agentsmith-hub --namespace agentsmith-hub
```

### 3. Access the application

After installation, follow the instructions displayed by Helm to access the application.

## Architecture

The chart deploys the following components:

- **Leader**: Single instance managing the cluster and providing API endpoints
- **Followers**: Multiple instances for load distribution and high availability
- **Frontend**: Web interface for configuration and monitoring
- **Redis**: Data store for cluster state and caching

## Configuration

### Default Values

The chart comes with sensible defaults, but you can customize the deployment by overriding values.

### Key Configuration Options

#### Leader Configuration

```yaml
leader:
  enabled: true
  replicaCount: 1
  resources:
    limits:
      cpu: 1000m
      memory: 2Gi
    requests:
      cpu: 500m
      memory: 1Gi
```

#### Follower Configuration

```yaml
follower:
  enabled: true
  replicaCount: 2
  resources:
    limits:
      cpu: 500m
      memory: 1Gi
    requests:
      cpu: 250m
      memory: 512Mi
```

#### Redis Configuration

```yaml
redis:
  enabled: true
  auth:
    password: "toor"
  master:
    persistence:
      enabled: true
      size: 8Gi
```

#### Frontend Configuration

```yaml
frontend:
  enabled: true
  replicaCount: 1
  ingress:
    enabled: false
    hosts:
      - host: agentsmith-hub.local
        paths:
          - path: /
            pathType: Prefix
```

### Environment-Specific Values

Create environment-specific value files:

#### Development

```yaml
# values-dev.yaml
leader:
  replicaCount: 1
follower:
  replicaCount: 1
frontend:
  replicaCount: 1
redis:
  master:
    persistence:
      enabled: false
```

#### Production

```yaml
# values-prod.yaml
leader:
  replicaCount: 1
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
follower:
  replicaCount: 3
  resources:
    limits:
      cpu: 1000m
      memory: 2Gi
frontend:
  replicaCount: 2
  ingress:
    enabled: true
    hosts:
      - host: agentsmith-hub.yourdomain.com
redis:
  master:
    persistence:
      enabled: true
      size: 20Gi
hpa:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
```

## Advanced Configuration

### Ingress Configuration

To enable ingress, set `frontend.ingress.enabled: true` and configure your ingress controller:

```yaml
frontend:
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: /
    hosts:
      - host: agentsmith-hub.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: agentsmith-hub-tls
        hosts:
          - agentsmith-hub.yourdomain.com
```

### Persistent Storage

Configure persistent storage for logs and Redis data:

```yaml
persistence:
  enabled: true
  storageClass: "fast-ssd"
  size: 10Gi

redis:
  master:
    persistence:
      enabled: true
      storageClass: "fast-ssd"
      size: 8Gi
```

### Resource Management

Configure resource limits and requests:

```yaml
leader:
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
    requests:
      cpu: 1000m
      memory: 2Gi

follower:
  resources:
    limits:
      cpu: 1000m
      memory: 2Gi
    requests:
      cpu: 500m
      memory: 1Gi
```

### Horizontal Pod Autoscaling

Enable HPA for automatic scaling:

```yaml
hpa:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80
```

## Monitoring and Logging

### Health Checks

The chart includes health checks for all components:

- Liveness probes ensure pods are restarted if they become unresponsive
- Readiness probes ensure traffic is only sent to healthy pods

### Logging

Logs are stored in `/tmp/hub_logs` within each pod. For persistent logging, enable persistence:

```yaml
persistence:
  enabled: true
```

### Monitoring

To enable monitoring with Prometheus:

```yaml
monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s
```

## Security

### Network Policies

Enable network policies to restrict pod communication:

```yaml
networkPolicy:
  enabled: true
```

### Pod Security Standards

Enable Pod Security Standards:

```yaml
podSecurityStandards:
  enabled: true
  level: "restricted"
```

### RBAC

RBAC is enabled by default. The chart creates:

- ServiceAccount for the application
- Role with necessary permissions
- RoleBinding to bind the role to the service account

## Troubleshooting

### Common Issues

1. **Pods not starting**: Check resource limits and node capacity
2. **Redis connection issues**: Verify Redis service is running and accessible
3. **Leader-follower communication**: Ensure network policies allow communication

### Debug Commands

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=agentsmith-hub

# Check service status
kubectl get svc -l app.kubernetes.io/name=agentsmith-hub

# Check events
kubectl get events --sort-by='.lastTimestamp'

# Check logs
kubectl logs -l app.kubernetes.io/name=agentsmith-hub

# Access Redis CLI
kubectl exec -it <redis-pod> -- redis-cli -a <password>
```

### Scaling

Scale followers dynamically:

```bash
kubectl scale deployment agentsmith-hub-follower --replicas=5
```

## Upgrading

### Upgrade the chart

```bash
helm upgrade agentsmith-hub ./helm/agentsmith-hub
```

### Rollback

```bash
helm rollback agentsmith-hub
```

## Uninstalling

```bash
helm uninstall agentsmith-hub
```

**Note**: This will remove all resources created by the chart. If you enabled persistence, you may need to manually delete PersistentVolumeClaims.

## Contributing

To contribute to this Helm chart:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with `helm template` and `helm lint`
5. Submit a pull request

## License

This chart is licensed under the same license as AgentSmith-HUB. 
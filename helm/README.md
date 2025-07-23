# AgentSmith-HUB Kubernetes Deployment

This directory contains the Helm chart for deploying AgentSmith-HUB on Kubernetes.

## Quick Start

### Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure

### Installation

1. **Development Environment**:
   ```bash
   helm install agentsmith-hub ./agentsmith-hub -f ./agentsmith-hub/values-dev.yaml
   ```

2. **Production Environment**:
   ```bash
   helm install agentsmith-hub ./agentsmith-hub -f ./agentsmith-hub/values-prod.yaml
   ```

3. **Custom Configuration**:
   ```bash
   helm install agentsmith-hub ./agentsmith-hub -f custom-values.yaml
   ```

### Access the Application

After installation, follow the instructions displayed by Helm to access the application.

## Architecture

The Helm chart deploys a complete AgentSmith-HUB cluster with:

- **1 Leader**: Manages the cluster and provides API endpoints
- **2+ Followers**: Handle data processing and load distribution
- **1+ Frontend**: Web interface for configuration and monitoring
- **1 Redis**: Data store for cluster state and caching

## Components

### Leader
- Single instance (can be scaled to multiple for high availability)
- Manages cluster coordination
- Provides REST API endpoints
- Handles configuration management

### Followers
- Multiple instances for load distribution
- Process data streams
- Execute rules and plugins
- Communicate with leader for coordination

### Frontend
- Web-based user interface
- Configuration management
- Real-time monitoring
- Cluster status visualization

### Redis
- Cluster state storage
- Configuration caching
- Message queuing
- Session management

## Configuration

### Environment-Specific Values

- `values-dev.yaml`: Development environment with relaxed security and minimal resources
- `values-prod.yaml`: Production environment with strict security and high availability
- `values.yaml`: Default values (balanced configuration)

### Key Configuration Options

#### Scaling
```yaml
follower:
  replicaCount: 3  # Number of follower instances

hpa:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
```

#### Resources
```yaml
leader:
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
    requests:
      cpu: 1000m
      memory: 2Gi
```

#### Storage
```yaml
persistence:
  enabled: true
  storageClass: "fast-ssd"
  size: 20Gi

redis:
  master:
    persistence:
      enabled: true
      size: 20Gi
```

#### Security
```yaml
networkPolicy:
  enabled: true

podSecurityStandards:
  enabled: true
  level: "restricted"
```

## Monitoring and Logging

### Health Checks
- Liveness probes for all components
- Readiness probes for traffic management
- Startup probes for slow-starting containers

### Logging
- Centralized logging via ConfigMap
- Persistent log storage (optional)
- Log level configuration

### Monitoring
- Prometheus metrics (optional)
- ServiceMonitor for Prometheus integration
- Custom metrics for business KPIs

## Security Features

### Network Policies
- Restrict pod-to-pod communication
- Allow only necessary ports and protocols
- Isolate components by namespace

### RBAC
- ServiceAccount with minimal required permissions
- Role-based access control
- Namespace isolation

### Pod Security
- Non-root containers
- Read-only root filesystem
- No privilege escalation
- Security context constraints

## Backup and Recovery

### Data Backup
- Redis persistence enabled
- ConfigMap for configuration backup
- PVC for log persistence

### Disaster Recovery
- Multi-replica deployments
- Pod disruption budgets
- Rolling update strategies

## Troubleshooting

### Common Issues

1. **Pods not starting**:
   ```bash
   kubectl describe pod <pod-name>
   kubectl logs <pod-name>
   ```

2. **Redis connection issues**:
   ```bash
   kubectl get svc agentsmith-hub-redis
   kubectl exec -it <redis-pod> -- redis-cli ping
   ```

3. **Leader-follower communication**:
   ```bash
   kubectl logs -l app.kubernetes.io/component=leader
   kubectl logs -l app.kubernetes.io/component=follower
   ```

### Debug Commands

```bash
# Check all resources
kubectl get all -l app.kubernetes.io/name=agentsmith-hub

# Check events
kubectl get events --sort-by='.lastTimestamp'

# Port forward to services
kubectl port-forward svc/agentsmith-hub-frontend 8080:80
kubectl port-forward svc/agentsmith-hub-leader 8081:8080

# Access Redis CLI
kubectl exec -it <redis-pod> -- redis-cli -a toor
```

## Upgrading

### Chart Upgrade
```bash
helm upgrade agentsmith-hub ./agentsmith-hub
```

### Rollback
```bash
helm rollback agentsmith-hub
```

### Version Migration
```bash
# Check current version
helm list

# Upgrade with new values
helm upgrade agentsmith-hub ./agentsmith-hub -f new-values.yaml
```

## Uninstalling

```bash
helm uninstall agentsmith-hub
```

**Note**: This removes all resources. Persistent data may need manual cleanup.

## Development

### Local Development
```bash
# Template the chart
helm template agentsmith-hub ./agentsmith-hub

# Lint the chart
helm lint ./agentsmith-hub

# Dry run installation
helm install agentsmith-hub ./agentsmith-hub --dry-run
```

### Customization
- Modify `values.yaml` for default changes
- Create environment-specific value files
- Use `--set` for command-line overrides

## Support

For issues and questions:
1. Check the troubleshooting section
2. Review the logs and events
3. Consult the main AgentSmith-HUB documentation
4. Open an issue in the repository 
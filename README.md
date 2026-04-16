# Hermes Operator

基于 Kubernetes 的 Hermes AI Agent 即服务（Hermes as a Service）平台。

## 项目概述

Hermes Operator 是一个使用 kubebuilder 构建的 Kubernetes Operator，用于在 Kubernetes 集群中部署和管理 Hermes AI Agent。用户可以通过创建 Custom Resource（CR）或调用 REST API 来启动 Hermes Agent 实例，为其提供 AI 服务能力。

## 功能特性

- **Kubernetes 原生**: 完全基于 Kubernetes CRD 和 Controller 模式
- **声明式配置**: 通过 YAML 或 JSON 声明式定义 Agent 实例
- **自动 Pod 管理**: 自动创建、更新、删除 Hermes Agent Pod
- **服务暴露**: 自动创建 ClusterIP Service 暴露 Agent 服务
- **完整状态跟踪**: 实时跟踪 Agent 状态、端点、Pod IP 等信息
- **REST API**: 提供 HTTP REST API 用于 CRUD 操作
- **配置灵活性**: 支持自定义镜像、资源限制、环境变量、挂载卷等

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Hermes Operator                             │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────┐   │
│  │ REST API    │  │ Controller   │  │ CRD: HermesAgent    │   │
│  │ Server      │──▶│ Reconcile    │──▶│                     │   │
│  │ :8090       │  │ Loop         │  │ api/v1              │   │
│  └─────────────┘  └──────────────┘  └─────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                           │
│                                                                  │
│  ┌────────────────────┐    ┌────────────────────┐              │
│  │ Service            │    │ Pod                │              │
│  │ hermes-agent-xxx   │───▶│ hermes-agent-xxx   │              │
│  │ ClusterIP: 8090     │    │ [Hermes Container] │              │
│  └────────────────────┘    └────────────────────┘              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## 快速开始

### 前提条件

- Go 1.21+
- Docker 17.03+
- kubectl 1.11.3+
- Kubernetes 1.11.3+ 集群

### 安装

1. **构建并推送镜像**

```bash
make docker-build docker-push IMG=<your-registry>/hermes-operator:tag
```

2. **安装 CRD 到集群**

```bash
make install
```

3. **部署 Operator**

```bash
make deploy IMG=<your-registry>/hermes-operator:tag
```

### 使用方法

#### 方法一：通过 YAML 创建 HermesAgent

```yaml
apiVersion: core.hermes.io/v1
kind: HermesAgent
metadata:
  name: my-hermes-agent
spec:
  image: ghcr.io/hermes-project/hermes-agent:latest
  servicePort: 8080
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "512Mi"
      cpu: "500m"
  hermesConfig:
    model: "gpt-4"
    maxIterations: 100
```

应用配置：

```bash
kubectl apply -f config/samples/core_v1_hermesagent.yaml
```

查看状态：

```bash
kubectl get hermesagent
kubectl describe hermesagent my-hermes-agent
```

#### 方法二：通过 REST API 创建

启动 Operator 后，REST API 默认监听在 `:8090` 端口。

**创建 HermesAgent：**

```bash
curl -X POST http://localhost:8090/api/v1/hermesagents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-hermes-agent",
    "namespace": "default",
    "image": "ghcr.io/hermes-project/hermes-agent:latest",
    "servicePort": 8080,
    "model": "gpt-4",
    "maxIterations": 100
  }'
```

**查询 HermesAgent：**

```bash
# 查询所有
curl http://localhost:8090/api/v1/hermesagents

# 查询指定实例
curl http://localhost:8090/api/v1/hermesagent/my-hermes-agent?namespace=default
```

**删除 HermesAgent：**

```bash
curl -X DELETE http://localhost:8090/api/v1/hermesagent/my-hermes-agent?namespace=default
```

## API 参考

### 端点

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/hermesagents` | 列出所有 HermesAgent |
| GET | `/api/v1/hermesagent/{name}?namespace=ns` | 获取指定的 HermesAgent |
| POST | `/api/v1/hermesagents` | 创建新的 HermesAgent |
| DELETE | `/api/v1/hermesagent/{name}?namespace=ns` | 删除 HermesAgent |
| PATCH | `/api/v1/hermesagent/{name}?namespace=ns` | 更新 HermesAgent |
| GET | `/healthz` | 健康检查 |

### 响应示例

```json
{
  "name": "my-hermes-agent",
  "namespace": "default",
  "image": "ghcr.io/hermes-project/hermes-agent:latest",
  "servicePort": 8080,
  "phase": "Running",
  "endpoint": "http://hermes-agent-my-hermes-agent.default.svc.cluster.local:8080",
  "podIP": "10.244.1.15",
  "readyReplicas": 1,
  "conditions": [
    {
      "type": "PodCreated",
      "status": "True",
      "reason": "PodCreated",
      "message": "Pod has been created"
    }
  ]
}
```

## HermesAgent CRD 规格

### Spec

| 字段 | 类型 | 描述 | 必填 |
|------|------|------|------|
| `image` | string | Hermes Agent 容器镜像 | 否 |
| `imagePullPolicy` | string | 镜像拉取策略 | 否 |
| `servicePort` | int32 | 服务监听端口 | 否 |
| `resources` | ResourceRequirements | 计算资源限制 | 否 |
| `config` | map[string]string | 自定义配置（环境变量） | 否 |
| `labels` | map[string]string | Pod 标签 | 否 |
| `annotations` | map[string]string | Pod 注解 | 否 |
| `nodeSelector` | map[string]string | 节点选择器 | 否 |
| `tolerations` | []Toleration | 容忍设置 | 否 |
| `affinity` | *Affinity | 亲和性设置 | 否 |
| `volumes` | []Volume | 挂载卷 | 否 |
| `volumeMounts` | []VolumeMount | 卷挂载 | 否 |
| `hermesConfig` | HermesConfigSpec | Hermes Agent 配置 | 否 |

### HermesConfig

| 字段 | 类型 | 描述 |
|------|------|------|
| `model` | string | AI 模型名称 |
| `apiKeySecretRef` | *SecretRef | API Key Secret 引用 |
| `tools` | []string | 启用的工具列表 |
| `maxIterations` | int32 | 最大迭代次数 |
| `systemPrompt` | string | 系统提示词 |

### Status

| 字段 | 类型 | 描述 |
|------|------|------|
| `podName` | string | Pod 名称 |
| `phase` | string | 当前阶段 |
| `serviceName` | string | Service 名称 |
| `servicePort` | int32 | Service 端口 |
| `endpoint` | string | 访问端点 |
| `podIP` | string | Pod IP |
| `readyReplicas` | int32 | 就绪副本数 |
| `conditions` | []Condition | 状态条件 |

## 开发

### 项目结构

```
hermes-operator/
├── api/v1/                    # CRD 定义
│   ├── hermesagent_types.go  # HermesAgent 类型定义
│   └── zz_generated.deepcopy.go
├── cmd/                       # 程序入口
│   └── main.go
├── internal/
│   ├── api/                   # REST API 实现
│   │   └── server.go
│   └── controller/            # Controller 实现
│       └── hermesagent_controller.go
├── config/                    # Kubernetes 配置
│   ├── crd/                  # CRD manifests
│   ├── rbac/                 # RBAC 配置
│   ├── samples/              # 示例 CR
│   └── deployment/           # Deployment 配置
├── examples/                  # API 请求示例
│   └── api-request-example.json
├── hack/                      # 工具脚本
└── Makefile                   # 构建脚本
```

### 本地运行

```bash
# 生成代码
make generate
make manifests

# 本地运行（需要 kubeconfig）
make run

# 运行测试
make test
```

### 构建 Docker 镜像

```bash
make docker-build IMG=<image:tag>
make docker-push IMG=<image:tag>
```

## 环境变量

Hermes Agent Pod 中可用的环境变量：

| 变量名 | 描述 |
|--------|------|
| `HERMES_AGENT_NAME` | Agent 实例名称 |
| `HERMES_AGENT_NAMESPACE` | Agent 命名空间 |
| `HERMES_SERVICE_PORT` | 服务端口 |
| `HERMES_POD_NAME` | Pod 名称 |
| `HERMES_POD_IP` | Pod IP 地址 |
| `HERMES_MODEL` | AI 模型名称 |
| `HERMES_MAX_ITERATIONS` | 最大迭代次数 |
| `HERMES_SYSTEM_PROMPT` | 系统提示词 |
| `HERMES_TOOLS` | 工具列表（JSON 数组） |
| `HERMES_API_KEY` | API Key（从 Secret 读取） |
| `HERMES_CONFIG_*` | 自定义配置 |

## 清理

```bash
# 删除示例
kubectl delete -f config/samples/

# 卸载 Operator
make undeploy

# 删除 CRD
make uninstall
```

## License

Apache License 2.0

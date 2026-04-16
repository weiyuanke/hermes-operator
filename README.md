# Hermes Operator

基于 Kubernetes 的 Hermes AI Agent 即服务（Hermes as a Service）平台。

## 项目概述

Hermes Operator 是一个使用 kubebuilder 构建的 Kubernetes Operator，用于在 Kubernetes 集群中部署和管理 Hermes AI Agent。用户只需提供 **3 个必填字段**（模型、供应商、API Key），即可快速拉起 Hermes Agent 实例。

## 功能特性

- **最小化配置**: 只需 3 个必填字段即可启动 Agent
- **Kubernetes 原生**: 完全基于 Kubernetes CRD 和 Controller 模式
- **声明式配置**: 通过 YAML 或 JSON 声明式定义 Agent 实例
- **自动 Pod 管理**: 自动创建、更新、删除 Hermes Agent Pod
- **服务暴露**: 自动创建 ClusterIP Service 暴露 Agent 服务
- **完整状态跟踪**: 实时跟踪 Agent 状态、端点、Pod IP 等信息
- **REST API**: 提供 HTTP REST API 用于 CRUD 操作

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Hermes Operator                             │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────┐   │
│  │ REST API    │  │ Controller   │  │ CRD: HermesAgent     │   │
│  │ Server      │──▶│ Reconcile   │──▶│                     │   │
│  │ :8090       │  │ Loop        │  │ api/v1              │   │
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
│  │ ClusterIP: 8000    │    │ [Hermes Container] │              │
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

**最小配置示例：**

```yaml
apiVersion: core.hermes.io/v1
kind: HermesAgent
metadata:
  name: my-hermes-agent
spec:
  # 必填：AI 模型名称
  model: kimi-k2.5

  # 必填：模型供应商
  provider: kimi-coding-cn

  # 必填：API Key Secret 引用
  apiSecretRef:
    name: kimi-api-key
```

**1. 首先创建 Secret：**

```bash
kubectl create secret generic kimi-api-key --from-literal=api-key=your-api-key
```

**2. 应用配置：**

```bash
kubectl apply -f config/samples/core_v1_hermesagent.yaml
```

**3. 查看状态：**

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
    "model": "kimi-k2.5",
    "provider": "kimi-coding-cn",
    "apiSecretRef": {"name": "kimi-api-key"}
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
  "model": "kimi-k2.5",
  "provider": "kimi-coding-cn",
  "phase": "Running",
  "endpoint": "http://hermes-agent-my-hermes-agent.default.svc.cluster.local:8000",
  "podIP": "10.244.1.15",
  "serviceName": "hermes-agent-my-hermes-agent",
  "conditions": [
    {
      "type": "Ready",
      "status": "True",
      "reason": "PodReady",
      "message": "Pod is running"
    }
  ]
}
```

## HermesAgent CRD 规格

### Spec

#### 必填字段

| 字段 | 类型 | 描述 |
|------|------|------|
| `model` | string | AI 模型名称 (如 `kimi-k2.5`, `gpt-4`) |
| `provider` | string | 模型供应商 (如 `kimi-coding-cn`, `openai`) |
| `apiSecretRef.name` | string | API Key Secret 名称 |

#### 可选字段

| 字段 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `baseURL` | string | `https://api.moonshot.cn/v1` | API 端点 |
| `maxTurns` | int | `90` | 最大对话轮数 |
| `personality` | string | `kawaii` | Agent 人格预设 |
| `image` | string | `ghcr.io/aisuko/hermes:latest` | 容器镜像 |
| `servicePort` | int | `8000` | 服务监听端口 |
| `resources` | ResourceRequirements | - | 计算资源限制 |

### SecretRef

| 字段 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `name` | string | - | Secret 名称 (必填) |
| `namespace` | string | 同 CR 命名空间 | Secret 所在命名空间 |
| `key` | string | `api-key` | Secret 中的键名 |

### Status

| 字段 | 类型 | 描述 |
|------|------|------|
| `podName` | string | Pod 名称 |
| `phase` | string | 当前阶段 |
| `serviceName` | string | Service 名称 |
| `servicePort` | int | Service 端口 |
| `endpoint` | string | 访问端点 |
| `podIP` | string | Pod IP |
| `startedAt` | time | 启动时间 |
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
│   └── samples/              # 示例 CR
├── Makefile                   # 构建脚本
└── README.md
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
| `HERMES_MODEL` | AI 模型名称 |
| `HERMES_PROVIDER` | 模型供应商 |
| `HERMES_BASE_URL` | API 端点 |
| `HERMES_API_KEY` | API Key（从 Secret 读取） |
| `HERMES_MAX_TURNS` | 最大对话轮数 |
| `HERMES_PERSONALITY` | Agent 人格预设 |
| `HERMES_SERVICE_PORT` | 服务端口 |
| `HERMES_POD_NAME` | Pod 名称 |
| `HERMES_POD_IP` | Pod IP 地址 |
| `HERMES_NAMESPACE` | 命名空间 |

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

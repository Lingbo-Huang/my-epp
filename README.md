# 高级EPP (External Processing Protocol) 演示

这是一个完整的EPP分层架构演示项目，展示了企业级模型推理网关的核心功能和设计模式。

## 🏗️ 架构概览

EPP系统采用六层分层架构设计，每层职责明确，便于维护和扩展：

```
┌─────────────────────────────────────────────────────────────┐
│                    Envoy 请求                               │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                1. Request Processing Layer                  │
│         ExtProcServer → RequestHandler → ResponseHandler    │
│            (接收和解析Envoy请求，构建响应)                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                   2. Routing Layer                         │
│      Director → ModelNameResolver → TrafficSplitter        │
│              (路由决策和流量拆分)                              │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                3. Flow Control Layer                       │
│   FlowController → QueueSystem → SaturationDetector        │
│              (流控、排队、饱和度检测)                           │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                 4. Scheduling Layer                        │
│   SchedulerEngine → FilterPlugins → ScorePlugins           │
│               (端点过滤、评分、选择)                            │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                    5. Data Layer                           │
│   PodRegistry → MetricsCollector → CapabilityManager       │
│             (数据存储和状态管理)                               │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│               6. Backend Interface Layer                   │
│    ModelServerProtocol → LoRAInterface → KVCacheInterface  │
│               (与模型服务的协议适配)                            │
└─────────────────────────────────────────────────────────────┘
```

## 📦 项目结构

```
my-epp/
├── pkg/
│   ├── types/              # 核心数据类型定义
│   ├── interfaces/         # 接口定义
│   ├── request_processing/ # 请求处理层
│   ├── routing/           # 路由层
│   ├── flow_control/      # 流控层
│   ├── scheduling/        # 调度层
│   ├── data/              # 数据层
│   └── backend/           # 后端接口层
├── cmd/
│   ├── simple-epp/        # 简单版本 (simple-epp分支)
│   ├── advanced-epp-server/ # 高级服务端
│   └── advanced-epp-client/ # 演示客户端
└── examples/              # 演示场景
```

## 🚀 快速开始

### 1. 启动高级EPP服务器

```bash
cd my-epp
go run cmd/advanced-epp-server/main.go
```

服务器将在 `:9002` 端口启动，并显示各层初始化信息。

### 2. 运行演示客户端

在另一个终端运行：

```bash
go run cmd/advanced-epp-client/main.go
```

客户端将执行多个演示场景，展示EPP的各项功能。

## 🎭 演示场景

演示客户端包含以下场景：

### 场景1：高优先级LoRA推理
- **模型**: `llama2-7b-lora`
- **特点**: 需要LoRA适配器，高优先级用户
- **演示**: 能力匹配过滤，优先级调度

### 场景2：普通Base模型推理
- **模型**: `llama2-7b-base`
- **特点**: 标准用户，基础模型
- **演示**: 负载均衡调度

### 场景3：流控测试
- **特点**: 快速连续请求
- **演示**: 速率限制，请求排队

### 场景4：调度测试
- **模型**: `llama2-13b`
- **特点**: 多端点负载均衡
- **演示**: 多插件协同评分

### 场景5：LoRA Fallback场景
- **特点**: LoRA需求但端点不支持
- **演示**: 智能fallback机制

## 🔍 核心功能特性

### 1. 智能路由 (Routing)
- **模型名解析**: 支持模糊匹配和版本解析
- **流量拆分**: 支持随机、权重、粘性、金丝雀等策略
- **路由策略**: 基于条件的动态路由规则

### 2. 流量控制 (Flow Control)
- **速率限制**: 支持令牌桶和滑动窗口算法
- **智能排队**: 支持FIFO、LIFO、优先级等队列策略
- **饱和度检测**: 基于CPU、内存、队列长度等指标

### 3. 智能调度 (Scheduling)
- **多阶段过滤**: 健康检查、能力匹配、负载过滤
- **插件化评分**: 负载、延迟、亲和性等评分插件
- **并行处理**: 支持并行评分以提高性能

### 4. 能力感知 (Capability-Aware)
- **LoRA支持检测**: 检查端点是否已加载所需LoRA
- **硬件匹配**: 基于GPU、内存等硬件要求匹配
- **模型版本兼容**: 支持模型版本兼容性检查

## 📊 监控和指标

EPP内置了完整的监控体系：

### 实时指标
- **请求统计**: 总请求数、成功率、失败率
- **性能指标**: 平均延迟、P95/P99延迟
- **资源利用**: CPU、内存、队列使用情况
- **调度效果**: 端点选择分布、调度耗时

### 日志输出示例
```
📈 系统状态报告:
  🚦 流控层: 总请求=150, 接受=142, 拒绝=5, 排队=3
  📥 队列系统: 当前大小=3, 平均等待=1.2s, 超时=0
  🎯 调度引擎: 总调度=142, 成功=142, 失败=0, 平均耗时=15ms
  💻 系统指标: CPU=45.2%, 内存=67.8%, 队列=3
```

## 🛠️ 扩展性设计

### 插件化架构
- **过滤插件**: 健康检查、能力匹配、负载、可用区等
- **评分插件**: 负载、延迟、亲和性、吞吐量等
- **绑定插件**: 粘性会话、亲和性绑定等

### 策略配置
- **路由策略**: 支持复杂的条件匹配和规则组合
- **流控策略**: 支持多维度限流和自定义策略
- **调度策略**: 支持灵活的插件组合和权重配置

## 🆚 版本对比

| 特性 | Simple EPP | Advanced EPP |
|------|------------|--------------|
| 架构复杂度 | 单层简单实现 | 六层分层架构 |
| 端点选择 | 基础负载比较 | 多插件智能调度 |
| 流控能力 | 无 | 完整流控体系 |
| 路由策略 | 无 | 策略化路由 |
| 监控指标 | 基础日志 | 完整监控体系 |
| 扩展性 | 有限 | 高度可扩展 |
| 生产就绪 | 演示级别 | 接近生产级别 |

## 🎯 适用场景

### 1. 模型推理网关
- 统一多个模型服务的访问入口
- 智能负载均衡和故障转移
- 基于模型特征的智能路由

### 2. LoRA模型管理
- LoRA适配器的感知和匹配
- 动态LoRA加载优化
- LoRA专用端点路由

### 3. 多租户推理服务
- 基于用户/租户的流控
- 资源隔离和优先级调度
- 用户级别的指标和监控

### 4. 高可用推理集群
- 健康检查和自动故障切换
- 饱和度检测和过载保护
- 优雅降级和容错处理

## 🔧 配置示例

### 路由策略配置
```go
policy := &types.RoutingPolicy{
    Name: "lora-routing",
    Enabled: true,
    Rules: []types.RoutingRule{
        {
            Name: "lora-models",
            Conditions: []types.PolicyCondition{
                {Field: "lora_required", Operator: "eq", Value: true},
            },
            Target: "lora-endpoints",
        },
    },
}
```

### 流控策略配置
```go
policy := &types.FlowControlPolicy{
    Name: "basic-flow-control", 
    RateLimit: &types.RateLimitConfig{
        MaxRequests: 100,
        TimeWindow: time.Minute,
        BurstSize: 20,
    },
    QueueConfig: &types.QueueConfig{
        MaxSize: 50,
        Strategy: types.QueueStrategyPriority,
    },
}
```

## 📚 技术栈

- **语言**: Go 1.19+
- **协议**: gRPC, Envoy External Processing
- **架构**: 分层架构, 插件化设计
- **并发**: Goroutines, Channels
- **监控**: 内置指标收集和上报

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证。详见 `LICENSE` 文件。

## 👥 作者

- **Lingbo Huang** - *初始开发* - AI Assistant

---

**注意**: 这是一个演示项目，展示EPP架构设计和核心概念。在生产环境使用前，需要进一步完善错误处理、性能优化、安全加固等方面。

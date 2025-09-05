package interfaces

import (
	"context"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// RequestProcessor 请求处理接口
type RequestProcessor interface {
	// ProcessRequest 处理请求的主要入口
	ProcessRequest(ctx context.Context, req *types.ProcessingRequest) (*types.ProcessingResponse, error)
}

// RequestHandler 请求解析处理器
type RequestHandler interface {
	// ParseRequest 解析原始请求为ProcessingRequest
	ParseRequest(ctx context.Context, rawReq interface{}) (*types.ProcessingRequest, error)

	// ValidateRequest 验证请求格式和内容
	ValidateRequest(req *types.ProcessingRequest) error
}

// ResponseHandler 响应处理器
type ResponseHandler interface {
	// BuildResponse 构建响应
	BuildResponse(ctx context.Context, req *types.ProcessingRequest, result interface{}) (*types.ProcessingResponse, error)

	// FormatResponse 格式化响应为特定协议格式
	FormatResponse(resp *types.ProcessingResponse) (interface{}, error)
}

// Router 路由器接口
type Router interface {
	// Route 执行路由决策
	Route(ctx context.Context, req *types.ProcessingRequest) (*types.RoutingDecision, error)

	// UpdatePolicy 更新路由策略
	UpdatePolicy(policy *types.RoutingPolicy) error
}

// ModelNameResolver 模型名解析器
type ModelNameResolver interface {
	// ResolveModelName 解析模型名称和版本
	ResolveModelName(modelName string) (resolvedName, version string, err error)

	// GetModelCapabilities 获取模型能力要求
	GetModelCapabilities(modelName string) (*types.EndpointCapability, error)
}

// TrafficSplitter 流量拆分器
type TrafficSplitter interface {
	// SplitTraffic 根据策略拆分流量
	SplitTraffic(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error)
}

// FlowController 流控器接口
type FlowController interface {
	// ShouldAllow 判断是否允许请求通过
	ShouldAllow(ctx context.Context, req *types.ProcessingRequest) (*types.FlowControlInfo, error)

	// UpdatePolicy 更新流控策略
	UpdatePolicy(policy *types.FlowControlPolicy) error
}

// QueueSystem 队列系统接口
type QueueSystem interface {
	// Enqueue 将请求加入队列
	Enqueue(req *types.ProcessingRequest) error

	// Dequeue 从队列中取出请求
	Dequeue() (*types.ProcessingRequest, error)

	// Size 获取队列大小
	Size() int

	// EstimatedWaitTime 估算等待时间
	EstimatedWaitTime() time.Duration
}

// SaturationDetector 饱和度检测器
type SaturationDetector interface {
	// CheckSaturation 检查系统饱和度
	CheckSaturation(ctx context.Context) (bool, map[string]float64, error)

	// UpdateThresholds 更新饱和度阈值
	UpdateThresholds(config *types.SaturationConfig) error
}

// Scheduler 调度器接口
type Scheduler interface {
	// Schedule 调度请求到最优端点
	Schedule(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) (*types.EndpointInfo, error)

	// UpdatePolicy 更新调度策略
	UpdatePolicy(policy *types.SchedulingPolicy) error
}

// FilterPlugin 过滤插件接口
type FilterPlugin interface {
	// Name 插件名称
	Name() string

	// Filter 过滤端点
	Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error)
}

// ScorePlugin 评分插件接口
type ScorePlugin interface {
	// Name 插件名称
	Name() string

	// Score 为端点评分
	Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error)
}

// BindPlugin 绑定插件接口
type BindPlugin interface {
	// Name 插件名称
	Name() string

	// Bind 绑定请求到端点
	Bind(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) error
}

// PodRegistry Pod注册表接口
type PodRegistry interface {
	// RegisterPod 注册Pod
	RegisterPod(pod *types.PodInfo) error

	// UnregisterPod 注销Pod
	UnregisterPod(podID string) error

	// GetPod 获取Pod信息
	GetPod(podID string) (*types.PodInfo, error)

	// ListPods 列出所有Pod
	ListPods() ([]*types.PodInfo, error)

	// ListEndpoints 列出所有端点
	ListEndpoints() ([]*types.EndpointInfo, error)

	// ListEndpointsByLabels 根据标签筛选端点
	ListEndpointsByLabels(labels map[string]string) ([]*types.EndpointInfo, error)
}

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	// CollectEndpointMetrics 收集端点指标
	CollectEndpointMetrics(endpointID string) (*types.EndpointMetrics, error)

	// UpdateEndpointMetrics 更新端点指标
	UpdateEndpointMetrics(endpointID string, metrics *types.EndpointMetrics) error

	// GetSystemMetrics 获取系统级指标
	GetSystemMetrics() (map[string]interface{}, error)
}

// CapabilityManager 能力管理器接口
type CapabilityManager interface {
	// UpdateCapability 更新端点能力
	UpdateCapability(endpointID string, capability *types.EndpointCapability) error

	// GetCapability 获取端点能力
	GetCapability(endpointID string) (*types.EndpointCapability, error)

	// QueryByCapability 根据能力要求查询端点
	QueryByCapability(required *types.EndpointCapability) ([]*types.EndpointInfo, error)
}

// EndpointManager 端点管理器接口
type EndpointManager interface {
	// UpdateEndpoint 更新端点信息
	UpdateEndpoint(endpoint *types.EndpointInfo) error

	// GetEndpoint 获取端点信息
	GetEndpoint(endpointID string) (*types.EndpointInfo, error)

	// ListHealthyEndpoints 列出健康的端点
	ListHealthyEndpoints() ([]*types.EndpointInfo, error)

	// CheckHealth 检查端点健康状态
	CheckHealth(endpointID string) (*types.HealthStatus, error)
}

// ModelServerProtocol 模型服务协议接口
type ModelServerProtocol interface {
	// SendRequest 发送请求到模型服务
	SendRequest(ctx context.Context, endpoint *types.EndpointInfo, req *types.ProcessingRequest) (interface{}, error)

	// GetProtocolType 获取协议类型
	GetProtocolType() string

	// IsHealthy 检查服务健康状态
	IsHealthy(endpoint *types.EndpointInfo) bool
}

// KVCacheInterface KV缓存接口
type KVCacheInterface interface {
	// GetCacheStatus 获取缓存状态
	GetCacheStatus(endpointID string) (map[string]interface{}, error)

	// InvalidateCache 失效缓存
	InvalidateCache(endpointID string, keys []string) error

	// GetCacheHitRate 获取缓存命中率
	GetCacheHitRate(endpointID string) (float64, error)
}

// LoRAInterface LoRA适配器接口
type LoRAInterface interface {
	// LoadLoRA 加载LoRA适配器
	LoadLoRA(endpointID string, loraName string) error

	// UnloadLoRA 卸载LoRA适配器
	UnloadLoRA(endpointID string, loraName string) error

	// GetLoadedLoRAs 获取已加载的LoRA列表
	GetLoadedLoRAs(endpointID string) ([]string, error)

	// IsLoRALoaded 检查LoRA是否已加载
	IsLoRALoaded(endpointID string, loraName string) (bool, error)
}

// EventBus 事件总线接口
type EventBus interface {
	// Publish 发布事件
	Publish(topic string, event interface{}) error

	// Subscribe 订阅事件
	Subscribe(topic string, handler func(interface{})) error

	// Unsubscribe 取消订阅
	Unsubscribe(topic string) error
}

// MetricsReporter 指标上报接口
type MetricsReporter interface {
	// ReportCounter 上报计数器指标
	ReportCounter(name string, value int64, labels map[string]string) error

	// ReportGauge 上报仪表盘指标
	ReportGauge(name string, value float64, labels map[string]string) error

	// ReportHistogram 上报直方图指标
	ReportHistogram(name string, value float64, labels map[string]string) error
}

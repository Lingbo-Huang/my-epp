package types

import (
	"time"
)

// EndpointInfo 端点信息
type EndpointInfo struct {
	// 基本信息
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // http, grpc

	// 标签和能力
	Labels       map[string]string   `json:"labels"`
	Capabilities *EndpointCapability `json:"capabilities"`

	// 状态信息
	Status  EndpointStatus   `json:"status"`
	Health  *HealthStatus    `json:"health"`
	Metrics *EndpointMetrics `json:"metrics"`

	// 时间戳
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// EndpointStatus 端点状态
type EndpointStatus int

const (
	EndpointStatusUnknown EndpointStatus = iota
	EndpointStatusHealthy
	EndpointStatusUnhealthy
	EndpointStatusDraining
	EndpointStatusMaintenance
)

func (s EndpointStatus) String() string {
	switch s {
	case EndpointStatusHealthy:
		return "healthy"
	case EndpointStatusUnhealthy:
		return "unhealthy"
	case EndpointStatusDraining:
		return "draining"
	case EndpointStatusMaintenance:
		return "maintenance"
	default:
		return "unknown"
	}
}

// EndpointCapability 端点能力
type EndpointCapability struct {
	// 模型能力
	SupportedModels []string `json:"supported_models"`
	LoRASupport     bool     `json:"lora_support"`
	LoadedLoRAs     []string `json:"loaded_loras"`
	KVCacheSupport  bool     `json:"kv_cache_support"`

	// 硬件能力
	GPUCount       int   `json:"gpu_count"`
	GPUMemoryTotal int64 `json:"gpu_memory_total"` // bytes
	GPUMemoryFree  int64 `json:"gpu_memory_free"`  // bytes
	CPUCores       int   `json:"cpu_cores"`
	RAMTotal       int64 `json:"ram_total"` // bytes
	RAMFree        int64 `json:"ram_free"`  // bytes

	// 性能能力
	MaxConcurrent   int           `json:"max_concurrent"`
	MaxQueueSize    int           `json:"max_queue_size"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	Throughput      float64       `json:"throughput"` // requests/second
}

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy          bool              `json:"healthy"`
	LastCheck        time.Time         `json:"last_check"`
	CheckInterval    time.Duration     `json:"check_interval"`
	FailureCount     int               `json:"failure_count"`
	ConsecutiveFails int               `json:"consecutive_fails"`
	Details          map[string]string `json:"details"`
}

// EndpointMetrics 端点指标
type EndpointMetrics struct {
	// 请求指标
	RequestsTotal     int64   `json:"requests_total"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	ActiveRequests    int     `json:"active_requests"`
	QueuedRequests    int     `json:"queued_requests"`

	// 性能指标
	AvgLatency time.Duration `json:"avg_latency"`
	P95Latency time.Duration `json:"p95_latency"`
	P99Latency time.Duration `json:"p99_latency"`
	ErrorRate  float64       `json:"error_rate"`

	// 资源利用率
	CPUUsage    float64 `json:"cpu_usage"`    // 0.0 - 1.0
	GPUUsage    float64 `json:"gpu_usage"`    // 0.0 - 1.0
	MemoryUsage float64 `json:"memory_usage"` // 0.0 - 1.0

	// 业务指标
	ModelLoadTime time.Duration `json:"model_load_time"`
	LoRALoadTime  time.Duration `json:"lora_load_time"`
	CacheHitRate  float64       `json:"cache_hit_rate"`

	// 时间戳
	LastUpdated time.Time `json:"last_updated"`
}

// PodInfo Kubernetes Pod信息
type PodInfo struct {
	// K8s 元数据
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	UID         string            `json:"uid"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`

	// Pod 状态
	Phase    string `json:"phase"`
	PodIP    string `json:"pod_ip"`
	NodeName string `json:"node_name"`

	// 端点映射
	Endpoints []*EndpointInfo `json:"endpoints"`

	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SchedulingScore 调度评分
type SchedulingScore struct {
	EndpointID     string             `json:"endpoint_id"`
	TotalScore     float64            `json:"total_score"`
	ScoreBreakdown map[string]float64 `json:"score_breakdown"`
	Reasons        []string           `json:"reasons"`
	Excluded       bool               `json:"excluded"`
	ExcludeReason  string             `json:"exclude_reason,omitempty"`
}

package types

import (
	"time"
)

// FlowControlPolicy 流控策略
type FlowControlPolicy struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// 限流配置
	RateLimit *RateLimitConfig `json:"rate_limit,omitempty"`

	// 排队配置
	QueueConfig *QueueConfig `json:"queue_config,omitempty"`

	// 饱和度检测
	Saturation *SaturationConfig `json:"saturation,omitempty"`

	// 优先级配置
	Priority *PriorityConfig `json:"priority,omitempty"`

	// 条件匹配
	Conditions []PolicyCondition `json:"conditions,omitempty"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	MaxRequests int           `json:"max_requests"`
	TimeWindow  time.Duration `json:"time_window"`
	BurstSize   int           `json:"burst_size,omitempty"`

	// 分组限流
	GroupBy []string `json:"group_by,omitempty"` // user, model, endpoint等
}

// QueueConfig 排队配置
type QueueConfig struct {
	MaxSize     int           `json:"max_size"`
	MaxWaitTime time.Duration `json:"max_wait_time"`

	// 排队策略
	Strategy QueueStrategy `json:"strategy"`

	// 优先级队列
	PriorityLevels int `json:"priority_levels,omitempty"`
}

// QueueStrategy 排队策略
type QueueStrategy int

const (
	QueueStrategyFIFO QueueStrategy = iota
	QueueStrategyLIFO
	QueueStrategyPriority
	QueueStrategyShortest
)

func (s QueueStrategy) String() string {
	switch s {
	case QueueStrategyFIFO:
		return "fifo"
	case QueueStrategyLIFO:
		return "lifo"
	case QueueStrategyPriority:
		return "priority"
	case QueueStrategyShortest:
		return "shortest"
	default:
		return "unknown"
	}
}

// SaturationConfig 饱和度检测配置
type SaturationConfig struct {
	// CPU阈值
	CPUThreshold float64 `json:"cpu_threshold"`

	// GPU阈值
	GPUThreshold float64 `json:"gpu_threshold"`

	// 内存阈值
	MemoryThreshold float64 `json:"memory_threshold"`

	// 队列长度阈值
	QueueThreshold int `json:"queue_threshold"`

	// 响应时间阈值
	LatencyThreshold time.Duration `json:"latency_threshold"`

	// 错误率阈值
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// 检测窗口
	CheckInterval time.Duration `json:"check_interval"`
	SampleWindow  time.Duration `json:"sample_window"`
}

// PriorityConfig 优先级配置
type PriorityConfig struct {
	Levels       []PriorityLevel `json:"levels"`
	DefaultLevel int             `json:"default_level"`
}

// PriorityLevel 优先级级别
type PriorityLevel struct {
	Level         int               `json:"level"`
	Name          string            `json:"name"`
	Weight        float64           `json:"weight"`
	MaxConcurrent int               `json:"max_concurrent,omitempty"`
	Conditions    []PolicyCondition `json:"conditions"`
}

// PolicyCondition 策略条件
type PolicyCondition struct {
	Field    string        `json:"field"`    // model_name, user_id, endpoint等
	Operator string        `json:"operator"` // eq, ne, in, regex等
	Value    interface{}   `json:"value"`
	Values   []interface{} `json:"values,omitempty"`
}

// RoutingPolicy 路由策略
type RoutingPolicy struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// 路由规则
	Rules []RoutingRule `json:"rules"`

	// 默认行为
	DefaultAction string `json:"default_action"` // route, reject, queue
	DefaultTarget string `json:"default_target,omitempty"`

	// 流量拆分
	TrafficSplit map[string]float64 `json:"traffic_split,omitempty"`
}

// RoutingRule 路由规则
type RoutingRule struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`

	// 匹配条件
	Conditions []PolicyCondition `json:"conditions"`

	// 路由目标
	Target       string            `json:"target"` // endpoint group, specific endpoint
	TargetLabels map[string]string `json:"target_labels,omitempty"`

	// 流量权重
	Weight float64 `json:"weight,omitempty"`
}

// SchedulingPolicy 调度策略
type SchedulingPolicy struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// 调度算法
	Algorithm SchedulingAlgorithm `json:"algorithm"`

	// 过滤器配置
	Filters []FilterConfig `json:"filters"`

	// 评分器配置
	Scorers []ScorerConfig `json:"scorers"`

	// 绑定器配置
	Binders []BinderConfig `json:"binders,omitempty"`
}

// SchedulingAlgorithm 调度算法
type SchedulingAlgorithm int

const (
	SchedulingAlgorithmRoundRobin SchedulingAlgorithm = iota
	SchedulingAlgorithmWeightedRoundRobin
	SchedulingAlgorithmLeastConnections
	SchedulingAlgorithmConsistentHash
	SchedulingAlgorithmCustom
)

func (s SchedulingAlgorithm) String() string {
	switch s {
	case SchedulingAlgorithmRoundRobin:
		return "round_robin"
	case SchedulingAlgorithmWeightedRoundRobin:
		return "weighted_round_robin"
	case SchedulingAlgorithmLeastConnections:
		return "least_connections"
	case SchedulingAlgorithmConsistentHash:
		return "consistent_hash"
	case SchedulingAlgorithmCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// FilterConfig 过滤器配置
type FilterConfig struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // health, capability, load等
	Enabled    bool                   `json:"enabled"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ScorerConfig 评分器配置
type ScorerConfig struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // load, latency, capability等
	Weight     float64                `json:"weight"`
	Enabled    bool                   `json:"enabled"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// BinderConfig 绑定器配置
type BinderConfig struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // sticky, affinity等
	Enabled    bool                   `json:"enabled"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

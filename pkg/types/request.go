package types

import (
	"context"
	"time"
)

// ProcessingRequest 代表一个经过解析的推理请求
type ProcessingRequest struct {
	// 请求ID，用于跟踪和日志
	ID string `json:"id"`

	// 模型相关信息
	ModelName    string `json:"model_name"`
	ModelVersion string `json:"model_version,omitempty"`
	LoRARequired bool   `json:"lora_required"`
	LoRAAdapter  string `json:"lora_adapter,omitempty"`

	// 推理参数
	Parameters map[string]interface{} `json:"parameters"`

	// 请求载荷
	Prompt   string                 `json:"prompt"`
	Headers  map[string]string      `json:"headers"`
	Metadata map[string]interface{} `json:"metadata"`

	// 流控相关
	Priority    int           `json:"priority"`
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"max_retries"`
	QueuedTime  time.Time     `json:"queued_time"`
	ProcessTime time.Time     `json:"process_time"`

	// 上下文
	Context context.Context `json:"-"`
}

// ProcessingResponse 代表处理后的响应
type ProcessingResponse struct {
	RequestID        string            `json:"request_id"`
	SelectedEndpoint *EndpointInfo     `json:"selected_endpoint"`
	RoutingDecision  *RoutingDecision  `json:"routing_decision"`
	FlowControlInfo  *FlowControlInfo  `json:"flow_control_info"`
	ProcessingTime   time.Duration     `json:"processing_time"`
	Headers          map[string]string `json:"headers"`
	Status           ResponseStatus    `json:"status"`
	Error            string            `json:"error,omitempty"`
}

// ResponseStatus 响应状态枚举
type ResponseStatus int

const (
	StatusSuccess ResponseStatus = iota
	StatusRejected
	StatusQueued
	StatusFailed
)

func (s ResponseStatus) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusRejected:
		return "rejected"
	case StatusQueued:
		return "queued"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// RoutingDecision 路由决策信息
type RoutingDecision struct {
	Strategy     string             `json:"strategy"`
	Rules        []string           `json:"rules"`
	TrafficSplit map[string]float64 `json:"traffic_split,omitempty"`
	Reason       string             `json:"reason"`
}

// FlowControlInfo 流控信息
type FlowControlInfo struct {
	Allowed       bool          `json:"allowed"`
	QueuePosition int           `json:"queue_position,omitempty"`
	EstimatedWait time.Duration `json:"estimated_wait,omitempty"`
	Reason        string        `json:"reason"`
}

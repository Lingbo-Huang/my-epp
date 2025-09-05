package flow_control

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/interfaces"
	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// FlowController 流控器实现
type FlowController struct {
	// 依赖组件
	queueSystem        interfaces.QueueSystem
	saturationDetector interfaces.SaturationDetector

	// 策略配置
	policies     []*types.FlowControlPolicy
	policiesLock sync.RWMutex

	// 流控状态
	state     *FlowControlState
	stateLock sync.RWMutex

	// 配置
	config *FlowControlConfig
}

// FlowControlConfig 流控器配置
type FlowControlConfig struct {
	// 默认策略
	EnableRateLimit  bool `json:"enable_rate_limit"`
	DefaultRateLimit int  `json:"default_rate_limit"` // requests per second
	DefaultBurstSize int  `json:"default_burst_size"`

	// 队列配置
	EnableQueueing     bool          `json:"enable_queueing"`
	DefaultQueueSize   int           `json:"default_queue_size"`
	DefaultMaxWaitTime time.Duration `json:"default_max_wait_time"`

	// 饱和度检测
	EnableSaturationCheck   bool          `json:"enable_saturation_check"`
	SaturationCheckInterval time.Duration `json:"saturation_check_interval"`

	// 优先级
	EnablePriority bool `json:"enable_priority"`
}

// FlowControlState 流控状态
type FlowControlState struct {
	// 请求计数器
	RequestCounters map[string]*RateLimiter `json:"request_counters"`

	// 系统状态
	SystemSaturated   bool      `json:"system_saturated"`
	SaturationReasons []string  `json:"saturation_reasons"`
	LastCheckTime     time.Time `json:"last_check_time"`

	// 统计信息
	TotalRequests    int64 `json:"total_requests"`
	AcceptedRequests int64 `json:"accepted_requests"`
	RejectedRequests int64 `json:"rejected_requests"`
	QueuedRequests   int64 `json:"queued_requests"`
}

// RateLimiter 速率限制器
type RateLimiter struct {
	MaxRequests int           `json:"max_requests"`
	TimeWindow  time.Duration `json:"time_window"`
	BurstSize   int           `json:"burst_size"`

	// 当前状态
	CurrentCount int       `json:"current_count"`
	WindowStart  time.Time `json:"window_start"`
	Tokens       int       `json:"tokens"`
}

// NewFlowController 创建新的流控器
func NewFlowController(
	queueSystem interfaces.QueueSystem,
	saturationDetector interfaces.SaturationDetector,
	config *FlowControlConfig,
) *FlowController {
	if config == nil {
		config = &FlowControlConfig{
			EnableRateLimit:         true,
			DefaultRateLimit:        100,
			DefaultBurstSize:        10,
			EnableQueueing:          true,
			DefaultQueueSize:        100,
			DefaultMaxWaitTime:      30 * time.Second,
			EnableSaturationCheck:   true,
			SaturationCheckInterval: 5 * time.Second,
			EnablePriority:          true,
		}
	}

	fc := &FlowController{
		queueSystem:        queueSystem,
		saturationDetector: saturationDetector,
		config:             config,
		policies:           make([]*types.FlowControlPolicy, 0),
		state: &FlowControlState{
			RequestCounters: make(map[string]*RateLimiter),
		},
	}

	// 启动后台任务
	go fc.backgroundTasks()

	return fc
}

// ShouldAllow 判断是否允许请求通过
func (fc *FlowController) ShouldAllow(ctx context.Context, req *types.ProcessingRequest) (*types.FlowControlInfo, error) {
	fc.stateLock.Lock()
	fc.state.TotalRequests++
	fc.stateLock.Unlock()

	log.Printf("流控检查开始 - 请求ID: %s, 模型: %s, 优先级: %d",
		req.ID, req.ModelName, req.Priority)

	// 1. 检查系统饱和度
	if fc.config.EnableSaturationCheck {
		if saturated, reasons := fc.checkSaturation(); saturated {
			log.Printf("系统饱和度检查失败 - 原因: %v", reasons)

			// 高优先级请求可能允许通过
			if req.Priority >= 8 {
				log.Printf("高优先级请求通过饱和度检查 - 优先级: %d", req.Priority)
			} else {
				fc.stateLock.Lock()
				fc.state.RejectedRequests++
				fc.stateLock.Unlock()

				return &types.FlowControlInfo{
					Allowed: false,
					Reason:  "系统饱和度过高: " + reasons[0],
				}, nil
			}
		}
	}

	// 2. 检查速率限制
	if fc.config.EnableRateLimit {
		if allowed, reason := fc.checkRateLimit(req); !allowed {
			log.Printf("速率限制检查失败 - 原因: %s", reason)

			fc.stateLock.Lock()
			fc.state.RejectedRequests++
			fc.stateLock.Unlock()

			return &types.FlowControlInfo{
				Allowed: false,
				Reason:  reason,
			}, nil
		}
	}

	// 3. 检查是否需要排队
	if fc.config.EnableQueueing {
		if shouldQueue, queueInfo := fc.checkQueueing(req); shouldQueue {
			log.Printf("请求需要排队 - 位置: %d, 预估等待: %v",
				queueInfo.QueuePosition, queueInfo.EstimatedWait)

			fc.stateLock.Lock()
			fc.state.QueuedRequests++
			fc.stateLock.Unlock()

			return queueInfo, nil
		}
	}

	// 4. 应用策略
	if policyInfo, applied := fc.applyPolicies(req); applied {
		if !policyInfo.Allowed {
			log.Printf("策略检查失败 - 原因: %s", policyInfo.Reason)

			fc.stateLock.Lock()
			fc.state.RejectedRequests++
			fc.stateLock.Unlock()

			return policyInfo, nil
		}
	}

	// 5. 通过所有检查
	fc.stateLock.Lock()
	fc.state.AcceptedRequests++
	fc.stateLock.Unlock()

	log.Printf("流控检查通过 - 请求ID: %s", req.ID)

	return &types.FlowControlInfo{
		Allowed: true,
		Reason:  "通过所有流控检查",
	}, nil
}

// UpdatePolicy 更新流控策略
func (fc *FlowController) UpdatePolicy(policy *types.FlowControlPolicy) error {
	fc.policiesLock.Lock()
	defer fc.policiesLock.Unlock()

	// 查找是否存在同名策略
	for i, p := range fc.policies {
		if p.Name == policy.Name {
			fc.policies[i] = policy
			log.Printf("更新流控策略: %s", policy.Name)
			return nil
		}
	}

	// 添加新策略
	fc.policies = append(fc.policies, policy)
	log.Printf("添加新流控策略: %s", policy.Name)

	return nil
}

// checkSaturation 检查系统饱和度
func (fc *FlowController) checkSaturation() (bool, []string) {
	if fc.saturationDetector == nil {
		return false, nil
	}

	saturated, metrics, err := fc.saturationDetector.CheckSaturation(context.Background())
	if err != nil {
		log.Printf("饱和度检查失败: %v", err)
		return false, nil
	}

	if saturated {
		var reasons []string
		for metric, value := range metrics {
			reasons = append(reasons, fmt.Sprintf("%s: %.2f", metric, value))
		}

		fc.stateLock.Lock()
		fc.state.SystemSaturated = true
		fc.state.SaturationReasons = reasons
		fc.state.LastCheckTime = time.Now()
		fc.stateLock.Unlock()

		return true, reasons
	}

	fc.stateLock.Lock()
	fc.state.SystemSaturated = false
	fc.state.SaturationReasons = nil
	fc.state.LastCheckTime = time.Now()
	fc.stateLock.Unlock()

	return false, nil
}

// checkRateLimit 检查速率限制
func (fc *FlowController) checkRateLimit(req *types.ProcessingRequest) (bool, string) {
	// 根据用户、租户等维度进行限流
	key := fc.buildRateLimitKey(req)

	fc.stateLock.Lock()
	limiter, exists := fc.state.RequestCounters[key]
	if !exists {
		// 创建新的限流器
		limiter = &RateLimiter{
			MaxRequests: fc.config.DefaultRateLimit,
			TimeWindow:  time.Second,
			BurstSize:   fc.config.DefaultBurstSize,
			Tokens:      fc.config.DefaultBurstSize,
			WindowStart: time.Now(),
		}
		fc.state.RequestCounters[key] = limiter
	}
	fc.stateLock.Unlock()

	// 检查是否允许通过
	now := time.Now()

	// 重置时间窗口
	if now.Sub(limiter.WindowStart) >= limiter.TimeWindow {
		limiter.CurrentCount = 0
		limiter.WindowStart = now
		limiter.Tokens = limiter.BurstSize
	}

	// 检查令牌桶
	if limiter.Tokens > 0 {
		limiter.Tokens--
		limiter.CurrentCount++
		return true, ""
	}

	// 检查时间窗口限制
	if limiter.CurrentCount >= limiter.MaxRequests {
		return false, fmt.Sprintf("超过速率限制: %d requests/%v",
			limiter.MaxRequests, limiter.TimeWindow)
	}

	limiter.CurrentCount++
	return true, ""
}

// checkQueueing 检查是否需要排队
func (fc *FlowController) checkQueueing(req *types.ProcessingRequest) (bool, *types.FlowControlInfo) {
	if fc.queueSystem == nil {
		return false, nil
	}

	// 获取当前队列状态
	queueSize := fc.queueSystem.Size()
	estimatedWait := fc.queueSystem.EstimatedWaitTime()

	// 检查是否应该排队（简化逻辑）
	if queueSize < fc.config.DefaultQueueSize && estimatedWait < fc.config.DefaultMaxWaitTime {
		// 高优先级请求不排队
		if req.Priority >= 7 {
			return false, nil
		}

		// 需要排队
		err := fc.queueSystem.Enqueue(req)
		if err != nil {
			log.Printf("入队失败: %v", err)
			return false, &types.FlowControlInfo{
				Allowed: false,
				Reason:  "队列已满",
			}
		}

		return true, &types.FlowControlInfo{
			Allowed:       false,
			QueuePosition: queueSize + 1,
			EstimatedWait: estimatedWait,
			Reason:        "请求已加入队列",
		}
	}

	// 队列已满或等待时间过长
	return false, &types.FlowControlInfo{
		Allowed: false,
		Reason:  "队列已满或等待时间过长",
	}
}

// applyPolicies 应用流控策略
func (fc *FlowController) applyPolicies(req *types.ProcessingRequest) (*types.FlowControlInfo, bool) {
	fc.policiesLock.RLock()
	defer fc.policiesLock.RUnlock()

	for _, policy := range fc.policies {
		if !policy.Enabled {
			continue
		}

		// 检查策略条件
		if matches, info := fc.checkPolicyConditions(policy, req); matches {
			log.Printf("应用流控策略: %s", policy.Name)
			return info, true
		}
	}

	return nil, false
}

// checkPolicyConditions 检查策略条件
func (fc *FlowController) checkPolicyConditions(policy *types.FlowControlPolicy, req *types.ProcessingRequest) (bool, *types.FlowControlInfo) {
	// 检查条件匹配（简化实现）
	for _, condition := range policy.Conditions {
		if !fc.conditionMatches(&condition, req) {
			return false, nil
		}
	}

	// 如果有速率限制配置
	if policy.RateLimit != nil {
		// 应用特定的速率限制
		// 这里简化实现
		return true, &types.FlowControlInfo{
			Allowed: true,
			Reason:  "策略允许通过",
		}
	}

	return false, nil
}

// conditionMatches 检查单个条件是否匹配
func (fc *FlowController) conditionMatches(condition *types.PolicyCondition, req *types.ProcessingRequest) bool {
	// 简化的条件匹配实现
	fieldValue := fc.getFieldValue(condition.Field, req)

	switch condition.Operator {
	case "eq":
		return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", condition.Value)
	case "ne":
		return fmt.Sprintf("%v", fieldValue) != fmt.Sprintf("%v", condition.Value)
	default:
		return false
	}
}

// getFieldValue 获取请求字段值
func (fc *FlowController) getFieldValue(field string, req *types.ProcessingRequest) interface{} {
	switch field {
	case "model_name":
		return req.ModelName
	case "priority":
		return req.Priority
	case "user_id":
		return req.Headers["x-user-id"]
	default:
		return nil
	}
}

// buildRateLimitKey 构建速率限制键
func (fc *FlowController) buildRateLimitKey(req *types.ProcessingRequest) string {
	// 可以根据用户ID、租户ID、模型名等构建键
	if userID := req.Headers["x-user-id"]; userID != "" {
		return fmt.Sprintf("user:%s", userID)
	}

	if tenantID := req.Headers["x-tenant-id"]; tenantID != "" {
		return fmt.Sprintf("tenant:%s", tenantID)
	}

	return fmt.Sprintf("model:%s", req.ModelName)
}

// backgroundTasks 后台任务
func (fc *FlowController) backgroundTasks() {
	ticker := time.NewTicker(fc.config.SaturationCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		// 定期检查系统饱和度
		if fc.config.EnableSaturationCheck {
			fc.checkSaturation()
		}

		// 清理过期的限流器
		fc.cleanupRateLimiters()
	}
}

// cleanupRateLimiters 清理过期的限流器
func (fc *FlowController) cleanupRateLimiters() {
	fc.stateLock.Lock()
	defer fc.stateLock.Unlock()

	now := time.Now()
	for key, limiter := range fc.state.RequestCounters {
		// 如果限流器超过5分钟未使用，则删除
		if now.Sub(limiter.WindowStart) > 5*time.Minute {
			delete(fc.state.RequestCounters, key)
		}
	}
}

// GetState 获取流控状态
func (fc *FlowController) GetState() *FlowControlState {
	fc.stateLock.RLock()
	defer fc.stateLock.RUnlock()

	// 返回状态副本
	stateCopy := *fc.state
	return &stateCopy
}

package scheduling

import (
	"context"
	"math"
	"strings"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// LoadScorer 负载评分插件
type LoadScorer struct{}

// NewLoadScorer 创建负载评分插件
func NewLoadScorer() *LoadScorer {
	return &LoadScorer{}
}

// Name 返回插件名称
func (s *LoadScorer) Name() string {
	return "load"
}

// Score 基于负载为端点评分
func (s *LoadScorer) Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error) {
	score := &types.SchedulingScore{
		EndpointID:     endpoint.ID,
		ScoreBreakdown: make(map[string]float64),
	}

	if endpoint.Metrics == nil {
		// 没有指标信息，给默认分数
		score.TotalScore = 50.0
		score.ScoreBreakdown["no_metrics"] = 50.0
		return score, nil
	}

	metrics := endpoint.Metrics

	// CPU使用率评分 (0-100)
	cpuScore := math.Max(0, 100.0-metrics.CPUUsage*100)
	score.ScoreBreakdown["cpu"] = cpuScore

	// 内存使用率评分 (0-100)
	memoryScore := math.Max(0, 100.0-metrics.MemoryUsage*100)
	score.ScoreBreakdown["memory"] = memoryScore

	// 活跃请求数评分 (0-100)
	activeRequestsScore := 100.0
	if endpoint.Capabilities != nil && endpoint.Capabilities.MaxConcurrent > 0 {
		utilization := float64(metrics.ActiveRequests) / float64(endpoint.Capabilities.MaxConcurrent)
		activeRequestsScore = math.Max(0, 100.0-utilization*100)
	} else {
		// 简单基于活跃请求数评分
		activeRequestsScore = math.Max(0, 100.0-float64(metrics.ActiveRequests)*10)
	}
	score.ScoreBreakdown["active_requests"] = activeRequestsScore

	// 队列长度评分 (0-100)
	queueScore := math.Max(0, 100.0-float64(metrics.QueuedRequests)*5)
	score.ScoreBreakdown["queue"] = queueScore

	// 错误率评分 (0-100)
	errorScore := math.Max(0, 100.0-metrics.ErrorRate*100)
	score.ScoreBreakdown["error_rate"] = errorScore

	// 综合评分（加权平均）
	score.TotalScore = cpuScore*0.3 + memoryScore*0.2 + activeRequestsScore*0.3 +
		queueScore*0.1 + errorScore*0.1

	// 添加一些随机性，避免所有请求都选择同一个端点
	jitter := math.Max(-5, math.Min(5, float64(endpoint.ID[0]%10)-5))
	score.TotalScore += jitter

	return score, nil
}

// CapabilityScorer 能力匹配评分插件
type CapabilityScorer struct{}

// NewCapabilityScorer 创建能力匹配评分插件
func NewCapabilityScorer() *CapabilityScorer {
	return &CapabilityScorer{}
}

// Name 返回插件名称
func (s *CapabilityScorer) Name() string {
	return "capability"
}

// Score 基于能力匹配度为端点评分
func (s *CapabilityScorer) Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error) {
	score := &types.SchedulingScore{
		EndpointID:     endpoint.ID,
		ScoreBreakdown: make(map[string]float64),
	}

	if endpoint.Capabilities == nil {
		score.TotalScore = 0.0
		score.ScoreBreakdown["no_capabilities"] = 0.0
		return score, nil
	}

	capabilities := endpoint.Capabilities
	baseScore := 50.0

	// 模型支持评分
	modelScore := s.scoreModelSupport(req, capabilities)
	score.ScoreBreakdown["model_support"] = modelScore

	// LoRA支持评分
	loraScore := s.scoreLoRASupport(req, capabilities)
	score.ScoreBreakdown["lora_support"] = loraScore

	// KV缓存支持评分
	kvCacheScore := s.scoreKVCacheSupport(req, capabilities)
	score.ScoreBreakdown["kv_cache"] = kvCacheScore

	// 硬件匹配评分
	hardwareScore := s.scoreHardwareMatch(req, capabilities)
	score.ScoreBreakdown["hardware"] = hardwareScore

	// 综合评分
	score.TotalScore = baseScore + modelScore*0.4 + loraScore*0.3 +
		kvCacheScore*0.2 + hardwareScore*0.1

	return score, nil
}

// scoreModelSupport 评分模型支持度
func (s *CapabilityScorer) scoreModelSupport(req *types.ProcessingRequest, capabilities *types.EndpointCapability) float64 {
	// 检查是否支持请求的模型
	for _, supportedModel := range capabilities.SupportedModels {
		if strings.EqualFold(supportedModel, req.ModelName) {
			return 50.0 // 完全匹配
		}
	}

	// 检查是否支持模型家族
	modelFamily := s.extractModelFamily(req.ModelName)
	for _, supportedModel := range capabilities.SupportedModels {
		supportedFamily := s.extractModelFamily(supportedModel)
		if modelFamily != "" && strings.EqualFold(supportedFamily, modelFamily) {
			return 30.0 // 家族匹配
		}
	}

	return -50.0 // 不支持
}

// scoreLoRASupport 评分LoRA支持度
func (s *CapabilityScorer) scoreLoRASupport(req *types.ProcessingRequest, capabilities *types.EndpointCapability) float64 {
	if !req.LoRARequired {
		// 不需要LoRA，支持与否都可以
		return 0.0
	}

	if !capabilities.LoRASupport {
		return -100.0 // 需要但不支持
	}

	// 检查是否已加载所需LoRA
	if req.LoRAAdapter != "" {
		for _, loadedLoRA := range capabilities.LoadedLoRAs {
			if strings.EqualFold(loadedLoRA, req.LoRAAdapter) {
				return 50.0 // 已加载所需LoRA
			}
		}
		return 10.0 // 支持LoRA但未加载
	}

	return 25.0 // 支持LoRA
}

// scoreKVCacheSupport 评分KV缓存支持度
func (s *CapabilityScorer) scoreKVCacheSupport(req *types.ProcessingRequest, capabilities *types.EndpointCapability) float64 {
	if capabilities.KVCacheSupport {
		return 20.0 // 支持KV缓存，有加速效果
	}
	return 0.0 // 不支持KV缓存，不扣分
}

// scoreHardwareMatch 评分硬件匹配度
func (s *CapabilityScorer) scoreHardwareMatch(req *types.ProcessingRequest, capabilities *types.EndpointCapability) float64 {
	score := 0.0

	// GPU数量评分
	if capabilities.GPUCount > 0 {
		if capabilities.GPUCount >= 2 {
			score += 20.0 // 多GPU
		} else {
			score += 10.0 // 单GPU
		}
	}

	// GPU内存评分
	if capabilities.GPUMemoryFree > 0 {
		// 根据剩余GPU内存评分
		freeGB := float64(capabilities.GPUMemoryFree) / (1024 * 1024 * 1024)
		if freeGB > 16 {
			score += 20.0
		} else if freeGB > 8 {
			score += 10.0
		} else if freeGB > 4 {
			score += 5.0
		}
	}

	return score
}

// extractModelFamily 提取模型家族名
func (s *CapabilityScorer) extractModelFamily(modelName string) string {
	lower := strings.ToLower(modelName)

	if strings.Contains(lower, "llama") {
		return "llama"
	}
	if strings.Contains(lower, "gpt") {
		return "gpt"
	}
	if strings.Contains(lower, "bert") {
		return "bert"
	}
	if strings.Contains(lower, "t5") {
		return "t5"
	}

	return ""
}

// LatencyScorer 延迟评分插件
type LatencyScorer struct{}

// NewLatencyScorer 创建延迟评分插件
func NewLatencyScorer() *LatencyScorer {
	return &LatencyScorer{}
}

// Name 返回插件名称
func (s *LatencyScorer) Name() string {
	return "latency"
}

// Score 基于延迟为端点评分
func (s *LatencyScorer) Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error) {
	score := &types.SchedulingScore{
		EndpointID:     endpoint.ID,
		ScoreBreakdown: make(map[string]float64),
	}

	if endpoint.Metrics == nil {
		score.TotalScore = 50.0
		score.ScoreBreakdown["no_metrics"] = 50.0
		return score, nil
	}

	metrics := endpoint.Metrics

	// 平均延迟评分 (0-100)
	avgLatencyMs := float64(metrics.AvgLatency.Milliseconds())
	avgLatencyScore := math.Max(0, 100.0-avgLatencyMs/10.0) // 1秒=10分
	score.ScoreBreakdown["avg_latency"] = avgLatencyScore

	// P95延迟评分 (0-100)
	p95LatencyMs := float64(metrics.P95Latency.Milliseconds())
	p95LatencyScore := math.Max(0, 100.0-p95LatencyMs/20.0) // 2秒=10分
	score.ScoreBreakdown["p95_latency"] = p95LatencyScore

	// P99延迟评分 (0-100)
	p99LatencyMs := float64(metrics.P99Latency.Milliseconds())
	p99LatencyScore := math.Max(0, 100.0-p99LatencyMs/50.0) // 5秒=10分
	score.ScoreBreakdown["p99_latency"] = p99LatencyScore

	// 综合延迟评分
	score.TotalScore = avgLatencyScore*0.5 + p95LatencyScore*0.3 + p99LatencyScore*0.2

	return score, nil
}

// AffinityScorer 亲和性评分插件
type AffinityScorer struct{}

// NewAffinityScorer 创建亲和性评分插件
func NewAffinityScorer() *AffinityScorer {
	return &AffinityScorer{}
}

// Name 返回插件名称
func (s *AffinityScorer) Name() string {
	return "affinity"
}

// Score 基于亲和性为端点评分
func (s *AffinityScorer) Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error) {
	score := &types.SchedulingScore{
		EndpointID:     endpoint.ID,
		ScoreBreakdown: make(map[string]float64),
	}

	baseScore := 50.0

	// 用户亲和性评分
	userAffinityScore := s.scoreUserAffinity(req, endpoint)
	score.ScoreBreakdown["user_affinity"] = userAffinityScore

	// 模型亲和性评分
	modelAffinityScore := s.scoreModelAffinity(req, endpoint)
	score.ScoreBreakdown["model_affinity"] = modelAffinityScore

	// 地理位置亲和性评分
	locationAffinityScore := s.scoreLocationAffinity(req, endpoint)
	score.ScoreBreakdown["location_affinity"] = locationAffinityScore

	// 综合亲和性评分
	score.TotalScore = baseScore + userAffinityScore*0.4 +
		modelAffinityScore*0.4 + locationAffinityScore*0.2

	return score, nil
}

// scoreUserAffinity 评分用户亲和性
func (s *AffinityScorer) scoreUserAffinity(req *types.ProcessingRequest, endpoint *types.EndpointInfo) float64 {
	userID := req.Headers["x-user-id"]
	if userID == "" {
		return 0.0
	}

	// 检查端点是否有用户亲和性标签
	if endpoint.Labels != nil {
		if preferredUser, exists := endpoint.Labels["preferred-user"]; exists {
			if preferredUser == userID {
				return 30.0 // 用户专用端点
			}
		}

		// 检查用户组亲和性
		if userGroup := req.Headers["x-user-group"]; userGroup != "" {
			if preferredGroup, exists := endpoint.Labels["preferred-group"]; exists {
				if preferredGroup == userGroup {
					return 15.0 // 用户组亲和性
				}
			}
		}
	}

	return 0.0
}

// scoreModelAffinity 评分模型亲和性
func (s *AffinityScorer) scoreModelAffinity(req *types.ProcessingRequest, endpoint *types.EndpointInfo) float64 {
	if endpoint.Labels == nil {
		return 0.0
	}

	// 检查模型专用端点
	if preferredModel, exists := endpoint.Labels["preferred-model"]; exists {
		if strings.EqualFold(preferredModel, req.ModelName) {
			return 25.0 // 模型专用端点
		}
	}

	// 检查模型家族亲和性
	modelFamily := s.extractModelFamily(req.ModelName)
	if modelFamily != "" {
		if preferredFamily, exists := endpoint.Labels["preferred-family"]; exists {
			if strings.EqualFold(preferredFamily, modelFamily) {
				return 15.0 // 模型家族亲和性
			}
		}
	}

	return 0.0
}

// scoreLocationAffinity 评分地理位置亲和性
func (s *AffinityScorer) scoreLocationAffinity(req *types.ProcessingRequest, endpoint *types.EndpointInfo) float64 {
	clientLocation := req.Headers["x-client-location"]
	if clientLocation == "" {
		return 0.0
	}

	if endpoint.Labels == nil {
		return 0.0
	}

	// 检查端点位置
	endpointLocation, exists := endpoint.Labels["location"]
	if !exists {
		return 0.0
	}

	// 计算位置匹配度
	if clientLocation == endpointLocation {
		return 20.0 // 位置完全匹配
	}

	// 检查是否在同一区域
	clientRegion := s.extractRegion(clientLocation)
	endpointRegion := s.extractRegion(endpointLocation)

	if clientRegion != "" && clientRegion == endpointRegion {
		return 10.0 // 区域匹配
	}

	return 0.0
}

// extractRegion 提取区域信息
func (s *AffinityScorer) extractRegion(location string) string {
	parts := strings.Split(location, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1] // 例如：us-west, cn-north
	}
	return ""
}

// extractModelFamily 提取模型家族（复用CapabilityScorer中的方法）
func (s *AffinityScorer) extractModelFamily(modelName string) string {
	lower := strings.ToLower(modelName)

	if strings.Contains(lower, "llama") {
		return "llama"
	}
	if strings.Contains(lower, "gpt") {
		return "gpt"
	}
	if strings.Contains(lower, "bert") {
		return "bert"
	}
	if strings.Contains(lower, "t5") {
		return "t5"
	}

	return ""
}

// ThroughputScorer 吞吐量评分插件
type ThroughputScorer struct{}

// NewThroughputScorer 创建吞吐量评分插件
func NewThroughputScorer() *ThroughputScorer {
	return &ThroughputScorer{}
}

// Name 返回插件名称
func (s *ThroughputScorer) Name() string {
	return "throughput"
}

// Score 基于吞吐量为端点评分
func (s *ThroughputScorer) Score(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) (*types.SchedulingScore, error) {
	score := &types.SchedulingScore{
		EndpointID:     endpoint.ID,
		ScoreBreakdown: make(map[string]float64),
	}

	if endpoint.Capabilities == nil || endpoint.Metrics == nil {
		score.TotalScore = 50.0
		score.ScoreBreakdown["no_data"] = 50.0
		return score, nil
	}

	// 理论吞吐量评分
	theoreticalThroughput := endpoint.Capabilities.Throughput
	theoreticalScore := math.Min(100.0, theoreticalThroughput*10) // 10 rps = 100分
	score.ScoreBreakdown["theoretical"] = theoreticalScore

	// 实际吞吐量评分
	actualThroughput := endpoint.Metrics.RequestsPerSecond
	actualScore := math.Min(100.0, actualThroughput*10)
	score.ScoreBreakdown["actual"] = actualScore

	// 吞吐量利用率评分
	utilizationScore := 50.0
	if theoreticalThroughput > 0 {
		utilization := actualThroughput / theoreticalThroughput
		// 50-70%利用率为最佳
		if utilization >= 0.5 && utilization <= 0.7 {
			utilizationScore = 100.0
		} else if utilization < 0.5 {
			utilizationScore = utilization * 200.0 // 0-50%线性增长到100
		} else {
			utilizationScore = math.Max(0, 100.0-(utilization-0.7)*200) // 超过70%线性下降
		}
	}
	score.ScoreBreakdown["utilization"] = utilizationScore

	// 综合评分
	score.TotalScore = theoreticalScore*0.3 + actualScore*0.4 + utilizationScore*0.3

	return score, nil
}

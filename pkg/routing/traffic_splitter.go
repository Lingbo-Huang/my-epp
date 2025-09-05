package routing

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// TrafficSplitter 流量拆分器实现
type TrafficSplitter struct {
	// 配置
	config *SplitterConfig

	// 随机数生成器
	rng *rand.Rand
}

// SplitterConfig 拆分器配置
type SplitterConfig struct {
	// 拆分策略
	Strategy string `json:"strategy"` // random, sticky, canary

	// 粘性会话配置
	StickyKeyHeader string `json:"sticky_key_header"` // 用于粘性会话的头部字段
	StickyTTL       int    `json:"sticky_ttl"`        // 粘性会话TTL（秒）

	// 金丝雀发布配置
	CanaryRampRate   float64 `json:"canary_ramp_rate"`   // 金丝雀流量递增率
	CanaryMaxTraffic float64 `json:"canary_max_traffic"` // 金丝雀最大流量比例

	// 权重调整
	EnableWeightAdjust bool `json:"enable_weight_adjust"` // 是否根据负载动态调整权重
}

// NewTrafficSplitter 创建新的流量拆分器
func NewTrafficSplitter(config *SplitterConfig) *TrafficSplitter {
	if config == nil {
		config = &SplitterConfig{
			Strategy:           "random",
			StickyKeyHeader:    "x-user-id",
			StickyTTL:          3600,
			CanaryRampRate:     0.1,
			CanaryMaxTraffic:   0.2,
			EnableWeightAdjust: true,
		}
	}

	return &TrafficSplitter{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SplitTraffic 根据策略拆分流量
func (ts *TrafficSplitter) SplitTraffic(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可用的候选端点")
	}

	log.Printf("开始流量拆分 - 策略: %s, 候选数: %d", ts.config.Strategy, len(candidates))

	switch ts.config.Strategy {
	case "random":
		return ts.randomSplit(req, candidates)
	case "weighted":
		return ts.weightedSplit(req, candidates)
	case "sticky":
		return ts.stickySplit(req, candidates)
	case "canary":
		return ts.canarySplit(req, candidates)
	case "percentage":
		return ts.percentageSplit(req, candidates)
	default:
		log.Printf("未知的拆分策略: %s, 使用随机拆分", ts.config.Strategy)
		return ts.randomSplit(req, candidates)
	}
}

// randomSplit 随机流量拆分
func (ts *TrafficSplitter) randomSplit(req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 随机选择一个端点
	selectedIndex := ts.rng.Intn(len(candidates))
	selected := candidates[selectedIndex]

	log.Printf("随机选择端点: %s", selected.ID)

	return []*types.EndpointInfo{selected}, nil
}

// weightedSplit 加权流量拆分
func (ts *TrafficSplitter) weightedSplit(req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 构建权重列表
	weights := ts.calculateWeights(candidates)

	// 累积权重
	totalWeight := 0.0
	for _, weight := range weights {
		totalWeight += weight
	}

	if totalWeight == 0 {
		return ts.randomSplit(req, candidates)
	}

	// 根据权重随机选择
	random := ts.rng.Float64() * totalWeight
	currentWeight := 0.0

	for i, weight := range weights {
		currentWeight += weight
		if random <= currentWeight {
			log.Printf("加权选择端点: %s (权重: %.2f)", candidates[i].ID, weight)
			return []*types.EndpointInfo{candidates[i]}, nil
		}
	}

	// 兜底返回第一个
	return []*types.EndpointInfo{candidates[0]}, nil
}

// stickySplit 粘性会话流量拆分
func (ts *TrafficSplitter) stickySplit(req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 获取粘性键值
	stickyKey := ts.getStickyKey(req)
	if stickyKey == "" {
		log.Printf("无粘性键，使用随机拆分")
		return ts.randomSplit(req, candidates)
	}

	// 根据粘性键计算一致性哈希
	hash := ts.calculateHash(stickyKey)
	selectedIndex := hash % len(candidates)
	selected := candidates[selectedIndex]

	log.Printf("粘性选择端点: %s (键: %s, 哈希: %d)", selected.ID, stickyKey, hash)

	return []*types.EndpointInfo{selected}, nil
}

// canarySplit 金丝雀流量拆分
func (ts *TrafficSplitter) canarySplit(req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 分离稳定版本和金丝雀版本
	stableEndpoints, canaryEndpoints := ts.separateCanaryEndpoints(candidates)

	if len(canaryEndpoints) == 0 {
		log.Printf("无金丝雀端点，使用稳定版本")
		return ts.randomSplit(req, stableEndpoints)
	}

	// 计算当前金丝雀流量比例
	canaryRatio := ts.getCurrentCanaryRatio()

	// 根据比例决定是否路由到金丝雀
	if ts.rng.Float64() < canaryRatio {
		log.Printf("路由到金丝雀端点 (比例: %.2f%%)", canaryRatio*100)
		return ts.randomSplit(req, canaryEndpoints)
	} else {
		log.Printf("路由到稳定端点 (比例: %.2f%%)", (1-canaryRatio)*100)
		return ts.randomSplit(req, stableEndpoints)
	}
}

// percentageSplit 百分比流量拆分
func (ts *TrafficSplitter) percentageSplit(req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 从请求元数据中获取流量拆分配置
	trafficSplit := ts.getTrafficSplitConfig(req)
	if len(trafficSplit) == 0 {
		return ts.randomSplit(req, candidates)
	}

	// 构建加权候选列表
	var weightedCandidates []weightedEndpoint

	for _, endpoint := range candidates {
		weight := trafficSplit[endpoint.ID]
		if weight == 0 {
			// 检查是否有标签匹配
			weight = ts.getWeightByLabels(endpoint, trafficSplit)
		}

		if weight > 0 {
			weightedCandidates = append(weightedCandidates, weightedEndpoint{
				endpoint: endpoint,
				weight:   weight,
			})
		}
	}

	if len(weightedCandidates) == 0 {
		return ts.randomSplit(req, candidates)
	}

	// 根据权重选择
	selected := ts.selectByWeight(weightedCandidates)

	log.Printf("百分比选择端点: %s (权重: %.2f%%)",
		selected.ID, trafficSplit[selected.ID]*100)

	return []*types.EndpointInfo{selected}, nil
}

// calculateWeights 计算端点权重
func (ts *TrafficSplitter) calculateWeights(candidates []*types.EndpointInfo) []float64 {
	weights := make([]float64, len(candidates))

	for i, endpoint := range candidates {
		weight := 1.0 // 默认权重

		// 根据负载调整权重
		if ts.config.EnableWeightAdjust && endpoint.Metrics != nil {
			// CPU使用率越低，权重越高
			cpuFactor := math.Max(0.1, 1.0-endpoint.Metrics.CPUUsage)

			// 活跃请求数越少，权重越高
			activeFactor := math.Max(0.1, 1.0/(1.0+float64(endpoint.Metrics.ActiveRequests)/10.0))

			// 错误率越低，权重越高
			errorFactor := math.Max(0.1, 1.0-endpoint.Metrics.ErrorRate)

			weight = cpuFactor * activeFactor * errorFactor
		}

		// 考虑健康状态
		if endpoint.Status != types.EndpointStatusHealthy {
			weight *= 0.1 // 不健康端点权重降低
		}

		weights[i] = weight
	}

	return weights
}

// getStickyKey 获取粘性键
func (ts *TrafficSplitter) getStickyKey(req *types.ProcessingRequest) string {
	// 从指定头部获取
	if key := req.Headers[ts.config.StickyKeyHeader]; key != "" {
		return key
	}

	// 尝试其他常见键
	stickyHeaders := []string{"x-user-id", "x-session-id", "authorization"}
	for _, header := range stickyHeaders {
		if key := req.Headers[header]; key != "" {
			return key
		}
	}

	return ""
}

// calculateHash 计算哈希值
func (ts *TrafficSplitter) calculateHash(key string) int {
	hash := 0
	for _, char := range key {
		hash = hash*31 + int(char)
	}

	if hash < 0 {
		hash = -hash
	}

	return hash
}

// separateCanaryEndpoints 分离金丝雀端点
func (ts *TrafficSplitter) separateCanaryEndpoints(candidates []*types.EndpointInfo) ([]*types.EndpointInfo, []*types.EndpointInfo) {
	var stable, canary []*types.EndpointInfo

	for _, endpoint := range candidates {
		// 根据标签判断是否为金丝雀
		if ts.isCanaryEndpoint(endpoint) {
			canary = append(canary, endpoint)
		} else {
			stable = append(stable, endpoint)
		}
	}

	return stable, canary
}

// isCanaryEndpoint 判断是否为金丝雀端点
func (ts *TrafficSplitter) isCanaryEndpoint(endpoint *types.EndpointInfo) bool {
	if endpoint.Labels == nil {
		return false
	}

	// 检查金丝雀标签
	canaryLabels := []string{"canary", "beta", "experimental"}
	for _, label := range canaryLabels {
		if value, exists := endpoint.Labels[label]; exists && value == "true" {
			return true
		}
	}

	// 检查版本标签
	if version, exists := endpoint.Labels["version"]; exists {
		canaryVersions := []string{"canary", "beta", "rc"}
		for _, cv := range canaryVersions {
			if version == cv {
				return true
			}
		}
	}

	return false
}

// getCurrentCanaryRatio 获取当前金丝雀流量比例
func (ts *TrafficSplitter) getCurrentCanaryRatio() float64 {
	// 这里应该从配置或数据库中获取当前的金丝雀比例
	// 为了演示，返回固定值
	return math.Min(ts.config.CanaryMaxTraffic, 0.1) // 10%
}

// getTrafficSplitConfig 获取流量拆分配置
func (ts *TrafficSplitter) getTrafficSplitConfig(req *types.ProcessingRequest) map[string]float64 {
	// 从请求头中获取流量拆分配置
	if configHeader := req.Headers["x-traffic-split"]; configHeader != "" {
		// 解析配置（简化实现）
		// 实际应该解析JSON格式的配置
		return map[string]float64{
			"endpoint_1": 0.7,
			"endpoint_2": 0.3,
		}
	}

	return nil
}

// getWeightByLabels 根据标签获取权重
func (ts *TrafficSplitter) getWeightByLabels(endpoint *types.EndpointInfo, trafficSplit map[string]float64) float64 {
	if endpoint.Labels == nil {
		return 0
	}

	// 检查标签匹配
	for key, weight := range trafficSplit {
		if label, exists := endpoint.Labels["group"]; exists && label == key {
			return weight
		}
		if label, exists := endpoint.Labels["version"]; exists && label == key {
			return weight
		}
	}

	return 0
}

// weightedEndpoint 带权重的端点
type weightedEndpoint struct {
	endpoint *types.EndpointInfo
	weight   float64
}

// selectByWeight 根据权重选择端点
func (ts *TrafficSplitter) selectByWeight(candidates []weightedEndpoint) *types.EndpointInfo {
	if len(candidates) == 0 {
		return nil
	}

	if len(candidates) == 1 {
		return candidates[0].endpoint
	}

	// 计算总权重
	totalWeight := 0.0
	for _, candidate := range candidates {
		totalWeight += candidate.weight
	}

	// 随机选择
	random := ts.rng.Float64() * totalWeight
	currentWeight := 0.0

	for _, candidate := range candidates {
		currentWeight += candidate.weight
		if random <= currentWeight {
			return candidate.endpoint
		}
	}

	// 兜底返回第一个
	return candidates[0].endpoint
}

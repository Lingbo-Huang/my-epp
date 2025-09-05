package scheduling

import (
	"context"
	"log"
	"strings"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// HealthFilter 健康检查过滤器
type HealthFilter struct{}

// NewHealthFilter 创建健康检查过滤器
func NewHealthFilter() *HealthFilter {
	return &HealthFilter{}
}

// Name 返回插件名称
func (f *HealthFilter) Name() string {
	return "health"
}

// Filter 过滤不健康的端点
func (f *HealthFilter) Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	var healthyEndpoints []*types.EndpointInfo

	for _, endpoint := range endpoints {
		// 检查端点健康状态
		if f.isHealthy(endpoint) {
			healthyEndpoints = append(healthyEndpoints, endpoint)
		} else {
			log.Printf("过滤不健康端点: %s (状态: %s)", endpoint.ID, endpoint.Status.String())
		}
	}

	return healthyEndpoints, nil
}

// isHealthy 检查端点是否健康
func (f *HealthFilter) isHealthy(endpoint *types.EndpointInfo) bool {
	// 检查基本状态
	if endpoint.Status != types.EndpointStatusHealthy {
		return false
	}

	// 检查健康检查结果
	if endpoint.Health != nil {
		if !endpoint.Health.Healthy {
			return false
		}

		// 检查连续失败次数
		if endpoint.Health.ConsecutiveFails > 3 {
			return false
		}
	}

	return true
}

// CapabilityFilter 能力匹配过滤器
type CapabilityFilter struct{}

// NewCapabilityFilter 创建能力匹配过滤器
func NewCapabilityFilter() *CapabilityFilter {
	return &CapabilityFilter{}
}

// Name 返回插件名称
func (f *CapabilityFilter) Name() string {
	return "capability"
}

// Filter 过滤不符合能力要求的端点
func (f *CapabilityFilter) Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	var capableEndpoints []*types.EndpointInfo

	for _, endpoint := range endpoints {
		if f.hasRequiredCapabilities(req, endpoint) {
			capableEndpoints = append(capableEndpoints, endpoint)
		} else {
			log.Printf("过滤能力不匹配端点: %s", endpoint.ID)
		}
	}

	return capableEndpoints, nil
}

// hasRequiredCapabilities 检查端点是否具备所需能力
func (f *CapabilityFilter) hasRequiredCapabilities(req *types.ProcessingRequest, endpoint *types.EndpointInfo) bool {
	if endpoint.Capabilities == nil {
		return false
	}

	capabilities := endpoint.Capabilities

	// 检查模型支持
	modelSupported := false
	for _, supportedModel := range capabilities.SupportedModels {
		if strings.EqualFold(supportedModel, req.ModelName) {
			modelSupported = true
			break
		}
	}

	if !modelSupported {
		return false
	}

	// 检查LoRA支持
	if req.LoRARequired {
		if !capabilities.LoRASupport {
			return false
		}

		// 检查是否已加载所需的LoRA
		if req.LoRAAdapter != "" {
			loraLoaded := false
			for _, loadedLoRA := range capabilities.LoadedLoRAs {
				if strings.EqualFold(loadedLoRA, req.LoRAAdapter) {
					loraLoaded = true
					break
				}
			}

			if !loraLoaded {
				return false
			}
		}
	}

	// 检查资源容量
	if capabilities.MaxConcurrent > 0 {
		if endpoint.Metrics != nil {
			if endpoint.Metrics.ActiveRequests >= capabilities.MaxConcurrent {
				return false
			}
		}
	}

	return true
}

// LoadFilter 负载过滤器
type LoadFilter struct{}

// NewLoadFilter 创建负载过滤器
func NewLoadFilter() *LoadFilter {
	return &LoadFilter{}
}

// Name 返回插件名称
func (f *LoadFilter) Name() string {
	return "load"
}

// Filter 过滤负载过高的端点
func (f *LoadFilter) Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	var lowLoadEndpoints []*types.EndpointInfo

	for _, endpoint := range endpoints {
		if f.isLoadAcceptable(endpoint) {
			lowLoadEndpoints = append(lowLoadEndpoints, endpoint)
		} else {
			log.Printf("过滤高负载端点: %s", endpoint.ID)
		}
	}

	// 如果所有端点都被过滤掉了，返回负载最低的一个
	if len(lowLoadEndpoints) == 0 && len(endpoints) > 0 {
		bestEndpoint := f.findLowestLoadEndpoint(endpoints)
		if bestEndpoint != nil {
			log.Printf("所有端点负载过高，选择负载最低的: %s", bestEndpoint.ID)
			lowLoadEndpoints = []*types.EndpointInfo{bestEndpoint}
		}
	}

	return lowLoadEndpoints, nil
}

// isLoadAcceptable 检查端点负载是否可接受
func (f *LoadFilter) isLoadAcceptable(endpoint *types.EndpointInfo) bool {
	if endpoint.Metrics == nil {
		return true // 没有指标信息，假设负载可接受
	}

	metrics := endpoint.Metrics

	// 检查CPU使用率
	if metrics.CPUUsage > 0.90 { // 90% CPU使用率阈值
		return false
	}

	// 检查内存使用率
	if metrics.MemoryUsage > 0.85 { // 85% 内存使用率阈值
		return false
	}

	// 检查队列长度
	if endpoint.Capabilities != nil {
		if endpoint.Capabilities.MaxQueueSize > 0 {
			if metrics.QueuedRequests >= endpoint.Capabilities.MaxQueueSize {
				return false
			}
		}
	}

	// 检查错误率
	if metrics.ErrorRate > 0.10 { // 10% 错误率阈值
		return false
	}

	return true
}

// findLowestLoadEndpoint 找到负载最低的端点
func (f *LoadFilter) findLowestLoadEndpoint(endpoints []*types.EndpointInfo) *types.EndpointInfo {
	if len(endpoints) == 0 {
		return nil
	}

	bestEndpoint := endpoints[0]
	lowestScore := f.calculateLoadScore(bestEndpoint)

	for i := 1; i < len(endpoints); i++ {
		endpoint := endpoints[i]
		score := f.calculateLoadScore(endpoint)

		if score < lowestScore {
			lowestScore = score
			bestEndpoint = endpoint
		}
	}

	return bestEndpoint
}

// calculateLoadScore 计算负载分数（越低越好）
func (f *LoadFilter) calculateLoadScore(endpoint *types.EndpointInfo) float64 {
	if endpoint.Metrics == nil {
		return 0.0 // 没有指标，假设负载最低
	}

	metrics := endpoint.Metrics

	// 综合负载分数计算
	score := metrics.CPUUsage*0.3 +
		metrics.MemoryUsage*0.3 +
		float64(metrics.ActiveRequests)/10.0*0.2 +
		metrics.ErrorRate*0.2

	return score
}

// ZoneFilter 可用区过滤器
type ZoneFilter struct {
	PreferredZones []string
	AvoidZones     []string
}

// NewZoneFilter 创建可用区过滤器
func NewZoneFilter(preferred, avoid []string) *ZoneFilter {
	return &ZoneFilter{
		PreferredZones: preferred,
		AvoidZones:     avoid,
	}
}

// Name 返回插件名称
func (f *ZoneFilter) Name() string {
	return "zone"
}

// Filter 根据可用区过滤端点
func (f *ZoneFilter) Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	var filteredEndpoints []*types.EndpointInfo

	for _, endpoint := range endpoints {
		zone := f.getEndpointZone(endpoint)

		// 检查是否在避免区域
		if f.isInAvoidZones(zone) {
			log.Printf("过滤避免区域端点: %s (区域: %s)", endpoint.ID, zone)
			continue
		}

		filteredEndpoints = append(filteredEndpoints, endpoint)
	}

	// 如果有首选区域，进一步过滤
	if len(f.PreferredZones) > 0 {
		var preferredEndpoints []*types.EndpointInfo

		for _, endpoint := range filteredEndpoints {
			zone := f.getEndpointZone(endpoint)
			if f.isInPreferredZones(zone) {
				preferredEndpoints = append(preferredEndpoints, endpoint)
			}
		}

		// 如果首选区域有端点，使用首选区域的端点
		if len(preferredEndpoints) > 0 {
			return preferredEndpoints, nil
		}
	}

	return filteredEndpoints, nil
}

// getEndpointZone 获取端点所在区域
func (f *ZoneFilter) getEndpointZone(endpoint *types.EndpointInfo) string {
	if endpoint.Labels != nil {
		if zone, exists := endpoint.Labels["zone"]; exists {
			return zone
		}
		if az, exists := endpoint.Labels["availability-zone"]; exists {
			return az
		}
	}

	return "unknown"
}

// isInAvoidZones 检查是否在避免区域
func (f *ZoneFilter) isInAvoidZones(zone string) bool {
	for _, avoid := range f.AvoidZones {
		if zone == avoid {
			return true
		}
	}
	return false
}

// isInPreferredZones 检查是否在首选区域
func (f *ZoneFilter) isInPreferredZones(zone string) bool {
	for _, preferred := range f.PreferredZones {
		if zone == preferred {
			return true
		}
	}
	return false
}

// VersionFilter 版本过滤器
type VersionFilter struct {
	RequiredVersion string
	MinVersion      string
	MaxVersion      string
}

// NewVersionFilter 创建版本过滤器
func NewVersionFilter(required, min, max string) *VersionFilter {
	return &VersionFilter{
		RequiredVersion: required,
		MinVersion:      min,
		MaxVersion:      max,
	}
}

// Name 返回插件名称
func (f *VersionFilter) Name() string {
	return "version"
}

// Filter 根据版本过滤端点
func (f *VersionFilter) Filter(ctx context.Context, req *types.ProcessingRequest, endpoints []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	var filteredEndpoints []*types.EndpointInfo

	for _, endpoint := range endpoints {
		version := f.getEndpointVersion(endpoint)

		if f.isVersionCompatible(version) {
			filteredEndpoints = append(filteredEndpoints, endpoint)
		} else {
			log.Printf("过滤版本不兼容端点: %s (版本: %s)", endpoint.ID, version)
		}
	}

	return filteredEndpoints, nil
}

// getEndpointVersion 获取端点版本
func (f *VersionFilter) getEndpointVersion(endpoint *types.EndpointInfo) string {
	if endpoint.Labels != nil {
		if version, exists := endpoint.Labels["version"]; exists {
			return version
		}
	}

	return "unknown"
}

// isVersionCompatible 检查版本是否兼容
func (f *VersionFilter) isVersionCompatible(version string) bool {
	if version == "unknown" {
		return true // 未知版本，假设兼容
	}

	// 简化的版本比较实现
	if f.RequiredVersion != "" && f.RequiredVersion != version {
		return false
	}

	// 实际实现中应该进行更复杂的语义版本比较

	return true
}

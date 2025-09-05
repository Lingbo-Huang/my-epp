package routing

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Lingbo-Huang/my-epp/pkg/interfaces"
	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// Director 路由决策器实现
type Director struct {
	// 依赖组件
	modelNameResolver interfaces.ModelNameResolver
	trafficSplitter   interfaces.TrafficSplitter

	// 策略配置
	policies     []*types.RoutingPolicy
	policiesLock sync.RWMutex

	// 配置
	config *DirectorConfig
}

// DirectorConfig Director配置
type DirectorConfig struct {
	// 默认路由策略
	DefaultStrategy string `json:"default_strategy"` // round_robin, weighted, capability_based

	// 流量拆分启用
	EnableTrafficSplit bool `json:"enable_traffic_split"`

	// 路由规则匹配模式
	MatchMode string `json:"match_mode"` // first_match, best_match
}

// NewDirector 创建新的路由决策器
func NewDirector(
	modelNameResolver interfaces.ModelNameResolver,
	trafficSplitter interfaces.TrafficSplitter,
	config *DirectorConfig,
) *Director {
	if config == nil {
		config = &DirectorConfig{
			DefaultStrategy:    "capability_based",
			EnableTrafficSplit: true,
			MatchMode:          "first_match",
		}
	}

	return &Director{
		modelNameResolver: modelNameResolver,
		trafficSplitter:   trafficSplitter,
		config:            config,
		policies:          make([]*types.RoutingPolicy, 0),
	}
}

// Route 执行路由决策
func (d *Director) Route(ctx context.Context, req *types.ProcessingRequest) (*types.RoutingDecision, error) {
	log.Printf("开始路由决策 - 请求ID: %s, 模型: %s", req.ID, req.ModelName)

	// 解析模型名称
	resolvedName, version, err := d.modelNameResolver.ResolveModelName(req.ModelName)
	if err != nil {
		return nil, fmt.Errorf("模型名解析失败: %w", err)
	}

	log.Printf("模型名解析完成 - 原始: %s, 解析: %s, 版本: %s",
		req.ModelName, resolvedName, version)

	// 更新请求中的模型信息
	req.ModelName = resolvedName
	if req.ModelVersion == "" {
		req.ModelVersion = version
	}

	// 查找匹配的路由策略
	matchedPolicy, matchedRules := d.findMatchingPolicy(req)

	// 确定路由策略
	strategy := d.config.DefaultStrategy
	var trafficSplit map[string]float64
	var appliedRules []string

	if matchedPolicy != nil {
		log.Printf("匹配到路由策略: %s", matchedPolicy.Name)

		// 处理匹配的规则
		if len(matchedRules) > 0 {
			// 使用第一个匹配规则的策略
			rule := matchedRules[0]
			if rule.Target != "" {
				strategy = "target_based"
			}

			appliedRules = append(appliedRules, rule.Name)
			log.Printf("应用路由规则: %s -> 目标: %s", rule.Name, rule.Target)
		}

		// 处理流量拆分
		if d.config.EnableTrafficSplit && len(matchedPolicy.TrafficSplit) > 0 {
			trafficSplit = matchedPolicy.TrafficSplit
			strategy = "traffic_split"
			log.Printf("启用流量拆分: %v", trafficSplit)
		}
	}

	// 根据模型特征调整策略
	strategy = d.adjustStrategyByModel(req, strategy)

	// 构建路由决策
	decision := &types.RoutingDecision{
		Strategy:     strategy,
		Rules:        appliedRules,
		TrafficSplit: trafficSplit,
		Reason:       d.buildDecisionReason(req, strategy, matchedPolicy),
	}

	log.Printf("路由决策完成 - 策略: %s, 原因: %s", strategy, decision.Reason)

	return decision, nil
}

// UpdatePolicy 更新路由策略
func (d *Director) UpdatePolicy(policy *types.RoutingPolicy) error {
	d.policiesLock.Lock()
	defer d.policiesLock.Unlock()

	// 查找是否存在同名策略
	for i, p := range d.policies {
		if p.Name == policy.Name {
			d.policies[i] = policy
			log.Printf("更新路由策略: %s", policy.Name)
			return nil
		}
	}

	// 添加新策略
	d.policies = append(d.policies, policy)
	log.Printf("添加新路由策略: %s", policy.Name)

	return nil
}

// findMatchingPolicy 查找匹配的路由策略
func (d *Director) findMatchingPolicy(req *types.ProcessingRequest) (*types.RoutingPolicy, []*types.RoutingRule) {
	d.policiesLock.RLock()
	defer d.policiesLock.RUnlock()

	for _, policy := range d.policies {
		if !policy.Enabled {
			continue
		}

		matchedRules := d.findMatchingRules(policy, req)
		if len(matchedRules) > 0 {
			return policy, matchedRules
		}
	}

	return nil, nil
}

// findMatchingRules 查找匹配的路由规则
func (d *Director) findMatchingRules(policy *types.RoutingPolicy, req *types.ProcessingRequest) []*types.RoutingRule {
	var matchedRules []*types.RoutingRule

	for _, rule := range policy.Rules {
		if d.ruleMatches(&rule, req) {
			matchedRules = append(matchedRules, &rule)

			// 如果是第一匹配模式，找到一个就返回
			if d.config.MatchMode == "first_match" {
				break
			}
		}
	}

	return matchedRules
}

// ruleMatches 检查规则是否匹配请求
func (d *Director) ruleMatches(rule *types.RoutingRule, req *types.ProcessingRequest) bool {
	for _, condition := range rule.Conditions {
		if !d.conditionMatches(&condition, req) {
			return false
		}
	}
	return true
}

// conditionMatches 检查单个条件是否匹配
func (d *Director) conditionMatches(condition *types.PolicyCondition, req *types.ProcessingRequest) bool {
	fieldValue := d.getFieldValue(condition.Field, req)

	switch condition.Operator {
	case "eq":
		return d.valueEquals(fieldValue, condition.Value)
	case "ne":
		return !d.valueEquals(fieldValue, condition.Value)
	case "in":
		return d.valueInSlice(fieldValue, condition.Values)
	case "not_in":
		return !d.valueInSlice(fieldValue, condition.Values)
	case "contains":
		return d.valueContains(fieldValue, condition.Value)
	case "regex":
		// TODO: 实现正则表达式匹配
		return false
	default:
		log.Printf("未支持的条件操作符: %s", condition.Operator)
		return false
	}
}

// getFieldValue 获取请求字段值
func (d *Director) getFieldValue(field string, req *types.ProcessingRequest) interface{} {
	switch field {
	case "model_name":
		return req.ModelName
	case "model_version":
		return req.ModelVersion
	case "lora_required":
		return req.LoRARequired
	case "lora_adapter":
		return req.LoRAAdapter
	case "priority":
		return req.Priority
	case "user_id":
		return req.Headers["x-user-id"]
	case "tenant_id":
		return req.Headers["x-tenant-id"]
	case "path":
		return req.Metadata["path"]
	default:
		// 尝试从headers或metadata中获取
		if val, exists := req.Headers[field]; exists {
			return val
		}
		if val, exists := req.Metadata[field]; exists {
			return val
		}
		return nil
	}
}

// valueEquals 检查值是否相等
func (d *Director) valueEquals(fieldValue, condValue interface{}) bool {
	if fieldValue == nil || condValue == nil {
		return fieldValue == condValue
	}

	return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", condValue)
}

// valueInSlice 检查值是否在切片中
func (d *Director) valueInSlice(fieldValue interface{}, values []interface{}) bool {
	if fieldValue == nil {
		return false
	}

	fieldStr := fmt.Sprintf("%v", fieldValue)
	for _, val := range values {
		if fieldStr == fmt.Sprintf("%v", val) {
			return true
		}
	}

	return false
}

// valueContains 检查值是否包含子串
func (d *Director) valueContains(fieldValue, condValue interface{}) bool {
	if fieldValue == nil || condValue == nil {
		return false
	}

	fieldStr := strings.ToLower(fmt.Sprintf("%v", fieldValue))
	condStr := strings.ToLower(fmt.Sprintf("%v", condValue))

	return strings.Contains(fieldStr, condStr)
}

// adjustStrategyByModel 根据模型特征调整策略
func (d *Director) adjustStrategyByModel(req *types.ProcessingRequest, currentStrategy string) string {
	// 如果请求需要LoRA，优先使用capability_based策略
	if req.LoRARequired {
		if currentStrategy == "round_robin" {
			log.Printf("LoRA请求调整策略: %s -> capability_based", currentStrategy)
			return "capability_based"
		}
	}

	// 根据模型类型调整
	if strings.Contains(strings.ToLower(req.ModelName), "large") {
		// 大模型优先使用负载均衡
		if currentStrategy == "round_robin" {
			return "least_connections"
		}
	}

	return currentStrategy
}

// buildDecisionReason 构建决策原因描述
func (d *Director) buildDecisionReason(req *types.ProcessingRequest, strategy string, policy *types.RoutingPolicy) string {
	var reasons []string

	reasons = append(reasons, fmt.Sprintf("策略=%s", strategy))

	if req.LoRARequired {
		reasons = append(reasons, "需要LoRA适配器")
	}

	if policy != nil {
		reasons = append(reasons, fmt.Sprintf("匹配策略=%s", policy.Name))
	}

	if req.Priority > 5 {
		reasons = append(reasons, "高优先级请求")
	}

	return strings.Join(reasons, ", ")
}

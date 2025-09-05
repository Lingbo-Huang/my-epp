package routing

import (
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// ModelNameResolver 模型名解析器实现
type ModelNameResolver struct {
	// 模型映射配置
	modelMappings map[string]*ModelMapping
	mappingsLock  sync.RWMutex

	// 模型能力配置
	modelCapabilities map[string]*types.EndpointCapability
	capabilitiesLock  sync.RWMutex

	// 配置
	config *ResolverConfig
}

// ModelMapping 模型映射配置
type ModelMapping struct {
	// 原始名称模式
	Pattern string `json:"pattern"` // 支持通配符或正则

	// 解析后的信息
	ResolvedName   string `json:"resolved_name"`
	DefaultVersion string `json:"default_version"`

	// 能力要求
	RequiresLoRA   bool     `json:"requires_lora"`
	SupportedLoRAs []string `json:"supported_loras"`
	MinGPUMemory   int64    `json:"min_gpu_memory"`

	// 标签
	Labels map[string]string `json:"labels"`
}

// ResolverConfig 解析器配置
type ResolverConfig struct {
	// 默认版本
	DefaultVersion string `json:"default_version"`

	// 是否启用模糊匹配
	EnableFuzzyMatch bool `json:"enable_fuzzy_match"`

	// 版本提取模式
	VersionPattern string `json:"version_pattern"`
}

// NewModelNameResolver 创建新的模型名解析器
func NewModelNameResolver(config *ResolverConfig) *ModelNameResolver {
	if config == nil {
		config = &ResolverConfig{
			DefaultVersion:   "v1",
			EnableFuzzyMatch: true,
			VersionPattern:   `v?(\d+(?:\.\d+)*(?:-\w+)?)`,
		}
	}

	resolver := &ModelNameResolver{
		modelMappings:     make(map[string]*ModelMapping),
		modelCapabilities: make(map[string]*types.EndpointCapability),
		config:            config,
	}

	// 初始化默认映射
	resolver.initDefaultMappings()

	return resolver
}

// ResolveModelName 解析模型名称和版本
func (r *ModelNameResolver) ResolveModelName(modelName string) (resolvedName, version string, err error) {
	log.Printf("开始解析模型名: %s", modelName)

	// 预处理模型名
	cleanName := r.preprocessModelName(modelName)

	// 提取版本信息
	extractedName, extractedVersion := r.extractVersion(cleanName)

	// 查找精确匹配
	if mapping, found := r.findExactMapping(extractedName); found {
		resolvedName = mapping.ResolvedName
		version = extractedVersion
		if version == "" {
			version = mapping.DefaultVersion
		}

		log.Printf("精确匹配 - 原始: %s, 解析: %s, 版本: %s",
			modelName, resolvedName, version)
		return resolvedName, version, nil
	}

	// 查找模式匹配
	if mapping, found := r.findPatternMapping(extractedName); found {
		resolvedName = mapping.ResolvedName
		version = extractedVersion
		if version == "" {
			version = mapping.DefaultVersion
		}

		log.Printf("模式匹配 - 原始: %s, 解析: %s, 版本: %s",
			modelName, resolvedName, version)
		return resolvedName, version, nil
	}

	// 模糊匹配
	if r.config.EnableFuzzyMatch {
		if mapping, found := r.findFuzzyMapping(extractedName); found {
			resolvedName = mapping.ResolvedName
			version = extractedVersion
			if version == "" {
				version = mapping.DefaultVersion
			}

			log.Printf("模糊匹配 - 原始: %s, 解析: %s, 版本: %s",
				modelName, resolvedName, version)
			return resolvedName, version, nil
		}
	}

	// 如果都没匹配到，使用原始名称
	resolvedName = extractedName
	version = extractedVersion
	if version == "" {
		version = r.config.DefaultVersion
	}

	log.Printf("使用原始名称 - 原始: %s, 解析: %s, 版本: %s",
		modelName, resolvedName, version)

	return resolvedName, version, nil
}

// GetModelCapabilities 获取模型能力要求
func (r *ModelNameResolver) GetModelCapabilities(modelName string) (*types.EndpointCapability, error) {
	r.capabilitiesLock.RLock()
	defer r.capabilitiesLock.RUnlock()

	// 首先查找精确匹配
	if capability, exists := r.modelCapabilities[modelName]; exists {
		log.Printf("找到模型能力配置: %s", modelName)
		return capability, nil
	}

	// 查找模型映射中的能力信息
	if mapping, found := r.findExactMapping(modelName); found {
		capability := &types.EndpointCapability{
			SupportedModels: []string{modelName},
			LoRASupport:     mapping.RequiresLoRA,
			LoadedLoRAs:     mapping.SupportedLoRAs,
		}

		if mapping.MinGPUMemory > 0 {
			capability.GPUMemoryTotal = mapping.MinGPUMemory
		}

		log.Printf("从映射生成模型能力: %s", modelName)
		return capability, nil
	}

	// 根据模型名推断能力
	capability := r.inferCapabilitiesFromName(modelName)
	log.Printf("推断模型能力: %s -> LoRA: %v", modelName, capability.LoRASupport)

	return capability, nil
}

// AddModelMapping 添加模型映射
func (r *ModelNameResolver) AddModelMapping(pattern string, mapping *ModelMapping) {
	r.mappingsLock.Lock()
	defer r.mappingsLock.Unlock()

	r.modelMappings[pattern] = mapping
	log.Printf("添加模型映射: %s -> %s", pattern, mapping.ResolvedName)
}

// AddModelCapability 添加模型能力配置
func (r *ModelNameResolver) AddModelCapability(modelName string, capability *types.EndpointCapability) {
	r.capabilitiesLock.Lock()
	defer r.capabilitiesLock.Unlock()

	r.modelCapabilities[modelName] = capability
	log.Printf("添加模型能力: %s", modelName)
}

// initDefaultMappings 初始化默认映射
func (r *ModelNameResolver) initDefaultMappings() {
	defaultMappings := map[string]*ModelMapping{
		"llama2-7b": {
			Pattern:        "llama2-7b*",
			ResolvedName:   "llama2-7b",
			DefaultVersion: "v1",
			RequiresLoRA:   false,
			Labels: map[string]string{
				"family": "llama2",
				"size":   "7b",
			},
		},
		"llama2-7b-lora": {
			Pattern:        "llama2-7b-lora*",
			ResolvedName:   "llama2-7b-lora",
			DefaultVersion: "v1",
			RequiresLoRA:   true,
			SupportedLoRAs: []string{"llama2-7b-lora"},
			Labels: map[string]string{
				"family": "llama2",
				"size":   "7b",
				"type":   "lora",
			},
		},
		"llama2-7b-base": {
			Pattern:        "llama2-7b-base*",
			ResolvedName:   "llama2-7b-base",
			DefaultVersion: "v1",
			RequiresLoRA:   false,
			Labels: map[string]string{
				"family": "llama2",
				"size":   "7b",
				"type":   "base",
			},
		},
	}

	for pattern, mapping := range defaultMappings {
		r.modelMappings[pattern] = mapping
	}

	log.Printf("初始化 %d 个默认模型映射", len(defaultMappings))
}

// preprocessModelName 预处理模型名
func (r *ModelNameResolver) preprocessModelName(name string) string {
	// 转换为小写
	cleaned := strings.ToLower(strings.TrimSpace(name))

	// 移除常见前缀
	prefixes := []string{"model/", "models/", "inference/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = strings.TrimPrefix(cleaned, prefix)
			break
		}
	}

	// 移除特殊字符
	cleaned = regexp.MustCompile(`[^\w\-\.]`).ReplaceAllString(cleaned, "-")

	return cleaned
}

// extractVersion 从模型名中提取版本
func (r *ModelNameResolver) extractVersion(name string) (string, string) {
	if r.config.VersionPattern == "" {
		return name, ""
	}

	re, err := regexp.Compile(r.config.VersionPattern)
	if err != nil {
		log.Printf("版本正则表达式编译失败: %v", err)
		return name, ""
	}

	matches := re.FindStringSubmatch(name)
	if len(matches) >= 2 {
		version := matches[1]
		// 移除版本后的名称
		nameWithoutVersion := re.ReplaceAllString(name, "")
		nameWithoutVersion = strings.Trim(nameWithoutVersion, "-_")

		return nameWithoutVersion, version
	}

	return name, ""
}

// findExactMapping 查找精确匹配的映射
func (r *ModelNameResolver) findExactMapping(name string) (*ModelMapping, bool) {
	r.mappingsLock.RLock()
	defer r.mappingsLock.RUnlock()

	if mapping, exists := r.modelMappings[name]; exists {
		return mapping, true
	}

	return nil, false
}

// findPatternMapping 查找模式匹配的映射
func (r *ModelNameResolver) findPatternMapping(name string) (*ModelMapping, bool) {
	r.mappingsLock.RLock()
	defer r.mappingsLock.RUnlock()

	for pattern, mapping := range r.modelMappings {
		if r.patternMatches(pattern, name) {
			return mapping, true
		}
	}

	return nil, false
}

// findFuzzyMapping 查找模糊匹配的映射
func (r *ModelNameResolver) findFuzzyMapping(name string) (*ModelMapping, bool) {
	r.mappingsLock.RLock()
	defer r.mappingsLock.RUnlock()

	bestMatch := ""
	var bestMapping *ModelMapping
	maxScore := 0.0

	for pattern, mapping := range r.modelMappings {
		score := r.calculateSimilarity(name, pattern)
		if score > maxScore && score > 0.6 { // 60%相似度阈值
			maxScore = score
			bestMatch = pattern
			bestMapping = mapping
		}
	}

	if bestMatch != "" {
		log.Printf("模糊匹配: %s -> %s (相似度: %.2f)", name, bestMatch, maxScore)
		return bestMapping, true
	}

	return nil, false
}

// patternMatches 检查模式是否匹配
func (r *ModelNameResolver) patternMatches(pattern, name string) bool {
	// 简单通配符匹配
	if strings.Contains(pattern, "*") {
		prefix := strings.Split(pattern, "*")[0]
		return strings.HasPrefix(name, prefix)
	}

	// 正则表达式匹配
	if strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(name)
	}

	return pattern == name
}

// calculateSimilarity 计算字符串相似度
func (r *ModelNameResolver) calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// 简单的编辑距离算法
	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// 计算公共子串
	common := 0
	for i := 0; i < len1 && i < len2; i++ {
		if s1[i] == s2[i] {
			common++
		} else {
			break
		}
	}

	// 计算相似度
	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	return float64(common) / float64(maxLen)
}

// inferCapabilitiesFromName 从模型名推断能力
func (r *ModelNameResolver) inferCapabilitiesFromName(modelName string) *types.EndpointCapability {
	capability := &types.EndpointCapability{
		SupportedModels: []string{modelName},
		MaxConcurrent:   10,
	}

	lowerName := strings.ToLower(modelName)

	// 判断是否支持LoRA
	if strings.Contains(lowerName, "lora") {
		capability.LoRASupport = true
		capability.LoadedLoRAs = []string{modelName}
	}

	// 判断GPU内存需求
	if strings.Contains(lowerName, "7b") {
		capability.GPUMemoryTotal = 16 * 1024 * 1024 * 1024 // 16GB
	} else if strings.Contains(lowerName, "13b") {
		capability.GPUMemoryTotal = 32 * 1024 * 1024 * 1024 // 32GB
	} else if strings.Contains(lowerName, "70b") {
		capability.GPUMemoryTotal = 128 * 1024 * 1024 * 1024 // 128GB
	}

	// 判断KV Cache支持
	capability.KVCacheSupport = true

	return capability
}

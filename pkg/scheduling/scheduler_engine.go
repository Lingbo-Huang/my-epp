package scheduling

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/interfaces"
	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// SchedulerEngine 调度引擎实现
type SchedulerEngine struct {
	// 插件管理
	filterPlugins map[string]interfaces.FilterPlugin
	scorePlugins  map[string]interfaces.ScorePlugin
	bindPlugins   map[string]interfaces.BindPlugin

	pluginsLock sync.RWMutex

	// 策略配置
	policies     []*types.SchedulingPolicy
	policiesLock sync.RWMutex

	// 调度统计
	stats     *SchedulingStats
	statsLock sync.RWMutex

	// 配置
	config *SchedulerConfig
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// 默认算法
	DefaultAlgorithm types.SchedulingAlgorithm `json:"default_algorithm"`

	// 调度超时
	SchedulingTimeout time.Duration `json:"scheduling_timeout"`

	// 并行配置
	EnableParallelScore bool `json:"enable_parallel_score"`
	MaxParallelJobs     int  `json:"max_parallel_jobs"`

	// 缓存配置
	EnableScoreCache bool          `json:"enable_score_cache"`
	ScoreCacheTTL    time.Duration `json:"score_cache_ttl"`

	// 调试配置
	EnableDebugLog bool `json:"enable_debug_log"`
}

// SchedulingStats 调度统计
type SchedulingStats struct {
	TotalRequests       int64 `json:"total_requests"`
	SuccessfulSchedules int64 `json:"successful_schedules"`
	FailedSchedules     int64 `json:"failed_schedules"`

	AverageSchedulingTime time.Duration `json:"average_scheduling_time"`
	MaxSchedulingTime     time.Duration `json:"max_scheduling_time"`

	FilteredEndpoints map[string]int64 `json:"filtered_endpoints"`
	SelectedEndpoints map[string]int64 `json:"selected_endpoints"`

	LastScheduleTime time.Time `json:"last_schedule_time"`
}

// NewSchedulerEngine 创建新的调度引擎
func NewSchedulerEngine(config *SchedulerConfig) *SchedulerEngine {
	if config == nil {
		config = &SchedulerConfig{
			DefaultAlgorithm:    types.SchedulingAlgorithmLeastConnections,
			SchedulingTimeout:   5 * time.Second,
			EnableParallelScore: true,
			MaxParallelJobs:     10,
			EnableScoreCache:    true,
			ScoreCacheTTL:       60 * time.Second,
			EnableDebugLog:      true,
		}
	}

	se := &SchedulerEngine{
		filterPlugins: make(map[string]interfaces.FilterPlugin),
		scorePlugins:  make(map[string]interfaces.ScorePlugin),
		bindPlugins:   make(map[string]interfaces.BindPlugin),
		policies:      make([]*types.SchedulingPolicy, 0),
		stats: &SchedulingStats{
			FilteredEndpoints: make(map[string]int64),
			SelectedEndpoints: make(map[string]int64),
		},
		config: config,
	}

	// 注册默认插件
	se.registerDefaultPlugins()

	log.Printf("调度引擎初始化完成 - 默认算法: %s, 超时: %v",
		config.DefaultAlgorithm.String(), config.SchedulingTimeout)

	return se
}

// Schedule 调度请求到最优端点
func (se *SchedulerEngine) Schedule(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) (*types.EndpointInfo, error) {
	startTime := time.Now()

	se.statsLock.Lock()
	se.stats.TotalRequests++
	se.statsLock.Unlock()

	if se.config.EnableDebugLog {
		log.Printf("开始调度 - 请求ID: %s, 候选端点数: %d", req.ID, len(candidates))
	}

	// 设置调度超时
	scheduleCtx, cancel := context.WithTimeout(ctx, se.config.SchedulingTimeout)
	defer cancel()

	// 第一阶段：过滤
	filteredEndpoints, err := se.runFilters(scheduleCtx, req, candidates)
	if err != nil {
		se.recordFailure("filter_error")
		return nil, fmt.Errorf("过滤阶段失败: %w", err)
	}

	if len(filteredEndpoints) == 0 {
		se.recordFailure("no_candidates")
		return nil, fmt.Errorf("没有符合条件的端点")
	}

	if se.config.EnableDebugLog {
		log.Printf("过滤完成 - 剩余候选: %d", len(filteredEndpoints))
	}

	// 第二阶段：评分
	scoredEndpoints, err := se.runScoring(scheduleCtx, req, filteredEndpoints)
	if err != nil {
		se.recordFailure("scoring_error")
		return nil, fmt.Errorf("评分阶段失败: %w", err)
	}

	// 第三阶段：选择最优端点
	selectedEndpoint, err := se.selectBestEndpoint(req, scoredEndpoints)
	if err != nil {
		se.recordFailure("selection_error")
		return nil, fmt.Errorf("选择端点失败: %w", err)
	}

	// 第四阶段：绑定
	err = se.runBinding(scheduleCtx, req, selectedEndpoint)
	if err != nil {
		log.Printf("绑定失败，但继续处理: %v", err)
	}

	// 记录成功统计
	se.recordSuccess(selectedEndpoint, time.Since(startTime))

	if se.config.EnableDebugLog {
		log.Printf("调度完成 - 选中端点: %s, 耗时: %v",
			selectedEndpoint.ID, time.Since(startTime))
	}

	return selectedEndpoint, nil
}

// UpdatePolicy 更新调度策略
func (se *SchedulerEngine) UpdatePolicy(policy *types.SchedulingPolicy) error {
	se.policiesLock.Lock()
	defer se.policiesLock.Unlock()

	// 查找是否存在同名策略
	for i, p := range se.policies {
		if p.Name == policy.Name {
			se.policies[i] = policy
			log.Printf("更新调度策略: %s", policy.Name)
			return nil
		}
	}

	// 添加新策略
	se.policies = append(se.policies, policy)
	log.Printf("添加新调度策略: %s", policy.Name)

	return nil
}

// RegisterFilterPlugin 注册过滤插件
func (se *SchedulerEngine) RegisterFilterPlugin(plugin interfaces.FilterPlugin) {
	se.pluginsLock.Lock()
	defer se.pluginsLock.Unlock()

	se.filterPlugins[plugin.Name()] = plugin
	log.Printf("注册过滤插件: %s", plugin.Name())
}

// RegisterScorePlugin 注册评分插件
func (se *SchedulerEngine) RegisterScorePlugin(plugin interfaces.ScorePlugin) {
	se.pluginsLock.Lock()
	defer se.pluginsLock.Unlock()

	se.scorePlugins[plugin.Name()] = plugin
	log.Printf("注册评分插件: %s", plugin.Name())
}

// RegisterBindPlugin 注册绑定插件
func (se *SchedulerEngine) RegisterBindPlugin(plugin interfaces.BindPlugin) {
	se.pluginsLock.Lock()
	defer se.pluginsLock.Unlock()

	se.bindPlugins[plugin.Name()] = plugin
	log.Printf("注册绑定插件: %s", plugin.Name())
}

// runFilters 运行过滤插件
func (se *SchedulerEngine) runFilters(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.EndpointInfo, error) {
	// 获取当前策略的过滤器配置
	filterConfigs := se.getFilterConfigs(req)

	filteredEndpoints := candidates

	// 按顺序运行每个过滤器
	for _, config := range filterConfigs {
		if !config.Enabled {
			continue
		}

		se.pluginsLock.RLock()
		plugin, exists := se.filterPlugins[config.Name]
		se.pluginsLock.RUnlock()

		if !exists {
			log.Printf("过滤插件不存在: %s", config.Name)
			continue
		}

		result, err := plugin.Filter(ctx, req, filteredEndpoints)
		if err != nil {
			return nil, fmt.Errorf("过滤插件 %s 执行失败: %w", config.Name, err)
		}

		beforeCount := len(filteredEndpoints)
		filteredEndpoints = result
		afterCount := len(filteredEndpoints)

		se.statsLock.Lock()
		se.stats.FilteredEndpoints[config.Name] += int64(beforeCount - afterCount)
		se.statsLock.Unlock()

		if se.config.EnableDebugLog {
			log.Printf("过滤插件 %s: %d -> %d 端点", config.Name, beforeCount, afterCount)
		}

		// 如果没有端点剩余，提前退出
		if len(filteredEndpoints) == 0 {
			break
		}
	}

	return filteredEndpoints, nil
}

// runScoring 运行评分插件
func (se *SchedulerEngine) runScoring(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo) ([]*types.SchedulingScore, error) {
	// 获取当前策略的评分器配置
	scoreConfigs := se.getScoreConfigs(req)

	if len(scoreConfigs) == 0 {
		return se.createDefaultScores(candidates), nil
	}

	// 并行或串行执行评分
	if se.config.EnableParallelScore {
		return se.runScoringParallel(ctx, req, candidates, scoreConfigs)
	} else {
		return se.runScoringSequential(ctx, req, candidates, scoreConfigs)
	}
}

// runScoringParallel 并行运行评分插件
func (se *SchedulerEngine) runScoringParallel(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo, scoreConfigs []types.ScorerConfig) ([]*types.SchedulingScore, error) {
	scores := make([]*types.SchedulingScore, len(candidates))

	// 初始化分数
	for i, endpoint := range candidates {
		scores[i] = &types.SchedulingScore{
			EndpointID:     endpoint.ID,
			ScoreBreakdown: make(map[string]float64),
		}
	}

	// 为每个评分插件创建goroutine
	var wg sync.WaitGroup
	scoresChan := make(chan struct {
		endpointIdx int
		pluginName  string
		score       float64
		err         error
	}, len(candidates)*len(scoreConfigs))

	for _, config := range scoreConfigs {
		if !config.Enabled {
			continue
		}

		se.pluginsLock.RLock()
		plugin, exists := se.scorePlugins[config.Name]
		se.pluginsLock.RUnlock()

		if !exists {
			log.Printf("评分插件不存在: %s", config.Name)
			continue
		}

		wg.Add(1)
		go func(pluginName string, plugin interfaces.ScorePlugin, weight float64) {
			defer wg.Done()

			for i, endpoint := range candidates {
				result, err := plugin.Score(ctx, req, endpoint)

				scoresChan <- struct {
					endpointIdx int
					pluginName  string
					score       float64
					err         error
				}{i, pluginName, result.TotalScore * weight, err}
			}
		}(config.Name, plugin, config.Weight)
	}

	// 等待所有评分完成
	go func() {
		wg.Wait()
		close(scoresChan)
	}()

	// 收集分数
	for result := range scoresChan {
		if result.err != nil {
			log.Printf("评分插件 %s 执行失败: %v", result.pluginName, result.err)
			continue
		}

		scores[result.endpointIdx].ScoreBreakdown[result.pluginName] = result.score
		scores[result.endpointIdx].TotalScore += result.score
	}

	return scores, nil
}

// runScoringSequential 串行运行评分插件
func (se *SchedulerEngine) runScoringSequential(ctx context.Context, req *types.ProcessingRequest, candidates []*types.EndpointInfo, scoreConfigs []types.ScorerConfig) ([]*types.SchedulingScore, error) {
	scores := make([]*types.SchedulingScore, len(candidates))

	// 初始化分数
	for i, endpoint := range candidates {
		scores[i] = &types.SchedulingScore{
			EndpointID:     endpoint.ID,
			ScoreBreakdown: make(map[string]float64),
		}
	}

	// 逐个运行评分插件
	for _, config := range scoreConfigs {
		if !config.Enabled {
			continue
		}

		se.pluginsLock.RLock()
		plugin, exists := se.scorePlugins[config.Name]
		se.pluginsLock.RUnlock()

		if !exists {
			log.Printf("评分插件不存在: %s", config.Name)
			continue
		}

		// 为每个端点评分
		for i, endpoint := range candidates {
			result, err := plugin.Score(ctx, req, endpoint)
			if err != nil {
				log.Printf("评分插件 %s 对端点 %s 执行失败: %v", config.Name, endpoint.ID, err)
				continue
			}

			weightedScore := result.TotalScore * config.Weight
			scores[i].ScoreBreakdown[config.Name] = weightedScore
			scores[i].TotalScore += weightedScore
		}
	}

	return scores, nil
}

// selectBestEndpoint 选择最优端点
func (se *SchedulerEngine) selectBestEndpoint(req *types.ProcessingRequest, scores []*types.SchedulingScore) (*types.EndpointInfo, error) {
	if len(scores) == 0 {
		return nil, fmt.Errorf("没有可用的评分结果")
	}

	// 排序分数（降序）
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	// 选择最高分的端点
	bestScore := scores[0]

	if se.config.EnableDebugLog {
		log.Printf("最优端点评分: %s -> %.2f", bestScore.EndpointID, bestScore.TotalScore)
		for plugin, score := range bestScore.ScoreBreakdown {
			log.Printf("  %s: %.2f", plugin, score)
		}
	}

	// 这里需要根据ID找到实际的端点对象
	// 简化实现，假设端点ID就是地址
	return &types.EndpointInfo{
		ID:      bestScore.EndpointID,
		Address: bestScore.EndpointID,
		Port:    8000,
		Status:  types.EndpointStatusHealthy,
	}, nil
}

// runBinding 运行绑定插件
func (se *SchedulerEngine) runBinding(ctx context.Context, req *types.ProcessingRequest, endpoint *types.EndpointInfo) error {
	// 获取绑定插件配置
	bindConfigs := se.getBindConfigs(req)

	for _, config := range bindConfigs {
		if !config.Enabled {
			continue
		}

		se.pluginsLock.RLock()
		plugin, exists := se.bindPlugins[config.Name]
		se.pluginsLock.RUnlock()

		if !exists {
			log.Printf("绑定插件不存在: %s", config.Name)
			continue
		}

		err := plugin.Bind(ctx, req, endpoint)
		if err != nil {
			return fmt.Errorf("绑定插件 %s 执行失败: %w", config.Name, err)
		}
	}

	return nil
}

// getFilterConfigs 获取过滤器配置
func (se *SchedulerEngine) getFilterConfigs(req *types.ProcessingRequest) []types.FilterConfig {
	// 简化实现：返回默认过滤器配置
	return []types.FilterConfig{
		{Name: "health", Type: "health", Enabled: true},
		{Name: "capability", Type: "capability", Enabled: true},
		{Name: "load", Type: "load", Enabled: true},
	}
}

// getScoreConfigs 获取评分器配置
func (se *SchedulerEngine) getScoreConfigs(req *types.ProcessingRequest) []types.ScorerConfig {
	// 简化实现：返回默认评分器配置
	return []types.ScorerConfig{
		{Name: "load", Type: "load", Weight: 0.4, Enabled: true},
		{Name: "capability", Type: "capability", Weight: 0.4, Enabled: true},
		{Name: "latency", Type: "latency", Weight: 0.2, Enabled: true},
	}
}

// getBindConfigs 获取绑定器配置
func (se *SchedulerEngine) getBindConfigs(req *types.ProcessingRequest) []types.BinderConfig {
	// 简化实现：返回默认绑定器配置
	return []types.BinderConfig{
		{Name: "affinity", Type: "affinity", Enabled: false},
	}
}

// createDefaultScores 创建默认分数
func (se *SchedulerEngine) createDefaultScores(candidates []*types.EndpointInfo) []*types.SchedulingScore {
	scores := make([]*types.SchedulingScore, len(candidates))

	for i, endpoint := range candidates {
		scores[i] = &types.SchedulingScore{
			EndpointID:     endpoint.ID,
			TotalScore:     50.0, // 默认分数
			ScoreBreakdown: map[string]float64{"default": 50.0},
		}
	}

	return scores
}

// registerDefaultPlugins 注册默认插件
func (se *SchedulerEngine) registerDefaultPlugins() {
	// 注册默认过滤插件
	se.RegisterFilterPlugin(NewHealthFilter())
	se.RegisterFilterPlugin(NewCapabilityFilter())
	se.RegisterFilterPlugin(NewLoadFilter())

	// 注册默认评分插件
	se.RegisterScorePlugin(NewLoadScorer())
	se.RegisterScorePlugin(NewCapabilityScorer())
	se.RegisterScorePlugin(NewLatencyScorer())
}

// recordSuccess 记录成功调度
func (se *SchedulerEngine) recordSuccess(endpoint *types.EndpointInfo, duration time.Duration) {
	se.statsLock.Lock()
	defer se.statsLock.Unlock()

	se.stats.SuccessfulSchedules++
	se.stats.SelectedEndpoints[endpoint.ID]++
	se.stats.LastScheduleTime = time.Now()

	// 更新平均调度时间
	if se.stats.AverageSchedulingTime == 0 {
		se.stats.AverageSchedulingTime = duration
	} else {
		se.stats.AverageSchedulingTime = (se.stats.AverageSchedulingTime*9 + duration) / 10
	}

	// 更新最大调度时间
	if duration > se.stats.MaxSchedulingTime {
		se.stats.MaxSchedulingTime = duration
	}
}

// recordFailure 记录失败调度
func (se *SchedulerEngine) recordFailure(reason string) {
	se.statsLock.Lock()
	defer se.statsLock.Unlock()

	se.stats.FailedSchedules++

	log.Printf("调度失败: %s", reason)
}

// GetStats 获取调度统计
func (se *SchedulerEngine) GetStats() *SchedulingStats {
	se.statsLock.RLock()
	defer se.statsLock.RUnlock()

	// 返回统计副本
	statsCopy := *se.stats

	// 深拷贝map
	statsCopy.FilteredEndpoints = make(map[string]int64)
	for k, v := range se.stats.FilteredEndpoints {
		statsCopy.FilteredEndpoints[k] = v
	}

	statsCopy.SelectedEndpoints = make(map[string]int64)
	for k, v := range se.stats.SelectedEndpoints {
		statsCopy.SelectedEndpoints[k] = v
	}

	return &statsCopy
}

package flow_control

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// SaturationDetector 饱和度检测器实现
type SaturationDetector struct {
	// 配置
	config *SaturationConfig

	// 历史指标数据
	metricsHistory *MetricsHistory
	historyLock    sync.RWMutex

	// 检测状态
	lastCheckTime time.Time
	checkResults  map[string]bool
	resultsLock   sync.RWMutex
}

// SaturationConfig 饱和度检测配置
type SaturationConfig struct {
	// CPU阈值
	CPUThreshold float64 `json:"cpu_threshold"`

	// 内存阈值
	MemoryThreshold float64 `json:"memory_threshold"`

	// 队列长度阈值
	QueueThreshold int `json:"queue_threshold"`

	// 响应时间阈值
	LatencyThreshold time.Duration `json:"latency_threshold"`

	// 错误率阈值
	ErrorRateThreshold float64 `json:"error_rate_threshold"`

	// 检测配置
	CheckInterval time.Duration `json:"check_interval"`
	SampleWindow  time.Duration `json:"sample_window"`
	HistorySize   int           `json:"history_size"`

	// 阈值策略
	ConsecutiveFailures int           `json:"consecutive_failures"`
	RecoveryWaitTime    time.Duration `json:"recovery_wait_time"`
}

// MetricsHistory 指标历史数据
type MetricsHistory struct {
	CPUUsage    []float64       `json:"cpu_usage"`
	MemoryUsage []float64       `json:"memory_usage"`
	QueueSizes  []int           `json:"queue_sizes"`
	Latencies   []time.Duration `json:"latencies"`
	ErrorRates  []float64       `json:"error_rates"`
	Timestamps  []time.Time     `json:"timestamps"`
}

// SaturationMetrics 饱和度指标
type SaturationMetrics struct {
	CPU       float64       `json:"cpu"`
	Memory    float64       `json:"memory"`
	QueueSize int           `json:"queue_size"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
	Timestamp time.Time     `json:"timestamp"`
}

// NewSaturationDetector 创建新的饱和度检测器
func NewSaturationDetector(config *SaturationConfig) *SaturationDetector {
	if config == nil {
		config = &SaturationConfig{
			CPUThreshold:        0.80,            // 80% CPU使用率
			MemoryThreshold:     0.85,            // 85% 内存使用率
			QueueThreshold:      100,             // 队列长度100
			LatencyThreshold:    2 * time.Second, // 2秒响应时间
			ErrorRateThreshold:  0.05,            // 5% 错误率
			CheckInterval:       5 * time.Second,
			SampleWindow:        30 * time.Second,
			HistorySize:         100,
			ConsecutiveFailures: 3,
			RecoveryWaitTime:    60 * time.Second,
		}
	}

	sd := &SaturationDetector{
		config: config,
		metricsHistory: &MetricsHistory{
			CPUUsage:    make([]float64, 0, config.HistorySize),
			MemoryUsage: make([]float64, 0, config.HistorySize),
			QueueSizes:  make([]int, 0, config.HistorySize),
			Latencies:   make([]time.Duration, 0, config.HistorySize),
			ErrorRates:  make([]float64, 0, config.HistorySize),
			Timestamps:  make([]time.Time, 0, config.HistorySize),
		},
		checkResults: make(map[string]bool),
	}

	log.Printf("饱和度检测器初始化完成 - CPU阈值: %.2f%%, 内存阈值: %.2f%%, 检测间隔: %v",
		config.CPUThreshold*100, config.MemoryThreshold*100, config.CheckInterval)

	return sd
}

// CheckSaturation 检查系统饱和度
func (sd *SaturationDetector) CheckSaturation(ctx context.Context) (bool, map[string]float64, error) {
	// 收集当前指标
	metrics, err := sd.collectCurrentMetrics(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("收集指标失败: %w", err)
	}

	// 记录指标历史
	sd.recordMetrics(metrics)

	// 检查各项指标是否超过阈值
	results := sd.checkAllThresholds(metrics)

	// 更新检测结果
	sd.updateCheckResults(results)

	// 判断是否饱和
	saturated := sd.isSaturated(results)

	if saturated {
		log.Printf("系统饱和度检查 - 饱和状态: %v", results)
	}

	return saturated, results, nil
}

// UpdateThresholds 更新饱和度阈值
func (sd *SaturationDetector) UpdateThresholds(config *types.SaturationConfig) error {
	// 更新配置
	sd.config.CPUThreshold = config.CPUThreshold
	sd.config.MemoryThreshold = config.MemoryThreshold
	sd.config.QueueThreshold = config.QueueThreshold
	sd.config.LatencyThreshold = config.LatencyThreshold
	sd.config.ErrorRateThreshold = config.ErrorRateThreshold
	sd.config.CheckInterval = config.CheckInterval
	sd.config.SampleWindow = config.SampleWindow

	log.Printf("饱和度阈值已更新 - CPU: %.2f%%, 内存: %.2f%%, 队列: %d",
		config.CPUThreshold*100, config.MemoryThreshold*100, config.QueueThreshold)

	return nil
}

// collectCurrentMetrics 收集当前系统指标
func (sd *SaturationDetector) collectCurrentMetrics(ctx context.Context) (*SaturationMetrics, error) {
	metrics := &SaturationMetrics{
		Timestamp: time.Now(),
	}

	// 收集CPU使用率
	cpuUsage, err := sd.getCPUUsage()
	if err != nil {
		log.Printf("获取CPU使用率失败: %v", err)
		cpuUsage = 0.0 // 使用默认值
	}
	metrics.CPU = cpuUsage

	// 收集内存使用率
	memUsage, err := sd.getMemoryUsage()
	if err != nil {
		log.Printf("获取内存使用率失败: %v", err)
		memUsage = 0.0 // 使用默认值
	}
	metrics.Memory = memUsage

	// 收集队列大小（这里简化，实际应该从队列系统获取）
	queueSize := sd.getQueueSize()
	metrics.QueueSize = queueSize

	// 收集平均延迟（简化实现）
	latency := sd.getAverageLatency()
	metrics.Latency = latency

	// 收集错误率（简化实现）
	errorRate := sd.getErrorRate()
	metrics.ErrorRate = errorRate

	return metrics, nil
}

// recordMetrics 记录指标到历史数据
func (sd *SaturationDetector) recordMetrics(metrics *SaturationMetrics) {
	sd.historyLock.Lock()
	defer sd.historyLock.Unlock()

	// 添加新指标
	sd.metricsHistory.CPUUsage = append(sd.metricsHistory.CPUUsage, metrics.CPU)
	sd.metricsHistory.MemoryUsage = append(sd.metricsHistory.MemoryUsage, metrics.Memory)
	sd.metricsHistory.QueueSizes = append(sd.metricsHistory.QueueSizes, metrics.QueueSize)
	sd.metricsHistory.Latencies = append(sd.metricsHistory.Latencies, metrics.Latency)
	sd.metricsHistory.ErrorRates = append(sd.metricsHistory.ErrorRates, metrics.ErrorRate)
	sd.metricsHistory.Timestamps = append(sd.metricsHistory.Timestamps, metrics.Timestamp)

	// 保持历史大小限制
	if len(sd.metricsHistory.CPUUsage) > sd.config.HistorySize {
		sd.metricsHistory.CPUUsage = sd.metricsHistory.CPUUsage[1:]
		sd.metricsHistory.MemoryUsage = sd.metricsHistory.MemoryUsage[1:]
		sd.metricsHistory.QueueSizes = sd.metricsHistory.QueueSizes[1:]
		sd.metricsHistory.Latencies = sd.metricsHistory.Latencies[1:]
		sd.metricsHistory.ErrorRates = sd.metricsHistory.ErrorRates[1:]
		sd.metricsHistory.Timestamps = sd.metricsHistory.Timestamps[1:]
	}
}

// checkAllThresholds 检查所有阈值
func (sd *SaturationDetector) checkAllThresholds(metrics *SaturationMetrics) map[string]float64 {
	results := make(map[string]float64)

	// 检查CPU阈值
	if metrics.CPU > sd.config.CPUThreshold {
		results["cpu"] = metrics.CPU
	}

	// 检查内存阈值
	if metrics.Memory > sd.config.MemoryThreshold {
		results["memory"] = metrics.Memory
	}

	// 检查队列长度阈值
	if metrics.QueueSize > sd.config.QueueThreshold {
		results["queue_size"] = float64(metrics.QueueSize)
	}

	// 检查延迟阈值
	if metrics.Latency > sd.config.LatencyThreshold {
		results["latency"] = float64(metrics.Latency.Milliseconds())
	}

	// 检查错误率阈值
	if metrics.ErrorRate > sd.config.ErrorRateThreshold {
		results["error_rate"] = metrics.ErrorRate
	}

	return results
}

// updateCheckResults 更新检测结果
func (sd *SaturationDetector) updateCheckResults(results map[string]float64) {
	sd.resultsLock.Lock()
	defer sd.resultsLock.Unlock()

	sd.lastCheckTime = time.Now()

	// 更新各项指标的检测结果
	for metric := range results {
		sd.checkResults[metric] = true
	}

	// 清除不再超阈值的指标
	for metric := range sd.checkResults {
		if _, exists := results[metric]; !exists {
			sd.checkResults[metric] = false
		}
	}
}

// isSaturated 判断是否处于饱和状态
func (sd *SaturationDetector) isSaturated(results map[string]float64) bool {
	// 如果有任何指标超过阈值，则认为系统饱和
	if len(results) > 0 {
		return true
	}

	// 检查历史趋势（简化实现）
	if sd.checkTrendSaturation() {
		return true
	}

	return false
}

// checkTrendSaturation 检查趋势饱和度
func (sd *SaturationDetector) checkTrendSaturation() bool {
	sd.historyLock.RLock()
	defer sd.historyLock.RUnlock()

	// 检查最近的趋势
	historyLen := len(sd.metricsHistory.CPUUsage)
	if historyLen < 5 {
		return false
	}

	// 检查CPU使用率是否持续上升
	recentCPU := sd.metricsHistory.CPUUsage[historyLen-5:]
	if sd.isTrendIncreasing(recentCPU, sd.config.CPUThreshold*0.9) {
		return true
	}

	// 检查内存使用率是否持续上升
	recentMem := sd.metricsHistory.MemoryUsage[historyLen-5:]
	if sd.isTrendIncreasing(recentMem, sd.config.MemoryThreshold*0.9) {
		return true
	}

	return false
}

// isTrendIncreasing 检查趋势是否上升
func (sd *SaturationDetector) isTrendIncreasing(values []float64, threshold float64) bool {
	if len(values) < 3 {
		return false
	}

	// 简单检查：最后几个值是否呈上升趋势并接近阈值
	last := values[len(values)-1]
	middle := values[len(values)-2]
	first := values[len(values)-3]

	return last > middle && middle > first && last > threshold
}

// getCPUUsage 获取CPU使用率
func (sd *SaturationDetector) getCPUUsage() (float64, error) {
	// 简化实现：使用runtime信息模拟
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 基于goroutine数量模拟CPU使用率
	numGoroutine := float64(runtime.NumGoroutine())
	numCPU := float64(runtime.NumCPU())

	// 简化的CPU使用率计算
	usage := numGoroutine / (numCPU * 100)
	if usage > 1.0 {
		usage = 1.0
	}

	return usage, nil
}

// getMemoryUsage 获取内存使用率
func (sd *SaturationDetector) getMemoryUsage() (float64, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 计算内存使用率
	totalMem := float64(m.Sys)
	usedMem := float64(m.Alloc)

	if totalMem == 0 {
		return 0, nil
	}

	usage := usedMem / totalMem
	return usage, nil
}

// getQueueSize 获取队列大小（简化实现）
func (sd *SaturationDetector) getQueueSize() int {
	// 实际实现中应该从队列系统获取
	// 这里返回模拟值
	return 20
}

// getAverageLatency 获取平均延迟（简化实现）
func (sd *SaturationDetector) getAverageLatency() time.Duration {
	// 实际实现中应该从指标系统获取
	// 这里返回模拟值
	return 150 * time.Millisecond
}

// getErrorRate 获取错误率（简化实现）
func (sd *SaturationDetector) getErrorRate() float64 {
	// 实际实现中应该从指标系统获取
	// 这里返回模拟值
	return 0.01 // 1%错误率
}

// GetCurrentMetrics 获取当前指标
func (sd *SaturationDetector) GetCurrentMetrics() (*SaturationMetrics, error) {
	return sd.collectCurrentMetrics(context.Background())
}

// GetHistory 获取历史指标
func (sd *SaturationDetector) GetHistory() *MetricsHistory {
	sd.historyLock.RLock()
	defer sd.historyLock.RUnlock()

	// 返回历史数据副本
	historyCopy := &MetricsHistory{
		CPUUsage:    make([]float64, len(sd.metricsHistory.CPUUsage)),
		MemoryUsage: make([]float64, len(sd.metricsHistory.MemoryUsage)),
		QueueSizes:  make([]int, len(sd.metricsHistory.QueueSizes)),
		Latencies:   make([]time.Duration, len(sd.metricsHistory.Latencies)),
		ErrorRates:  make([]float64, len(sd.metricsHistory.ErrorRates)),
		Timestamps:  make([]time.Time, len(sd.metricsHistory.Timestamps)),
	}

	copy(historyCopy.CPUUsage, sd.metricsHistory.CPUUsage)
	copy(historyCopy.MemoryUsage, sd.metricsHistory.MemoryUsage)
	copy(historyCopy.QueueSizes, sd.metricsHistory.QueueSizes)
	copy(historyCopy.Latencies, sd.metricsHistory.Latencies)
	copy(historyCopy.ErrorRates, sd.metricsHistory.ErrorRates)
	copy(historyCopy.Timestamps, sd.metricsHistory.Timestamps)

	return historyCopy
}

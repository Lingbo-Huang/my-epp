package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/flow_control"
	"github.com/Lingbo-Huang/my-epp/pkg/request_processing"
	"github.com/Lingbo-Huang/my-epp/pkg/routing"
	"github.com/Lingbo-Huang/my-epp/pkg/scheduling"
	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// MockMetricsReporter 模拟指标上报器
type MockMetricsReporter struct{}

func (m *MockMetricsReporter) ReportCounter(name string, value int64, labels map[string]string) error {
	log.Printf("[METRICS] Counter: %s = %d, Labels: %v", name, value, labels)
	return nil
}

func (m *MockMetricsReporter) ReportGauge(name string, value float64, labels map[string]string) error {
	log.Printf("[METRICS] Gauge: %s = %.2f, Labels: %v", name, value, labels)
	return nil
}

func (m *MockMetricsReporter) ReportHistogram(name string, value float64, labels map[string]string) error {
	log.Printf("[METRICS] Histogram: %s = %.2f, Labels: %v", name, value, labels)
	return nil
}

// AdvancedEPPServer 高级EPP服务器
type AdvancedEPPServer struct {
	// 各层组件
	requestProcessor   *request_processing.RequestProcessor
	extProcServer      *request_processing.ExtProcServer
	router             *routing.Director
	modelNameResolver  *routing.ModelNameResolver
	trafficSplitter    *routing.TrafficSplitter
	flowController     *flow_control.FlowController
	queueSystem        *flow_control.QueueSystem
	saturationDetector *flow_control.SaturationDetector
	scheduler          *scheduling.SchedulerEngine
	metricsReporter    *MockMetricsReporter

	// 服务控制
	shutdownChan chan os.Signal
	wg           sync.WaitGroup
}

func main() {
	log.Println("🚀 启动高级EPP服务器...")

	// 创建服务器实例
	server := NewAdvancedEPPServer()

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("❌ 启动服务器失败: %v", err)
	}

	log.Println("✅ 高级EPP服务器已启动")

	// 等待关闭信号
	server.WaitForShutdown()

	log.Println("👋 高级EPP服务器已关闭")
}

// NewAdvancedEPPServer 创建新的高级EPP服务器
func NewAdvancedEPPServer() *AdvancedEPPServer {
	server := &AdvancedEPPServer{
		metricsReporter: &MockMetricsReporter{},
		shutdownChan:    make(chan os.Signal, 1),
	}

	// 初始化各层组件
	server.initializeComponents()

	return server
}

// initializeComponents 初始化各层组件
func (s *AdvancedEPPServer) initializeComponents() {
	log.Println("🔧 初始化EPP各层组件...")

	// 1. 初始化路由层
	log.Println("📍 初始化路由层...")
	s.modelNameResolver = routing.NewModelNameResolver(nil)
	s.trafficSplitter = routing.NewTrafficSplitter(nil)
	s.router = routing.NewDirector(s.modelNameResolver, s.trafficSplitter, nil)

	// 添加一些示例路由策略
	s.setupRoutingPolicies()

	// 2. 初始化流控层
	log.Println("🚦 初始化流控层...")
	s.queueSystem = flow_control.NewQueueSystem(nil)
	s.saturationDetector = flow_control.NewSaturationDetector(nil)
	s.flowController = flow_control.NewFlowController(s.queueSystem, s.saturationDetector, nil)

	// 添加一些示例流控策略
	s.setupFlowControlPolicies()

	// 3. 初始化调度层
	log.Println("🎯 初始化调度层...")
	s.scheduler = scheduling.NewSchedulerEngine(nil)

	// 添加一些示例调度策略
	s.setupSchedulingPolicies()

	// 4. 初始化请求处理层
	log.Println("⚡ 初始化请求处理层...")
	requestHandler := request_processing.NewRequestHandler()
	responseHandler := request_processing.NewResponseHandler()

	s.requestProcessor = request_processing.NewRequestProcessor(
		s.router,
		s.flowController,
		s.scheduler,
		s.metricsReporter,
		nil,
	)

	s.extProcServer = request_processing.NewExtProcServer(
		":9002",
		s.requestProcessor,
		requestHandler,
		responseHandler,
		s.metricsReporter,
	)

	log.Println("✅ EPP各层组件初始化完成")
}

// setupRoutingPolicies 设置路由策略
func (s *AdvancedEPPServer) setupRoutingPolicies() {
	// 示例策略：LoRA模型路由
	loraPolicy := &types.RoutingPolicy{
		Name:    "lora-routing",
		Enabled: true,
		Rules: []types.RoutingRule{
			{
				Name:     "lora-models",
				Priority: 10,
				Conditions: []types.PolicyCondition{
					{
						Field:    "lora_required",
						Operator: "eq",
						Value:    true,
					},
				},
				Target: "lora-endpoints",
				Weight: 1.0,
			},
		},
	}

	s.router.UpdatePolicy(loraPolicy)

	log.Println("📍 路由策略设置完成")
}

// setupFlowControlPolicies 设置流控策略
func (s *AdvancedEPPServer) setupFlowControlPolicies() {
	// 示例策略：基本流控
	basicPolicy := &types.FlowControlPolicy{
		Name:    "basic-flow-control",
		Enabled: true,
		RateLimit: &types.RateLimitConfig{
			MaxRequests: 100,
			TimeWindow:  time.Minute,
			BurstSize:   20,
		},
		QueueConfig: &types.QueueConfig{
			MaxSize:     50,
			MaxWaitTime: 30 * time.Second,
			Strategy:    types.QueueStrategyPriority,
		},
	}

	s.flowController.UpdatePolicy(basicPolicy)

	log.Println("🚦 流控策略设置完成")
}

// setupSchedulingPolicies 设置调度策略
func (s *AdvancedEPPServer) setupSchedulingPolicies() {
	// 示例策略：负载均衡调度
	loadBalancePolicy := &types.SchedulingPolicy{
		Name:      "load-balance",
		Enabled:   true,
		Algorithm: types.SchedulingAlgorithmLeastConnections,
		Filters: []types.FilterConfig{
			{Name: "health", Type: "health", Enabled: true},
			{Name: "capability", Type: "capability", Enabled: true},
			{Name: "load", Type: "load", Enabled: true},
		},
		Scorers: []types.ScorerConfig{
			{Name: "load", Type: "load", Weight: 0.5, Enabled: true},
			{Name: "capability", Type: "capability", Weight: 0.3, Enabled: true},
			{Name: "latency", Type: "latency", Weight: 0.2, Enabled: true},
		},
	}

	s.scheduler.UpdatePolicy(loadBalancePolicy)

	log.Println("🎯 调度策略设置完成")
}

// Start 启动服务器
func (s *AdvancedEPPServer) Start() error {
	// 设置信号处理
	signal.Notify(s.shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动ExtProc服务器
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		log.Println("🌐 启动ExtProc服务器...")
		if err := s.extProcServer.Start(); err != nil {
			log.Printf("❌ ExtProc服务器启动失败: %v", err)
		}
	}()

	// 启动监控和统计任务
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runMonitoringTasks()
	}()

	// 等待服务启动
	time.Sleep(100 * time.Millisecond)

	return nil
}

// runMonitoringTasks 运行监控任务
func (s *AdvancedEPPServer) runMonitoringTasks() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("📊 启动监控任务...")

	for {
		select {
		case <-ticker.C:
			s.reportSystemStats()
		case <-s.shutdownChan:
			return
		}
	}
}

// reportSystemStats 上报系统统计
func (s *AdvancedEPPServer) reportSystemStats() {
	log.Println("📈 系统状态报告:")

	// 流控统计
	if flowState := s.flowController.GetState(); flowState != nil {
		log.Printf("  🚦 流控层: 总请求=%d, 接受=%d, 拒绝=%d, 排队=%d",
			flowState.TotalRequests, flowState.AcceptedRequests,
			flowState.RejectedRequests, flowState.QueuedRequests)
	}

	// 队列统计
	if queueStats := s.queueSystem.GetStats(); queueStats != nil {
		log.Printf("  📥 队列系统: 当前大小=%d, 平均等待=%v, 超时=%d",
			queueStats.CurrentSize, queueStats.AverageWaitTime, queueStats.TotalTimedOut)
	}

	// 调度统计
	if schedStats := s.scheduler.GetStats(); schedStats != nil {
		log.Printf("  🎯 调度引擎: 总调度=%d, 成功=%d, 失败=%d, 平均耗时=%v",
			schedStats.TotalRequests, schedStats.SuccessfulSchedules,
			schedStats.FailedSchedules, schedStats.AverageSchedulingTime)
	}

	// 饱和度检测
	if detector := s.saturationDetector; detector != nil {
		if metrics, err := detector.GetCurrentMetrics(); err == nil {
			log.Printf("  💻 系统指标: CPU=%.1f%%, 内存=%.1f%%, 队列=%d",
				metrics.CPU*100, metrics.Memory*100, metrics.QueueSize)
		}
	}
}

// WaitForShutdown 等待关闭信号
func (s *AdvancedEPPServer) WaitForShutdown() {
	<-s.shutdownChan
	log.Println("🛑 收到关闭信号，正在优雅关闭...")

	// 停止ExtProc服务器
	s.extProcServer.Stop()

	// 清空队列
	s.queueSystem.Clear()

	// 等待所有goroutine结束
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// 等待最多10秒
	select {
	case <-done:
		log.Println("✅ 所有组件已优雅关闭")
	case <-time.After(10 * time.Second):
		log.Println("⚠️  关闭超时，强制退出")
	}
}

package request_processing

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/interfaces"
	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// RequestProcessor EPP请求处理器的核心实现
type RequestProcessor struct {
	// 依赖的组件
	router          interfaces.Router
	flowController  interfaces.FlowController
	scheduler       interfaces.Scheduler
	metricsReporter interfaces.MetricsReporter

	// 配置
	config *ProcessorConfig
}

// ProcessorConfig 处理器配置
type ProcessorConfig struct {
	// 超时配置
	DefaultTimeout time.Duration `json:"default_timeout"`
	MaxTimeout     time.Duration `json:"max_timeout"`

	// 重试配置
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`

	// 指标配置
	EnableMetrics bool `json:"enable_metrics"`
}

// NewRequestProcessor 创建新的请求处理器
func NewRequestProcessor(
	router interfaces.Router,
	flowController interfaces.FlowController,
	scheduler interfaces.Scheduler,
	metricsReporter interfaces.MetricsReporter,
	config *ProcessorConfig,
) *RequestProcessor {
	if config == nil {
		config = &ProcessorConfig{
			DefaultTimeout: 30 * time.Second,
			MaxTimeout:     300 * time.Second,
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			EnableMetrics:  true,
		}
	}

	return &RequestProcessor{
		router:          router,
		flowController:  flowController,
		scheduler:       scheduler,
		metricsReporter: metricsReporter,
		config:          config,
	}
}

// ProcessRequest 处理请求的主要入口
func (p *RequestProcessor) ProcessRequest(ctx context.Context, req *types.ProcessingRequest) (*types.ProcessingResponse, error) {
	startTime := time.Now()

	log.Printf("开始处理请求 - ID: %s, 模型: %s, LoRA: %v",
		req.ID, req.ModelName, req.LoRARequired)

	// 上报请求开始指标
	if p.config.EnableMetrics && p.metricsReporter != nil {
		p.metricsReporter.ReportCounter("epp_requests_total", 1, map[string]string{
			"model":         req.ModelName,
			"lora_required": fmt.Sprintf("%v", req.LoRARequired),
		})
		p.metricsReporter.ReportGauge("epp_active_requests", 1, nil)
		defer func() {
			p.metricsReporter.ReportGauge("epp_active_requests", -1, nil)
		}()
	}

	// 设置请求上下文超时
	processCtx, cancel := context.WithTimeout(ctx, p.getEffectiveTimeout(req))
	defer cancel()

	// 第一步：流控检查
	flowControlInfo, err := p.flowController.ShouldAllow(processCtx, req)
	if err != nil {
		return p.buildErrorResponse(req, "流控检查失败", err, startTime)
	}

	// 如果不允许通过，返回相应状态
	if !flowControlInfo.Allowed {
		log.Printf("请求被流控拒绝 - ID: %s, 原因: %s", req.ID, flowControlInfo.Reason)

		if p.config.EnableMetrics && p.metricsReporter != nil {
			p.metricsReporter.ReportCounter("epp_requests_rejected_total", 1, map[string]string{
				"reason": "flow_control",
			})
		}

		status := types.StatusRejected
		if flowControlInfo.QueuePosition > 0 {
			status = types.StatusQueued
		}

		return &types.ProcessingResponse{
			RequestID:       req.ID,
			Status:          status,
			FlowControlInfo: flowControlInfo,
			ProcessingTime:  time.Since(startTime),
			Error:           flowControlInfo.Reason,
		}, nil
	}

	log.Printf("流控检查通过 - ID: %s", req.ID)

	// 第二步：路由决策
	routingDecision, err := p.router.Route(processCtx, req)
	if err != nil {
		return p.buildErrorResponse(req, "路由决策失败", err, startTime)
	}

	log.Printf("路由决策完成 - ID: %s, 策略: %s, 原因: %s",
		req.ID, routingDecision.Strategy, routingDecision.Reason)

	// 第三步：端点调度
	// 这里需要先获取可用端点列表，然后让调度器选择最优端点
	endpoints, err := p.getAvailableEndpoints(req)
	if err != nil {
		return p.buildErrorResponse(req, "获取端点列表失败", err, startTime)
	}

	if len(endpoints) == 0 {
		log.Printf("没有可用端点 - ID: %s", req.ID)

		if p.config.EnableMetrics && p.metricsReporter != nil {
			p.metricsReporter.ReportCounter("epp_requests_failed_total", 1, map[string]string{
				"reason": "no_endpoints",
			})
		}

		return &types.ProcessingResponse{
			RequestID:       req.ID,
			Status:          types.StatusFailed,
			RoutingDecision: routingDecision,
			FlowControlInfo: flowControlInfo,
			ProcessingTime:  time.Since(startTime),
			Error:           "没有可用的端点",
		}, nil
	}

	// 调度器选择最优端点
	selectedEndpoint, err := p.scheduler.Schedule(processCtx, req, endpoints)
	if err != nil {
		return p.buildErrorResponse(req, "端点调度失败", err, startTime)
	}

	log.Printf("端点调度完成 - ID: %s, 选中端点: %s:%d",
		req.ID, selectedEndpoint.Address, selectedEndpoint.Port)

	// 上报成功指标
	if p.config.EnableMetrics && p.metricsReporter != nil {
		p.metricsReporter.ReportCounter("epp_requests_success_total", 1, map[string]string{
			"model":    req.ModelName,
			"endpoint": selectedEndpoint.ID,
		})
		p.metricsReporter.ReportHistogram("epp_request_duration_seconds",
			time.Since(startTime).Seconds(), map[string]string{
				"model": req.ModelName,
			})
	}

	// 构建成功响应
	return &types.ProcessingResponse{
		RequestID:        req.ID,
		SelectedEndpoint: selectedEndpoint,
		RoutingDecision:  routingDecision,
		FlowControlInfo:  flowControlInfo,
		Status:           types.StatusSuccess,
		ProcessingTime:   time.Since(startTime),
	}, nil
}

// buildErrorResponse 构建错误响应
func (p *RequestProcessor) buildErrorResponse(req *types.ProcessingRequest, stage string, err error, startTime time.Time) (*types.ProcessingResponse, error) {
	log.Printf("%s - ID: %s, 错误: %v", stage, req.ID, err)

	if p.config.EnableMetrics && p.metricsReporter != nil {
		p.metricsReporter.ReportCounter("epp_requests_failed_total", 1, map[string]string{
			"reason": stage,
		})
	}

	return &types.ProcessingResponse{
		RequestID:      req.ID,
		Status:         types.StatusFailed,
		ProcessingTime: time.Since(startTime),
		Error:          fmt.Sprintf("%s: %v", stage, err),
	}, nil
}

// getEffectiveTimeout 获取有效的超时时间
func (p *RequestProcessor) getEffectiveTimeout(req *types.ProcessingRequest) time.Duration {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = p.config.DefaultTimeout
	}

	// 限制最大超时时间
	if timeout > p.config.MaxTimeout {
		timeout = p.config.MaxTimeout
	}

	return timeout
}

// getAvailableEndpoints 获取可用端点列表
// 这里暂时从请求元数据中获取，实际应该从数据层获取
func (p *RequestProcessor) getAvailableEndpoints(req *types.ProcessingRequest) ([]*types.EndpointInfo, error) {
	// 从请求元数据中提取端点列表（演示用）
	if endpointsList, exists := req.Metadata["available_endpoints"]; exists {
		if endpoints, ok := endpointsList.([]string); ok {
			var result []*types.EndpointInfo

			for i, endpointAddr := range endpoints {
				// 解析端点地址
				address, port, err := p.parseEndpointAddress(endpointAddr)
				if err != nil {
					log.Printf("解析端点地址失败: %s, 错误: %v", endpointAddr, err)
					continue
				}

				endpoint := &types.EndpointInfo{
					ID:      fmt.Sprintf("endpoint_%d", i),
					Name:    endpointAddr,
					Address: address,
					Port:    port,
					Status:  types.EndpointStatusHealthy,

					// 模拟能力信息
					Capabilities: &types.EndpointCapability{
						SupportedModels: []string{req.ModelName},
						LoRASupport:     true,
						MaxConcurrent:   10,
					},

					// 模拟指标信息
					Metrics: &types.EndpointMetrics{
						ActiveRequests: p.mockGetActiveRequests(endpointAddr),
						CPUUsage:       p.mockGetCPUUsage(endpointAddr),
						AvgLatency:     time.Duration(50+i*10) * time.Millisecond,
						LastUpdated:    time.Now(),
					},
				}

				// 根据端点名设置LoRA能力
				if req.LoRARequired {
					if endpointAddr == "pod-llama2-lora-1:8000" || endpointAddr == "pod-llama2-lora-2:8000" {
						endpoint.Capabilities.LoadedLoRAs = []string{req.ModelName}
					}
				}

				result = append(result, endpoint)
			}

			return result, nil
		}
	}

	return nil, fmt.Errorf("无法获取端点列表")
}

// parseEndpointAddress 解析端点地址
func (p *RequestProcessor) parseEndpointAddress(addr string) (string, int, error) {

	// 假设都是8000端口（演示用）
	if addr == "pod-llama2-lora-1:8000" {
		return "pod-llama2-lora-1", 8000, nil
	} else if addr == "pod-llama2-lora-2:8000" {
		return "pod-llama2-lora-2", 8000, nil
	} else if addr == "pod-llama2-base:8000" {
		return "pod-llama2-base", 8000, nil
	} else if addr == "pod-llama2-base-2:8000" {
		return "pod-llama2-base-2", 8000, nil
	}

	return addr, 8000, nil
}

// mockGetActiveRequests 模拟获取活跃请求数
func (p *RequestProcessor) mockGetActiveRequests(endpoint string) int {
	// 模拟不同端点的负载
	switch endpoint {
	case "pod-llama2-lora-1:8000":
		return 2
	case "pod-llama2-lora-2:8000":
		return 5
	case "pod-llama2-base:8000":
		return 3
	case "pod-llama2-base-2:8000":
		return 0 // 模拟无负载数据
	default:
		return 1
	}
}

// mockGetCPUUsage 模拟获取CPU使用率
func (p *RequestProcessor) mockGetCPUUsage(endpoint string) float64 {
	switch endpoint {
	case "pod-llama2-lora-1:8000":
		return 0.3
	case "pod-llama2-lora-2:8000":
		return 0.7
	case "pod-llama2-base:8000":
		return 0.5
	default:
		return 0.2
	}
}

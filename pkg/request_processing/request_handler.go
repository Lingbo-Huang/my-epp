package request_processing

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// RequestHandler 请求处理器实现
type RequestHandler struct{}

// NewRequestHandler 创建新的请求处理器
func NewRequestHandler() *RequestHandler {
	return &RequestHandler{}
}

// ParseRequest 解析Envoy请求为内部处理请求
func (h *RequestHandler) ParseRequest(ctx context.Context, rawReq interface{}) (*types.ProcessingRequest, error) {
	// 类型断言
	extProcReq, ok := rawReq.(*extprocv3.ProcessingRequest)
	if !ok {
		return nil, fmt.Errorf("无效的请求类型")
	}

	// 获取请求头
	headers := extProcReq.GetRequestHeaders()
	if headers == nil {
		return nil, fmt.Errorf("请求头为空")
	}

	// 生成请求ID
	requestID := h.generateRequestID()

	// 解析请求头
	req := &types.ProcessingRequest{
		ID:          requestID,
		Context:     ctx,
		Headers:     make(map[string]string),
		Metadata:    make(map[string]interface{}),
		Parameters:  make(map[string]interface{}),
		ProcessTime: time.Now(),
	}

	// 提取标准HTTP头
	for _, header := range headers.Headers.Headers {
		key := strings.ToLower(header.Key)
		value := header.Value
		req.Headers[key] = value

		log.Printf("解析请求头: %s = %s", key, value)

		// 解析特定的EPP相关头部
		switch key {
		case "x-model-name":
			req.ModelName = value
		case "x-model-version":
			req.ModelVersion = value
		case "x-lora-required":
			req.LoRARequired = strings.ToLower(value) == "true"
		case "x-lora-adapter":
			req.LoRAAdapter = value
		case "x-request-priority":
			if priority, err := strconv.Atoi(value); err == nil {
				req.Priority = priority
			}
		case "x-max-retries":
			if retries, err := strconv.Atoi(value); err == nil {
				req.MaxRetries = retries
			}
		case "x-timeout":
			if timeout, err := time.ParseDuration(value); err == nil {
				req.Timeout = timeout
			}
		case "content-type":
			req.Parameters["content_type"] = value
		}
	}

	// 解析端点列表（从自定义头部）
	if endpointsHeader := req.Headers["x-available-endpoints"]; endpointsHeader != "" {
		endpoints := strings.Split(endpointsHeader, ",")
		req.Metadata["available_endpoints"] = endpoints
		log.Printf("提取端点列表：%s", strings.Join(endpoints, ","))
	}

	// 解析请求路径
	if path := req.Headers[":path"]; path != "" {
		req.Metadata["path"] = path
		// 从路径中提取模型名（如果未在头部指定）
		if req.ModelName == "" {
			modelName := h.extractModelFromPath(path)
			if modelName != "" {
				req.ModelName = modelName
				log.Printf("从路径提取模型名: %s", modelName)
			}
		}
	}

	// 解析查询参数
	if query := req.Headers["x-query-params"]; query != "" {
		params := h.parseQueryParams(query)
		for k, v := range params {
			req.Parameters[k] = v
		}
	}

	// 设置默认值
	h.setDefaults(req)

	// 验证请求
	if err := h.ValidateRequest(req); err != nil {
		return nil, fmt.Errorf("请求验证失败: %w", err)
	}

	log.Printf("成功解析请求 - ID: %s, Model: %s, LoRA: %v",
		req.ID, req.ModelName, req.LoRARequired)

	return req, nil
}

// ValidateRequest 验证请求的有效性
func (h *RequestHandler) ValidateRequest(req *types.ProcessingRequest) error {
	// 检查必填字段
	if req.ID == "" {
		return fmt.Errorf("请求ID不能为空")
	}

	if req.ModelName == "" {
		return fmt.Errorf("模型名称不能为空")
	}

	// 检查LoRA配置
	if req.LoRARequired && req.LoRAAdapter == "" {
		// 尝试从模型名推断LoRA适配器名
		if strings.Contains(strings.ToLower(req.ModelName), "lora") {
			req.LoRAAdapter = req.ModelName
		} else {
			log.Printf("警告: LoRA需求为true但未指定适配器名称")
		}
	}

	// 检查优先级范围
	if req.Priority < 0 || req.Priority > 10 {
		req.Priority = 5 // 设置默认优先级
	}

	// 检查超时时间
	if req.Timeout <= 0 {
		req.Timeout = 30 * time.Second // 默认30秒
	}

	return nil
}

// generateRequestID 生成唯一的请求ID
func (h *RequestHandler) generateRequestID() string {
	return fmt.Sprintf("req_%d_%s",
		time.Now().UnixNano(),
		h.randomString(8))
}

// randomString 生成随机字符串
func (h *RequestHandler) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// extractModelFromPath 从请求路径中提取模型名
func (h *RequestHandler) extractModelFromPath(path string) string {
	// 示例路径: /v1/models/llama2-7b/completions
	parts := strings.Split(strings.Trim(path, "/"), "/")

	for i, part := range parts {
		if part == "models" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// 尝试其他模式
	if len(parts) >= 2 && parts[0] == "inference" {
		return parts[1]
	}

	return ""
}

// parseQueryParams 解析查询参数
func (h *RequestHandler) parseQueryParams(query string) map[string]interface{} {
	params := make(map[string]interface{})

	pairs := strings.Split(query, "&")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := kv[0]
			value := kv[1]

			// 尝试转换为数字
			if intVal, err := strconv.Atoi(value); err == nil {
				params[key] = intVal
			} else if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
				params[key] = floatVal
			} else if boolVal, err := strconv.ParseBool(value); err == nil {
				params[key] = boolVal
			} else {
				params[key] = value
			}
		}
	}

	return params
}

// setDefaults 设置默认值
func (h *RequestHandler) setDefaults(req *types.ProcessingRequest) {
	if req.Priority == 0 {
		req.Priority = 5
	}

	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	if req.QueuedTime.IsZero() {
		req.QueuedTime = time.Now()
	}
}

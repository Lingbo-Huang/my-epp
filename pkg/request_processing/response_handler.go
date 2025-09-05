package request_processing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typesv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

// ResponseHandler 响应处理器实现
type ResponseHandler struct{}

// NewResponseHandler 创建新的响应处理器
func NewResponseHandler() *ResponseHandler {
	return &ResponseHandler{}
}

// BuildResponse 构建内部响应结构
func (h *ResponseHandler) BuildResponse(ctx context.Context, req *types.ProcessingRequest, result interface{}) (*types.ProcessingResponse, error) {
	// 类型断言获取处理结果
	processingResult, ok := result.(*types.ProcessingResponse)
	if !ok {
		return nil, fmt.Errorf("无效的处理结果类型")
	}

	// 完善响应信息
	if processingResult.RequestID == "" {
		processingResult.RequestID = req.ID
	}

	// 添加响应头
	if processingResult.Headers == nil {
		processingResult.Headers = make(map[string]string)
	}

	// 设置标准响应头
	processingResult.Headers["X-EPP-Request-ID"] = req.ID
	processingResult.Headers["X-EPP-Processing-Time"] = processingResult.ProcessingTime.String()

	if processingResult.SelectedEndpoint != nil {
		processingResult.Headers["X-EPP-Selected-Endpoint"] = processingResult.SelectedEndpoint.Address
		processingResult.Headers["X-EPP-Selected-Port"] = fmt.Sprintf("%d", processingResult.SelectedEndpoint.Port)

		log.Printf("响应发送成功：模型 [%s] -> 端点 [%s:%d]",
			req.ModelName,
			processingResult.SelectedEndpoint.Address,
			processingResult.SelectedEndpoint.Port)
	}

	// 添加路由决策信息
	if processingResult.RoutingDecision != nil {
		decisionJSON, _ := json.Marshal(processingResult.RoutingDecision)
		processingResult.Headers["X-EPP-Routing-Decision"] = string(decisionJSON)
	}

	// 添加流控信息
	if processingResult.FlowControlInfo != nil {
		flowJSON, _ := json.Marshal(processingResult.FlowControlInfo)
		processingResult.Headers["X-EPP-Flow-Control"] = string(flowJSON)
	}

	return processingResult, nil
}

// FormatResponse 格式化为ExtProc协议响应
func (h *ResponseHandler) FormatResponse(resp *types.ProcessingResponse) (interface{}, error) {
	switch resp.Status {
	case types.StatusSuccess:
		return h.buildSuccessResponse(resp)
	case types.StatusQueued:
		return h.buildQueuedResponse(resp)
	case types.StatusRejected:
		return h.buildRejectedResponse(resp)
	case types.StatusFailed:
		return h.buildFailedResponse(resp)
	default:
		return h.buildFailedResponse(resp)
	}
}

// buildSuccessResponse 构建成功响应
func (h *ResponseHandler) buildSuccessResponse(resp *types.ProcessingResponse) (*extprocv3.ProcessingResponse, error) {
	var headerMutations []*corev3.HeaderValueOption

	// 添加所有响应头
	for key, value := range resp.Headers {
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   key,
				Value: value,
			},
			AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}

	// 如果选中了端点，设置路由目标
	if resp.SelectedEndpoint != nil {
		// 添加端点路由信息
		targetCluster := fmt.Sprintf("%s_%d", resp.SelectedEndpoint.Address, resp.SelectedEndpoint.Port)
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   "X-EPP-Target-Cluster",
				Value: targetCluster,
			},
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})

		// 设置上游主机
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   "X-EPP-Upstream-Host",
				Value: fmt.Sprintf("%s:%d", resp.SelectedEndpoint.Address, resp.SelectedEndpoint.Port),
			},
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					Status: extprocv3.CommonResponse_CONTINUE,
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: headerMutations,
					},
				},
			},
		},
	}, nil
}

// buildQueuedResponse 构建排队响应
func (h *ResponseHandler) buildQueuedResponse(resp *types.ProcessingResponse) (*extprocv3.ProcessingResponse, error) {
	var headerMutations []*corev3.HeaderValueOption

	// 添加排队相关头部
	headerMutations = append(headerMutations, &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:   "X-EPP-Status",
			Value: "queued",
		},
		AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
	})

	if resp.FlowControlInfo != nil && resp.FlowControlInfo.QueuePosition > 0 {
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   "X-EPP-Queue-Position",
				Value: fmt.Sprintf("%d", resp.FlowControlInfo.QueuePosition),
			},
			AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})

		if resp.FlowControlInfo.EstimatedWait > 0 {
			headerMutations = append(headerMutations, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:   "X-EPP-Estimated-Wait",
					Value: resp.FlowControlInfo.EstimatedWait.String(),
				},
				AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
			})
		}
	}

	// 返回202 Accepted状态
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typesv3.HttpStatus{Code: typesv3.StatusCode_Accepted},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: headerMutations,
				},
				Body: []byte("请求已排队等待处理"),
			},
		},
	}, nil
}

// buildRejectedResponse 构建拒绝响应
func (h *ResponseHandler) buildRejectedResponse(resp *types.ProcessingResponse) (*extprocv3.ProcessingResponse, error) {
	var headerMutations []*corev3.HeaderValueOption

	// 添加拒绝原因
	headerMutations = append(headerMutations, &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:   "X-EPP-Status",
			Value: "rejected",
		},
		AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
	})

	if resp.FlowControlInfo != nil && resp.FlowControlInfo.Reason != "" {
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   "X-EPP-Reject-Reason",
				Value: resp.FlowControlInfo.Reason,
			},
			AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}

	reasonBody := "请求被拒绝"
	if resp.Error != "" {
		reasonBody = fmt.Sprintf("请求被拒绝: %s", resp.Error)
	}

	// 返回429 Too Many Requests状态
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typesv3.HttpStatus{Code: typesv3.StatusCode_TooManyRequests},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: headerMutations,
				},
				Body: []byte(reasonBody),
			},
		},
	}, nil
}

// buildFailedResponse 构建失败响应
func (h *ResponseHandler) buildFailedResponse(resp *types.ProcessingResponse) (*extprocv3.ProcessingResponse, error) {
	var headerMutations []*corev3.HeaderValueOption

	// 添加失败状态
	headerMutations = append(headerMutations, &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:   "X-EPP-Status",
			Value: "failed",
		},
		AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
	})

	if resp.Error != "" {
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   "X-EPP-Error",
				Value: resp.Error,
			},
			AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}

	errorBody := "内部处理错误"
	if resp.Error != "" {
		errorBody = fmt.Sprintf("处理失败: %s", resp.Error)
	}

	// 返回500 Internal Server Error状态
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typesv3.HttpStatus{Code: typesv3.StatusCode_InternalServerError},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: headerMutations,
				},
				Body: []byte(errorBody),
			},
		},
	}, nil
}

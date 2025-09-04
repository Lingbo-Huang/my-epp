package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	corev3 "github.com/Lingbo-Huang/my-epp/envoy/config/core/v3" // corev3 包（包含 HeaderValue/HeaderValueOption）
	extprocv3 "github.com/Lingbo-Huang/my-epp/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// 自定义 EPP 服务结构体
type myExternalProcessorServer struct {
	extprocv3.UnimplementedExternalProcessorServer
	podLoadMap map[string]int // 模拟端点负载数据
}

// 初始化服务：加载模拟负载
func newMyExternalProcessorServer() *myExternalProcessorServer {
	return &myExternalProcessorServer{
		podLoadMap: map[string]int{
			"pod-llama2-lora-1:8000": 2,
			"pod-llama2-lora-2:8000": 8,
			"pod-llama2-base:8000":   5,
		},
	}
}

// 选择最优端点（业务逻辑不变）
func (s *myExternalProcessorServer) pickOptimalEndpoint(endpointSubset []string, needLoRA bool) string {
	if len(endpointSubset) == 0 {
		log.Println("警告：无可用端点，使用默认值")
		return "default-llama2:8000"
	}

	bestEndpoint := ""
	minLoad := 9999
	for _, endpoint := range endpointSubset {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}

		load, exists := s.podLoadMap[endpoint]
		if !exists {
			log.Printf("警告：端点 [%s] 无负载数据，跳过", endpoint)
			continue
		}

		if needLoRA && !strings.Contains(strings.ToLower(endpoint), "lora") {
			continue
		}

		if load < minLoad {
			minLoad = load
			bestEndpoint = endpoint
		}
	}

	if bestEndpoint == "" {
		bestEndpoint = endpointSubset[0]
		log.Printf("警告：无符合条件端点，fallback 到 [%s]", bestEndpoint)
	}
	log.Printf("选中最优端点：[%s]（负载：%d）", bestEndpoint, minLoad)
	return bestEndpoint
}

// Process：核心流处理接口（仅修正 HeaderMutation.SetHeaders 部分）
func (s *myExternalProcessorServer) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	log.Println("新 Envoy 流连接建立")
	defer log.Println("Envoy 流连接关闭")

	for {
		// 1. 接收 Envoy 请求
		req, err := stream.Recv()
		if err == io.EOF {
			log.Println("接收请求结束（EOF）")
			return nil
		}
		if err != nil {
			log.Printf("接收请求错误：%v", err)
			return fmt.Errorf("recv error: %w", err)
		}

		// 2. 仅处理请求头类型
		httpReq := req.GetRequestHeaders()
		if httpReq == nil {
			log.Println("非请求头类型，返回空响应")
			if err := stream.Send(&extprocv3.ProcessingResponse{}); err != nil {
				return fmt.Errorf("send empty resp error: %w", err)
			}
			continue
		}

		// 3. 获取请求头列表（你的定义：返回 []*corev3.HeaderValue）
		headerMap := httpReq.GetHeaders()
		if headerMap == nil {
			log.Println("请求头为空，返回空响应")
			if err := stream.Send(&extprocv3.ProcessingResponse{}); err != nil {
				return fmt.Errorf("send empty resp error: %w", err)
			}
			continue
		}

		// 4. 提取核心参数（端点列表、LoRA 需求、模型名）
		endpointSubsetStr := ""
		needLoRA := false
		modelName := "unknown"
		// 直接遍历 HeaderValue 切片（匹配你的 GetHeaders() 返回类型）
		for _, header := range headerMap.GetHeaders() {
			if header == nil {
				continue
			}
			headerKey := strings.ToLower(header.GetKey())
			headerValue := header.GetValue()

			switch headerKey {
			case "x-gateway-destination-endpoint-subset":
				endpointSubsetStr = headerValue
				log.Printf("提取端点列表：%s", endpointSubsetStr)
			case "x-model-need-lora":
				needLoRA = strings.ToLower(headerValue) == "true"
				log.Printf("提取 LoRA 需求：%v", needLoRA)
			case "x-model-name":
				modelName = headerValue
				log.Printf("提取模型名：%s", modelName)
			}
		}

		// 5. 处理端点列表（清理空值）
		endpointSubset := strings.Split(endpointSubsetStr, ",")
		cleanedEndpoints := make([]string, 0, len(endpointSubset))
		for _, ep := range endpointSubset {
			if epTrimmed := strings.TrimSpace(ep); epTrimmed != "" {
				cleanedEndpoints = append(cleanedEndpoints, epTrimmed)
			}
		}

		// 6. 选择最优端点
		chosenEndpoint := s.pickOptimalEndpoint(cleanedEndpoints, needLoRA)

		// 7. 构造响应：核心修正！将 HeaderValue 封装为 HeaderValueOption（匹配 SetHeaders 类型）
		resp := &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{
							// 关键修正：SetHeaders 要求 []*HeaderValueOption，因此用 HeaderValueOption 包裹 HeaderValue
							// 若需指定操作类型（如追加），可添加 Operation: corev3.HeaderOperation_APPEND
							SetHeaders: []*corev3.HeaderValueOption{
								{
									// Header 字段：赋值为 *corev3.HeaderValue（你的键值对）
									Header: &corev3.HeaderValue{
										Key:   "x-gateway-destination-endpoint",
										Value: chosenEndpoint,
									},
									// 可选：指定操作类型（默认是替换，可省略；若需追加，设置为 APPEND）
									// Operation: corev3.HeaderOperation_APPEND,
								},
							},
						},
					},
				},
			},
			// 动态元数据（不变）
			DynamicMetadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"epp.chosen_endpoint": structpb.NewStringValue(chosenEndpoint),
					"epp.model_name":      structpb.NewStringValue(modelName),
					"epp.need_lora":       structpb.NewBoolValue(needLoRA),
				},
			},
		}

		// 8. 发送响应
		if err := stream.Send(resp); err != nil {
			log.Printf("发送响应错误：%v", err)
			return fmt.Errorf("send resp error: %w", err)
		}
		log.Printf("响应发送成功：模型 [%s] -> 端点 [%s]", modelName, chosenEndpoint)
	}
}

func main() {
	// 监听端口
	lis, err := net.Listen("tcp", ":9002")
	if err != nil {
		log.Fatalf("监听失败：%v", err)
	}
	defer lis.Close()

	// 创建 gRPC 服务
	grpcServer := grpc.NewServer()
	eppServer := newMyExternalProcessorServer()
	extprocv3.RegisterExternalProcessorServer(grpcServer, eppServer)

	// 启动服务
	log.Println("EPP 服务启动：:9002")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}

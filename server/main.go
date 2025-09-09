package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
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
			"pod-llama2-base-2":      6,
		},
	}
}

// 选择最优端点
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
		if !exists { // 没有这个名字的端点
			// 如果一个端点在 Envoy 的服务发现中存在，但我们的负载监控系统（这里是 podLoadMap）里没有它的数据，这意味着这个端点的状态是未知的。它可能刚刚启动，也可能已经崩溃但尚未从服务发现中移除。
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

	if bestEndpoint == "" { // 可能是因为 needLoRA 过滤掉了所有端点，没有符合条件的
		// 实际生产中可以选择一个默认的、专门用于处理 fallback 的端点池
		bestEndpoint = endpointSubset[0]
		log.Printf("警告：无符合条件端点，fallback 到 [%s]", bestEndpoint)
	}
	log.Printf("选中最优端点：[%s]（负载：%d）", bestEndpoint, minLoad)
	return bestEndpoint
}

// Process：核心流处理接口
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

		// 3. 获取请求头列表
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

		// 7. 构造响应（指令）
		resp := &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestHeaders{ // 针对“请求头”阶段的响应
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{ // 动态路由的关键：设置新的目标端点头
							SetHeaders: []*corev3.HeaderValueOption{
								{
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
			// 动态元数据（信息）
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

	// 创建 gRPC 服务
	grpcServer := grpc.NewServer()
	eppServer := newMyExternalProcessorServer()
	extprocv3.RegisterExternalProcessorServer(grpcServer, eppServer)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		// 阻塞等待信号（收到信号前会一直卡在这里）
		sig := <-sigChan
		log.Printf("\n收到退出信号：%v，开始优雅停止服务...", sig)

		// 步骤1：优雅停止 gRPC 服务（停止接收新请求，等待正在处理的请求完成）
		grpcServer.GracefulStop()
		log.Println("gRPC 服务已停止（所有正在处理的请求已完成）")

		// 步骤2：关闭监听端口（释放 9002 端口）
		if err := lis.Close(); err != nil {
			log.Printf("关闭监听端口失败：%v", err)
		} else {
			log.Println("监听端口 :9002 已关闭，端口已释放")
		}

		// 步骤3：清理其他资源（比如关闭数据库连接、释放缓存等）
		// 例如：if s.podLoadMap 是从数据库加载的，这里可以关闭数据库连接

		// 信号处理完成，退出信号通道（主进程会随之退出）
		close(sigChan)
	}()
	// 启动服务
	log.Println("EPP 服务启动：:9002")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
	<-sigChan
	log.Println("服务已优雅停止，程序退出")
}

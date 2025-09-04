package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	corev3 "github.com/Lingbo-Huang/my-epp/envoy/config/core/v3"
	extprocv3 "github.com/Lingbo-Huang/my-epp/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// 构造 Ext-Proc 请求头消息（模拟 Envoy 发送的请求）
// 参数：端点列表（逗号分隔字符串）、是否需要 LoRA、模型名
func buildRequestHeaders(endpointSubsetStr string, needLoRA bool, modelName string) *extprocv3.ProcessingRequest {
	// 1. 构造请求头列表（corev3.HeaderMap 类型，包含关键头字段）
	headerMap := &corev3.HeaderMap{
		Headers: []*corev3.HeaderValue{
			{
				Key:   "x-gateway-destination-endpoint-subset",
				Value: endpointSubsetStr,
			},
			{
				Key:   "x-model-need-lora",
				Value: fmt.Sprintf("%t", needLoRA),
			},
			{
				Key:   "x-model-name",
				Value: modelName,
			},
			// 可选：添加其他模拟 Envoy 头（如 Host、User-Agent）
			{
				Key:   "host",
				Value: "llama2-api-gateway",
			},
			{
				Key:   "user-agent",
				Value: "envoy-ext-proc-client/test", // **修正：将 "value" 改为 Value**
			},
		},
	}

	// 2. 构造“请求头阶段”的 ProcessingRequest（Ext-Proc 协议要求）
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{ // **核心修正：将 HttpRequestHeaders 改为 HttpHeaders**
				Headers: headerMap,
			},
		},
	}
}

// 解析服务端响应，提取选中的端点和元数据 (此函数无需修改)
func parseResponse(resp *extprocv3.ProcessingResponse) (string, string, bool, error) {
	// 1. 确认响应类型为“请求头响应”
	respHeaders, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	if !ok {
		return "", "", false, fmt.Errorf("响应类型错误：非请求头响应（实际类型：%T）", resp.Response)
	}

	// 2. 提取“选中的端点”（从 HeaderMutation.SetHeaders 中获取）
	commonResp := respHeaders.RequestHeaders.Response
	if commonResp == nil || commonResp.HeaderMutation == nil {
		return "", "", false, fmt.Errorf("响应中无 HeaderMutation 信息")
	}

	chosenEndpoint := ""
	for _, headerOpt := range commonResp.HeaderMutation.SetHeaders {
		if headerOpt.Header == nil {
			continue
		}
		if headerOpt.Header.Key == "x-gateway-destination-endpoint" {
			chosenEndpoint = headerOpt.Header.Value
			break
		}
	}
	if chosenEndpoint == "" {
		return "", "", false, fmt.Errorf("响应中未找到 x-gateway-destination-endpoint 头")
	}

	// 3. 提取动态元数据（epp.model_name、epp.need_lora）
	modelName := "unknown"
	needLoRA := false
	if resp.DynamicMetadata != nil && resp.DynamicMetadata.Fields != nil {
		if modelNameField, exists := resp.DynamicMetadata.Fields["epp.model_name"]; exists {
			modelName = modelNameField.GetStringValue()
		}
		if needLoRAField, exists := resp.DynamicMetadata.Fields["epp.need_lora"]; exists {
			needLoRA = needLoRAField.GetBoolValue()
		}
	}

	return chosenEndpoint, modelName, needLoRA, nil
}

// 客户端主逻辑：连接 EPP 服务，发送请求，接收响应 (此函数无需修改)
func runClient(eppServerAddr string) error {
	// 1. 连接 EPP 服务
	conn, err := grpc.Dial(
		eppServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("连接 EPP 服务失败（%s）：%w", eppServerAddr, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("关闭连接错误：%v", err)
		}
	}()
	log.Printf("成功连接 EPP 服务：%s", eppServerAddr)

	// 2. 创建 EPP 客户端 stub
	client := extprocv3.NewExternalProcessorClient(conn)

	// 3. 建立双向流
	stream, err := client.Process(context.Background())
	if err != nil {
		return fmt.Errorf("创建 stream 失败：%w", err)
	}
	log.Println("成功建立双向流，开始发送请求")

	// 4. 构造测试请求
	testCases := []struct {
		endpointSubsetStr string
		needLoRA          bool
		modelName         string
	}{
		{
			endpointSubsetStr: "pod-llama2-lora-1:8000,pod-llama2-lora-2:8000,pod-llama2-base:8000",
			needLoRA:          true,
			modelName:         "llama2-7b-lora",
		},
		{
			endpointSubsetStr: "pod-llama2-lora-1:8000,pod-llama2-lora-2:8000,pod-llama2-base:8000",
			needLoRA:          false,
			modelName:         "llama2-7b-base",
		},
		{
			endpointSubsetStr: "pod-llama2-base:8000,pod-llama2-base-2:8000",
			needLoRA:          true,
			modelName:         "llama2-7b-lora",
		},
	}

	// 5. 循环发送测试请求并接收响应
	for i, testCase := range testCases {
		log.Printf("\n=== 开始测试场景 %d：模型=%s，需要 LoRA=%v，端点列表=%s ===",
			i+1, testCase.modelName, testCase.needLoRA, testCase.endpointSubsetStr)

		req := buildRequestHeaders(testCase.endpointSubsetStr, testCase.needLoRA, testCase.modelName)
		if err := stream.Send(req); err != nil {
			return fmt.Errorf("场景 %d 发送请求失败：%w", i+1, err)
		}
		log.Println("请求已发送，等待服务端响应...")

		resp, err := stream.Recv()
		if err == io.EOF {
			log.Printf("场景 %d 接收响应结束（EOF）", i+1)
			continue
		}
		if err != nil {
			return fmt.Errorf("场景 %d 接收响应失败：%w", i+1, err)
		}

		chosenEndpoint, modelName, needLoRA, err := parseResponse(resp)
		if err != nil {
			log.Printf("场景 %d 解析响应失败：%v", i+1, err)
			continue
		}

		log.Printf("场景 %d 响应解析成功：", i+1)
		log.Printf("  - 选中的端点：%s", chosenEndpoint)
		log.Printf("  - 模型名：%s", modelName)
		log.Printf("  - LoRA 需求：%v", needLoRA)
	}

	// 6. 发送 EOF 告知服务端流结束
	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("关闭 stream 发送端失败：%w", err)
	}
	log.Println("\n所有测试场景执行完成，客户端退出")

	return nil
}

func main() {
	eppServerAddr := "localhost:9002"
	if err := runClient(eppServerAddr); err != nil {
		log.Fatalf("客户端执行失败：%v", err)
	}
}

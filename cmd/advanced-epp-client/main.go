package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	log.Println("🚀 启动高级EPP客户端演示...")

	// 连接到EPP服务器
	conn, err := grpc.Dial("localhost:9002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("❌ 连接EPP服务器失败: %v", err)
	}
	defer conn.Close()

	client := extprocv3.NewExternalProcessorClient(conn)

	// 运行多个演示场景
	scenarios := []struct {
		name         string
		modelName    string
		loraRequired bool
		loraAdapter  string
		userID       string
		priority     int
		endpoints    []string
	}{
		{
			name:         "高优先级LoRA推理",
			modelName:    "llama2-7b-lora",
			loraRequired: true,
			loraAdapter:  "llama2-7b-lora",
			userID:       "user-premium-001",
			priority:     8,
			endpoints:    []string{"pod-llama2-lora-1:8000", "pod-llama2-lora-2:8000"},
		},
		{
			name:         "普通Base模型推理",
			modelName:    "llama2-7b-base",
			loraRequired: false,
			userID:       "user-standard-002",
			priority:     5,
			endpoints:    []string{"pod-llama2-base:8000", "pod-llama2-lora-1:8000"},
		},
		{
			name:         "流控测试：快速连续请求",
			modelName:    "llama2-7b",
			loraRequired: false,
			userID:       "user-batch-003",
			priority:     3,
			endpoints:    []string{"pod-llama2-base:8000", "pod-llama2-base-2:8000"},
		},
		{
			name:         "调度测试：负载均衡",
			modelName:    "llama2-13b",
			loraRequired: false,
			userID:       "user-test-004",
			priority:     6,
			endpoints:    []string{"pod-llama2-13b-1:8000", "pod-llama2-13b-2:8000", "pod-llama2-13b-3:8000"},
		},
		{
			name:         "LoRA Fallback场景",
			modelName:    "llama2-7b-lora",
			loraRequired: true,
			loraAdapter:  "custom-lora-adapter",
			userID:       "user-research-005",
			priority:     7,
			endpoints:    []string{"pod-llama2-base:8000", "pod-llama2-base-2:8000"},
		},
	}

	// 执行每个场景
	for i, scenario := range scenarios {
		log.Printf("\n🎬 场景 %d: %s", i+1, scenario.name)
		log.Printf("   模型: %s, LoRA: %v, 用户: %s, 优先级: %d",
			scenario.modelName, scenario.loraRequired, scenario.userID, scenario.priority)

		err := runScenario(client, scenario)
		if err != nil {
			log.Printf("❌ 场景执行失败: %v", err)
		} else {
			log.Printf("✅ 场景执行完成")
		}

		// 场景间延迟
		time.Sleep(2 * time.Second)
	}

	// 流控压力测试
	log.Printf("\n🔥 开始流控压力测试...")
	runRateLimitTest(client, scenarios[0])

	log.Println("\n🎉 所有演示场景完成！")
}

// runScenario 运行单个演示场景
func runScenario(client extprocv3.ExternalProcessorClient, scenario struct {
	name         string
	modelName    string
	loraRequired bool
	loraAdapter  string
	userID       string
	priority     int
	endpoints    []string
}) error {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 建立流连接
	stream, err := client.Process(ctx)
	if err != nil {
		return err
	}
	defer stream.CloseSend()

	// 构造请求头
	headers := []*corev3.HeaderValue{
		{Key: ":method", Value: "POST"},
		{Key: ":path", Value: "/v1/chat/completions"},
		{Key: ":authority", Value: "api.example.com"},
		{Key: "content-type", Value: "application/json"},
		{Key: "x-model-name", Value: scenario.modelName},
		{Key: "x-user-id", Value: scenario.userID},
		{Key: "x-request-priority", Value: fmt.Sprintf("%d", scenario.priority)},
		{Key: "x-available-endpoints", Value: joinStrings(scenario.endpoints, ",")},
	}

	if scenario.loraRequired {
		headers = append(headers, &corev3.HeaderValue{
			Key: "x-lora-required", Value: "true",
		})
		if scenario.loraAdapter != "" {
			headers = append(headers, &corev3.HeaderValue{
				Key: "x-lora-adapter", Value: scenario.loraAdapter,
			})
		}
	}

	// 发送请求
	req := &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: headers,
				},
			},
		},
	}

	if err := stream.Send(req); err != nil {
		return err
	}

	// 接收响应
	resp, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	// 解析响应
	if immediateResp := resp.GetImmediateResponse(); immediateResp != nil {
		log.Printf("   📋 立即响应: 状态码=%d", immediateResp.Status.Code)
		if immediateResp.Body != nil {
			log.Printf("   📄 响应体: %s", string(immediateResp.Body))
		}
	} else if headerResp := resp.GetRequestHeaders(); headerResp != nil {
		log.Printf("   📤 头部响应: 状态=%s", headerResp.Response.Status.String())

		// 显示添加的头部
		if headerResp.Response.HeaderMutation != nil {
			for _, header := range headerResp.Response.HeaderMutation.SetHeaders {
				if header.Header.Key == "X-EPP-Selected-Endpoint" {
					log.Printf("   🎯 选中端点: %s", header.Header.Value)
				}
				if header.Header.Key == "X-EPP-Upstream-Host" {
					log.Printf("   🔗 上游主机: %s", header.Header.Value)
				}
				if header.Header.Key == "X-EPP-Processing-Time" {
					log.Printf("   ⏱️  处理时间: %s", header.Header.Value)
				}
			}
		}
	}

	return nil
}

// runRateLimitTest 运行限流测试
func runRateLimitTest(client extprocv3.ExternalProcessorClient, scenario struct {
	name         string
	modelName    string
	loraRequired bool
	loraAdapter  string
	userID       string
	priority     int
	endpoints    []string
}) {

	log.Printf("   📊 发送连续请求测试流控...")

	successCount := 0
	rejectedCount := 0
	queuedCount := 0

	// 快速发送20个请求
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		stream, err := client.Process(ctx)
		if err != nil {
			cancel()
			continue
		}

		// 构造快速请求
		headers := []*corev3.HeaderValue{
			{Key: ":method", Value: "POST"},
			{Key: ":path", Value: fmt.Sprintf("/test/%d", i)},
			{Key: "x-model-name", Value: scenario.modelName},
			{Key: "x-user-id", Value: fmt.Sprintf("rate-test-user-%d", i%3)}, // 3个不同用户
			{Key: "x-available-endpoints", Value: "pod-test:8000"},
		}

		req := &extprocv3.ProcessingRequest{
			Request: &extprocv3.ProcessingRequest_RequestHeaders{
				RequestHeaders: &extprocv3.HttpHeaders{
					Headers: &corev3.HeaderMap{Headers: headers},
				},
			},
		}

		if err := stream.Send(req); err != nil {
			stream.CloseSend()
			cancel()
			continue
		}

		resp, err := stream.Recv()
		stream.CloseSend()
		cancel()

		if err != nil {
			continue
		}

		// 统计响应类型
		if immediateResp := resp.GetImmediateResponse(); immediateResp != nil {
			switch immediateResp.Status.Code {
			case 202:
				queuedCount++
				log.Printf("   ⏳ 请求 %d: 已排队", i+1)
			case 429:
				rejectedCount++
				log.Printf("   🚫 请求 %d: 被限流", i+1)
			default:
				successCount++
				log.Printf("   ✅ 请求 %d: 成功", i+1)
			}
		} else {
			successCount++
			log.Printf("   ✅ 请求 %d: 通过", i+1)
		}

		// 短暂延迟
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("   📈 流控测试结果: 成功=%d, 排队=%d, 拒绝=%d",
		successCount, queuedCount, rejectedCount)
}

// joinStrings 连接字符串数组
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

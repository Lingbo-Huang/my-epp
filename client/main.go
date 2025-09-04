package main

import (
	"context"
	"log"
	"time"

	extprocv3 "github.com/Lingbo-Huang/my-epp/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := extprocv3.NewExternalProcessorClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		log.Fatalf("could not create stream: %v", err)
	}

	// 构造并发送一个 ProcessingRequest
	req := &extprocv3.ProcessingRequest{
		// 这里只做最简单的请求，实际可根据 proto 定义填充
		ObservabilityMode: false,
	}
	if err := stream.Send(req); err != nil {
		log.Fatalf("failed to send request: %v", err)
	}
	log.Println("Request sent.")

	// 关闭发送方向
	if err := stream.CloseSend(); err != nil {
		log.Fatalf("failed to close send: %v", err)
	}

	// 接收响应
	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Printf("stream closed or error: %v", err)
			break
		}
		log.Printf("Received response: %+v", resp)
	}
}

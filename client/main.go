package main

import (
	"context"
	"log"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := extprocv3.NewExternalProcessorClient(conn)

	// 构造请求
	req := &extprocv3.ExternalProcessorRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, err := client.Process(ctx, req)
	if err != nil {
		log.Fatalf("could not process: %v", err)
	}
	log.Printf("Response: %+v", resp)
}

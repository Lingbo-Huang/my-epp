package main

import (
	"fmt"
	"io"
	"log"
	"net"

	"context"
	extprocv3 "github.com/Lingbo-Huang/my-epp/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

type myExternalProcessorServer struct {
	extprocv3.UnimplementedExternalProcessorServer
}

func (s *myExternalProcessorServer) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("Stream closed by client.")
			return nil
		}
		if err != nil {
			log.Printf("Error receiving from stream: %v", err)
			return err
		}
		fmt.Printf("Received request: %+v\n", req)

		// 构造简单响应
		resp := &extprocv3.ProcessingResponse{}
		if err := stream.Send(resp); err != nil {
			log.Printf("Error sending response: %v", err)
			return err
		}
		fmt.Println("Response sent.")
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcServer, &myExternalProcessorServer{})
	log.Println("gRPC server listening at :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

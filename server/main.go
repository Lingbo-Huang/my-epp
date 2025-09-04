package main

import (
	"context"
	"fmt"
	"log"
	"net"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

type myExternalProcessorServer struct {
	extprocv3.UnimplementedExternalProcessorServer
}

func (s *myExternalProcessorServer) Process(context.Context, *extprocv3.ExternalProcessorRequest) (*extprocv3.ExternalProcessorResponse, error) {
	// TODO: 这里写你自己的逻辑
	fmt.Println("Received a request!")
	return &extprocv3.ExternalProcessorResponse{}, nil
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

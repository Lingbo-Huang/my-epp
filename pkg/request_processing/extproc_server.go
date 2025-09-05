package request_processing

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/Lingbo-Huang/my-epp/pkg/interfaces"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typesv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc"
)

// ExtProcServer EPP的External Processing服务器
type ExtProcServer struct {
	extprocv3.UnimplementedExternalProcessorServer

	// 依赖的组件
	requestProcessor interfaces.RequestProcessor
	requestHandler   interfaces.RequestHandler
	responseHandler  interfaces.ResponseHandler
	metricsReporter  interfaces.MetricsReporter

	// 配置
	addr   string
	server *grpc.Server
}

// NewExtProcServer 创建新的ExtProcServer
func NewExtProcServer(
	addr string,
	requestProcessor interfaces.RequestProcessor,
	requestHandler interfaces.RequestHandler,
	responseHandler interfaces.ResponseHandler,
	metricsReporter interfaces.MetricsReporter,
) *ExtProcServer {
	return &ExtProcServer{
		addr:             addr,
		requestProcessor: requestProcessor,
		requestHandler:   requestHandler,
		responseHandler:  responseHandler,
		metricsReporter:  metricsReporter,
	}
}

// Start 启动ExtProcServer
func (s *ExtProcServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.server = grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(s.server, s)

	log.Printf("EPP ExtProc服务启动在 %s", s.addr)

	// 上报启动指标
	if s.metricsReporter != nil {
		s.metricsReporter.ReportCounter("epp_server_starts_total", 1, map[string]string{
			"addr": s.addr,
		})
	}

	return s.server.Serve(listener)
}

// Stop 停止ExtProcServer
func (s *ExtProcServer) Stop() {
	if s.server != nil {
		log.Println("正在停止EPP ExtProc服务...")
		s.server.GracefulStop()
	}
}

// Process 实现ExtProcServer的核心处理逻辑
func (s *ExtProcServer) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	log.Println("新的Envoy流连接建立")

	// 上报连接指标
	if s.metricsReporter != nil {
		s.metricsReporter.ReportCounter("epp_connections_total", 1, nil)
	}

	defer func() {
		log.Println("Envoy流连接关闭")
		if s.metricsReporter != nil {
			s.metricsReporter.ReportCounter("epp_connections_closed_total", 1, nil)
		}
	}()

	for {
		// 接收来自Envoy的请求
		req, err := stream.Recv()
		if err == io.EOF {
			log.Println("接收请求结束（EOF）")
			return nil
		}
		if err != nil {
			log.Printf("接收请求失败: %v", err)
			if s.metricsReporter != nil {
				s.metricsReporter.ReportCounter("epp_receive_errors_total", 1, nil)
			}
			return err
		}

		// 处理请求
		resp, err := s.processRequest(stream.Context(), req)
		if err != nil {
			log.Printf("处理请求失败: %v", err)
			if s.metricsReporter != nil {
				s.metricsReporter.ReportCounter("epp_processing_errors_total", 1, nil)
			}
			// 返回错误响应
			resp = s.buildErrorResponse(req, err)
		}

		// 发送响应给Envoy
		if err := stream.Send(resp); err != nil {
			log.Printf("发送响应失败: %v", err)
			if s.metricsReporter != nil {
				s.metricsReporter.ReportCounter("epp_send_errors_total", 1, nil)
			}
			return err
		}

		// 上报处理成功指标
		if s.metricsReporter != nil {
			s.metricsReporter.ReportCounter("epp_requests_processed_total", 1, nil)
		}

		log.Println("请求处理完成")
	}
}

// processRequest 处理单个请求
func (s *ExtProcServer) processRequest(ctx context.Context, req *extprocv3.ProcessingRequest) (*extprocv3.ProcessingResponse, error) {
	// 只处理请求头阶段
	if req.GetRequestHeaders() == nil {
		log.Println("跳过非请求头阶段")
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_ImmediateResponse{
				ImmediateResponse: &extprocv3.ImmediateResponse{
					Status: &typesv3.HttpStatus{Code: typesv3.StatusCode_OK},
				},
			},
		}, nil
	}

	log.Printf("处理请求头：%v", req.GetRequestHeaders())

	// 解析请求
	processingReq, err := s.requestHandler.ParseRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("解析请求失败: %w", err)
	}

	// 处理业务逻辑
	result, err := s.requestProcessor.ProcessRequest(ctx, processingReq)
	if err != nil {
		return nil, fmt.Errorf("处理业务逻辑失败: %w", err)
	}

	// 构建响应
	response, err := s.responseHandler.FormatResponse(result)
	if err != nil {
		return nil, fmt.Errorf("构建响应失败: %w", err)
	}

	extProcResp, ok := response.(*extprocv3.ProcessingResponse)
	if !ok {
		return nil, fmt.Errorf("响应格式错误")
	}

	return extProcResp, nil
}

// buildErrorResponse 构建错误响应
func (s *ExtProcServer) buildErrorResponse(req *extprocv3.ProcessingRequest, err error) *extprocv3.ProcessingResponse {
	log.Printf("构建错误响应: %v", err)

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typesv3.HttpStatus{Code: typesv3.StatusCode_InternalServerError},
				Body:   []byte(fmt.Sprintf("EPP处理错误: %v", err)),
			},
		},
	}
}

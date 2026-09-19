package interceptor

import (
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

// ServerWithoutContent retains go-zero's timing metrics and load shedding while
// excluding every service method from request logging. Deriving the list from
// generated descriptors also covers future methods without a YAML allowlist.
func ServerWithoutContent(c zrpc.RpcServerConf, services ...*grpc.ServiceDesc) zrpc.RpcServerConf {
	methods := append([]string(nil), c.Middlewares.StatConf.IgnoreContentMethods...)
	for _, service := range services {
		for _, method := range service.Methods {
			methods = append(methods, "/"+service.ServiceName+"/"+method.MethodName)
		}
		for _, stream := range service.Streams {
			methods = append(methods, "/"+service.ServiceName+"/"+stream.StreamName)
		}
	}
	c.Middlewares.StatConf.IgnoreContentMethods = methods
	return c
}

// NewClient applies the logging policy to every RPC destination, including
// optional services. The stock duration interceptor logs requests on failures
// and request/response bodies on slow calls, so it must always be replaced.
func NewClient(c zrpc.RpcClientConf, opts ...zrpc.ClientOption) (zrpc.Client, error) {
	c.Middlewares.Duration = false
	options := append([]zrpc.ClientOption{
		zrpc.WithUnaryClientInterceptor(SafeDurationUnaryClientInterceptor()),
	}, opts...)
	return zrpc.NewClient(c, options...)
}

func MustNewClient(c zrpc.RpcClientConf, opts ...zrpc.ClientOption) zrpc.Client {
	client, err := NewClient(c, opts...)
	logx.Must(err)
	return client
}

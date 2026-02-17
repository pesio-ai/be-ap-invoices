package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// forwardMetadata is a gRPC unary client interceptor that propagates
// incoming request metadata (including the Bearer auth token) to outgoing
// service-to-service calls. Without this, the user's JWT would be stripped
// when ap-invoices calls ap-vendors, gl-accounts, etc.
func forwardMetadata(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

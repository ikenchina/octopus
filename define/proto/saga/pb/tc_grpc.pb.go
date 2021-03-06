// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.6.1
// source: tc.proto

package pb

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// TcClient is the client API for Tc service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TcClient interface {
	NewGtid(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*SagaResponse, error)
	Commit(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error)
	Get(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error)
}

type tcClient struct {
	cc grpc.ClientConnInterface
}

func NewTcClient(cc grpc.ClientConnInterface) TcClient {
	return &tcClient{cc}
}

func (c *tcClient) NewGtid(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*SagaResponse, error) {
	out := new(SagaResponse)
	err := c.cc.Invoke(ctx, "/saga.Tc/NewGtid", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tcClient) Commit(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error) {
	out := new(SagaResponse)
	err := c.cc.Invoke(ctx, "/saga.Tc/Commit", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tcClient) Get(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error) {
	out := new(SagaResponse)
	err := c.cc.Invoke(ctx, "/saga.Tc/Get", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TcServer is the server API for Tc service.
// All implementations must embed UnimplementedTcServer
// for forward compatibility
type TcServer interface {
	NewGtid(context.Context, *empty.Empty) (*SagaResponse, error)
	Commit(context.Context, *SagaRequest) (*SagaResponse, error)
	Get(context.Context, *SagaRequest) (*SagaResponse, error)
	mustEmbedUnimplementedTcServer()
}

// UnimplementedTcServer must be embedded to have forward compatible implementations.
type UnimplementedTcServer struct {
}

func (UnimplementedTcServer) NewGtid(context.Context, *empty.Empty) (*SagaResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NewGtid not implemented")
}
func (UnimplementedTcServer) Commit(context.Context, *SagaRequest) (*SagaResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Commit not implemented")
}
func (UnimplementedTcServer) Get(context.Context, *SagaRequest) (*SagaResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Get not implemented")
}
func (UnimplementedTcServer) mustEmbedUnimplementedTcServer() {}

// UnsafeTcServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TcServer will
// result in compilation errors.
type UnsafeTcServer interface {
	mustEmbedUnimplementedTcServer()
}

func RegisterTcServer(s grpc.ServiceRegistrar, srv TcServer) {
	s.RegisterService(&Tc_ServiceDesc, srv)
}

func _Tc_NewGtid_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TcServer).NewGtid(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/saga.Tc/NewGtid",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TcServer).NewGtid(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Tc_Commit_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SagaRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TcServer).Commit(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/saga.Tc/Commit",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TcServer).Commit(ctx, req.(*SagaRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Tc_Get_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SagaRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TcServer).Get(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/saga.Tc/Get",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TcServer).Get(ctx, req.(*SagaRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Tc_ServiceDesc is the grpc.ServiceDesc for Tc service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Tc_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "saga.Tc",
	HandlerType: (*TcServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "NewGtid",
			Handler:    _Tc_NewGtid_Handler,
		},
		{
			MethodName: "Commit",
			Handler:    _Tc_Commit_Handler,
		},
		{
			MethodName: "Get",
			Handler:    _Tc_Get_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "tc.proto",
}

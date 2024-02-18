// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.19.4
// source: verrpc/verrpc.proto

package verrpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// VersionerClient is the client API for Versioner service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type VersionerClient interface {
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Display pldctl and pld version info`
	//
	//GetVersion returns the current version and build information of the running
	//daemon.
	GetVersion(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*Version, error)
}

type versionerClient struct {
	cc grpc.ClientConnInterface
}

func NewVersionerClient(cc grpc.ClientConnInterface) VersionerClient {
	return &versionerClient{cc}
}

func (c *versionerClient) GetVersion(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*Version, error) {
	out := new(Version)
	err := c.cc.Invoke(ctx, "/verrpc.Versioner/GetVersion", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// VersionerServer is the server API for Versioner service.
// All implementations should embed UnimplementedVersionerServer
// for forward compatibility
type VersionerServer interface {
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Display pldctl and pld version info`
	//
	//GetVersion returns the current version and build information of the running
	//daemon.
	GetVersion(context.Context, *VersionRequest) (*Version, error)
}

// UnimplementedVersionerServer should be embedded to have forward compatible implementations.
type UnimplementedVersionerServer struct {
}

func (UnimplementedVersionerServer) GetVersion(context.Context, *VersionRequest) (*Version, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetVersion not implemented")
}

// UnsafeVersionerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to VersionerServer will
// result in compilation errors.
type UnsafeVersionerServer interface {
	mustEmbedUnimplementedVersionerServer()
}

func RegisterVersionerServer(s grpc.ServiceRegistrar, srv VersionerServer) {
	s.RegisterService(&Versioner_ServiceDesc, srv)
}

func _Versioner_GetVersion_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VersionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(VersionerServer).GetVersion(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/verrpc.Versioner/GetVersion",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(VersionerServer).GetVersion(ctx, req.(*VersionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Versioner_ServiceDesc is the grpc.ServiceDesc for Versioner service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Versioner_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "verrpc.Versioner",
	HandlerType: (*VersionerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetVersion",
			Handler:    _Versioner_GetVersion_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "verrpc/verrpc.proto",
}

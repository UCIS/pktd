// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.19.4
// source: metaservice.proto

package lnrpc

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

// MetaServiceClient is the client API for MetaService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type MetaServiceClient interface {
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Returns basic information related to the active daemon`
	//
	//GetInfo returns general information concerning the lightning node including
	//it's identity pubkey, alias, the chains it is connected to, and information
	//concerning the number of open+pending channels.
	GetInfo2(ctx context.Context, in *GetInfo2Request, opts ...grpc.CallOption) (*GetInfo2Response, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Change an encrypted wallet's password at startup`
	//
	//ChangePassword changes the password of the encrypted wallet. This will
	//automatically unlock the wallet database if successful.
	ChangePassword(ctx context.Context, in *ChangePasswordRequest, opts ...grpc.CallOption) (*ChangePasswordResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Check the wallet's password`
	//
	//CheckPassword verify that the password in the request is valid for the wallet.
	CheckPassword(ctx context.Context, in *CheckPasswordRequest, opts ...grpc.CallOption) (*CheckPasswordResponse, error)
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Force pld to crash (for debugging purposes)`
	//
	//Force a pld crash (for debugging purposes)
	ForceCrash(ctx context.Context, in *CrashRequest, opts ...grpc.CallOption) (*CrashResponse, error)
}

type metaServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewMetaServiceClient(cc grpc.ClientConnInterface) MetaServiceClient {
	return &metaServiceClient{cc}
}

func (c *metaServiceClient) GetInfo2(ctx context.Context, in *GetInfo2Request, opts ...grpc.CallOption) (*GetInfo2Response, error) {
	out := new(GetInfo2Response)
	err := c.cc.Invoke(ctx, "/lnrpc.MetaService/GetInfo2", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metaServiceClient) ChangePassword(ctx context.Context, in *ChangePasswordRequest, opts ...grpc.CallOption) (*ChangePasswordResponse, error) {
	out := new(ChangePasswordResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.MetaService/ChangePassword", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metaServiceClient) CheckPassword(ctx context.Context, in *CheckPasswordRequest, opts ...grpc.CallOption) (*CheckPasswordResponse, error) {
	out := new(CheckPasswordResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.MetaService/CheckPassword", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metaServiceClient) ForceCrash(ctx context.Context, in *CrashRequest, opts ...grpc.CallOption) (*CrashResponse, error) {
	out := new(CrashResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.MetaService/ForceCrash", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MetaServiceServer is the server API for MetaService service.
// All implementations should embed UnimplementedMetaServiceServer
// for forward compatibility
type MetaServiceServer interface {
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Returns basic information related to the active daemon`
	//
	//GetInfo returns general information concerning the lightning node including
	//it's identity pubkey, alias, the chains it is connected to, and information
	//concerning the number of open+pending channels.
	GetInfo2(context.Context, *GetInfo2Request) (*GetInfo2Response, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Change an encrypted wallet's password at startup`
	//
	//ChangePassword changes the password of the encrypted wallet. This will
	//automatically unlock the wallet database if successful.
	ChangePassword(context.Context, *ChangePasswordRequest) (*ChangePasswordResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Check the wallet's password`
	//
	//CheckPassword verify that the password in the request is valid for the wallet.
	CheckPassword(context.Context, *CheckPasswordRequest) (*CheckPasswordResponse, error)
	//
	//$pld.category: `Meta`
	//$pld.short_description: `Force pld to crash (for debugging purposes)`
	//
	//Force a pld crash (for debugging purposes)
	ForceCrash(context.Context, *CrashRequest) (*CrashResponse, error)
}

// UnimplementedMetaServiceServer should be embedded to have forward compatible implementations.
type UnimplementedMetaServiceServer struct {
}

func (UnimplementedMetaServiceServer) GetInfo2(context.Context, *GetInfo2Request) (*GetInfo2Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInfo2 not implemented")
}
func (UnimplementedMetaServiceServer) ChangePassword(context.Context, *ChangePasswordRequest) (*ChangePasswordResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ChangePassword not implemented")
}
func (UnimplementedMetaServiceServer) CheckPassword(context.Context, *CheckPasswordRequest) (*CheckPasswordResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckPassword not implemented")
}
func (UnimplementedMetaServiceServer) ForceCrash(context.Context, *CrashRequest) (*CrashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ForceCrash not implemented")
}

// UnsafeMetaServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to MetaServiceServer will
// result in compilation errors.
type UnsafeMetaServiceServer interface {
	mustEmbedUnimplementedMetaServiceServer()
}

func RegisterMetaServiceServer(s grpc.ServiceRegistrar, srv MetaServiceServer) {
	s.RegisterService(&MetaService_ServiceDesc, srv)
}

func _MetaService_GetInfo2_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInfo2Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetaServiceServer).GetInfo2(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.MetaService/GetInfo2",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetaServiceServer).GetInfo2(ctx, req.(*GetInfo2Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetaService_ChangePassword_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ChangePasswordRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetaServiceServer).ChangePassword(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.MetaService/ChangePassword",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetaServiceServer).ChangePassword(ctx, req.(*ChangePasswordRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetaService_CheckPassword_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckPasswordRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetaServiceServer).CheckPassword(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.MetaService/CheckPassword",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetaServiceServer).CheckPassword(ctx, req.(*CheckPasswordRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetaService_ForceCrash_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CrashRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetaServiceServer).ForceCrash(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.MetaService/ForceCrash",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetaServiceServer).ForceCrash(ctx, req.(*CrashRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// MetaService_ServiceDesc is the grpc.ServiceDesc for MetaService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var MetaService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "lnrpc.MetaService",
	HandlerType: (*MetaServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetInfo2",
			Handler:    _MetaService_GetInfo2_Handler,
		},
		{
			MethodName: "ChangePassword",
			Handler:    _MetaService_ChangePassword_Handler,
		},
		{
			MethodName: "CheckPassword",
			Handler:    _MetaService_CheckPassword_Handler,
		},
		{
			MethodName: "ForceCrash",
			Handler:    _MetaService_ForceCrash_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "metaservice.proto",
}

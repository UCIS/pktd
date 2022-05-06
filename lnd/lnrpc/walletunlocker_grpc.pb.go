// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

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

// WalletUnlockerClient is the client API for WalletUnlocker service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type WalletUnlockerClient interface {
	//
	//$pld.category: `Seed`
	//$pld.short_description: `Create a secret seed`
	//
	//GenSeed is the first method that should be used to instantiate a new lnd
	//instance. This method allows a caller to generate a new aezeed cipher seed
	//given an optional passphrase. If provided, the passphrase will be necessary
	//to decrypt the cipherseed to expose the internal wallet seed.
	//
	//Once the cipherseed is obtained and verified by the user, the InitWallet
	//method should be used to commit the newly generated seed, and create the
	//wallet.
	GenSeed(ctx context.Context, in *GenSeedRequest, opts ...grpc.CallOption) (*GenSeedResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Initialize a wallet when starting lnd for the first time`
	//
	//InitWallet is used when lnd is starting up for the first time to fully
	//initialize the daemon and its internal wallet. At the very least a wallet
	//password must be provided. This will be used to encrypt sensitive material
	//on disk.
	//
	//In the case of a recovery scenario, the user can also specify their aezeed
	//mnemonic and passphrase. If set, then the daemon will use this prior state
	//to initialize its internal wallet.
	//
	//Alternatively, this can be used along with the GenSeed RPC to obtain a
	//seed, then present it to the user. Once it has been verified by the user,
	//the seed can be fed into this RPC in order to commit the new wallet.
	InitWallet(ctx context.Context, in *InitWalletRequest, opts ...grpc.CallOption) (*InitWalletResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Unlock an encrypted wallet at startup`
	//
	//UnlockWallet is used at startup of lnd to provide a password to unlock
	//the wallet database.
	UnlockWallet(ctx context.Context, in *UnlockWalletRequest, opts ...grpc.CallOption) (*UnlockWalletResponse, error)
}

type walletUnlockerClient struct {
	cc grpc.ClientConnInterface
}

func NewWalletUnlockerClient(cc grpc.ClientConnInterface) WalletUnlockerClient {
	return &walletUnlockerClient{cc}
}

func (c *walletUnlockerClient) GenSeed(ctx context.Context, in *GenSeedRequest, opts ...grpc.CallOption) (*GenSeedResponse, error) {
	out := new(GenSeedResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.WalletUnlocker/GenSeed", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletUnlockerClient) InitWallet(ctx context.Context, in *InitWalletRequest, opts ...grpc.CallOption) (*InitWalletResponse, error) {
	out := new(InitWalletResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.WalletUnlocker/InitWallet", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *walletUnlockerClient) UnlockWallet(ctx context.Context, in *UnlockWalletRequest, opts ...grpc.CallOption) (*UnlockWalletResponse, error) {
	out := new(UnlockWalletResponse)
	err := c.cc.Invoke(ctx, "/lnrpc.WalletUnlocker/UnlockWallet", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WalletUnlockerServer is the server API for WalletUnlocker service.
// All implementations should embed UnimplementedWalletUnlockerServer
// for forward compatibility
type WalletUnlockerServer interface {
	//
	//$pld.category: `Seed`
	//$pld.short_description: `Create a secret seed`
	//
	//GenSeed is the first method that should be used to instantiate a new lnd
	//instance. This method allows a caller to generate a new aezeed cipher seed
	//given an optional passphrase. If provided, the passphrase will be necessary
	//to decrypt the cipherseed to expose the internal wallet seed.
	//
	//Once the cipherseed is obtained and verified by the user, the InitWallet
	//method should be used to commit the newly generated seed, and create the
	//wallet.
	GenSeed(context.Context, *GenSeedRequest) (*GenSeedResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Initialize a wallet when starting lnd for the first time`
	//
	//InitWallet is used when lnd is starting up for the first time to fully
	//initialize the daemon and its internal wallet. At the very least a wallet
	//password must be provided. This will be used to encrypt sensitive material
	//on disk.
	//
	//In the case of a recovery scenario, the user can also specify their aezeed
	//mnemonic and passphrase. If set, then the daemon will use this prior state
	//to initialize its internal wallet.
	//
	//Alternatively, this can be used along with the GenSeed RPC to obtain a
	//seed, then present it to the user. Once it has been verified by the user,
	//the seed can be fed into this RPC in order to commit the new wallet.
	InitWallet(context.Context, *InitWalletRequest) (*InitWalletResponse, error)
	//
	//$pld.category: `Wallet`
	//$pld.short_description: `Unlock an encrypted wallet at startup`
	//
	//UnlockWallet is used at startup of lnd to provide a password to unlock
	//the wallet database.
	UnlockWallet(context.Context, *UnlockWalletRequest) (*UnlockWalletResponse, error)
}

// UnimplementedWalletUnlockerServer should be embedded to have forward compatible implementations.
type UnimplementedWalletUnlockerServer struct {
}

func (UnimplementedWalletUnlockerServer) GenSeed(context.Context, *GenSeedRequest) (*GenSeedResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GenSeed not implemented")
}
func (UnimplementedWalletUnlockerServer) InitWallet(context.Context, *InitWalletRequest) (*InitWalletResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InitWallet not implemented")
}
func (UnimplementedWalletUnlockerServer) UnlockWallet(context.Context, *UnlockWalletRequest) (*UnlockWalletResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UnlockWallet not implemented")
}

// UnsafeWalletUnlockerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to WalletUnlockerServer will
// result in compilation errors.
type UnsafeWalletUnlockerServer interface {
	mustEmbedUnimplementedWalletUnlockerServer()
}

func RegisterWalletUnlockerServer(s grpc.ServiceRegistrar, srv WalletUnlockerServer) {
	s.RegisterService(&WalletUnlocker_ServiceDesc, srv)
}

func _WalletUnlocker_GenSeed_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GenSeedRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletUnlockerServer).GenSeed(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.WalletUnlocker/GenSeed",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletUnlockerServer).GenSeed(ctx, req.(*GenSeedRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletUnlocker_InitWallet_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitWalletRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletUnlockerServer).InitWallet(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.WalletUnlocker/InitWallet",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletUnlockerServer).InitWallet(ctx, req.(*InitWalletRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _WalletUnlocker_UnlockWallet_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UnlockWalletRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WalletUnlockerServer).UnlockWallet(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/lnrpc.WalletUnlocker/UnlockWallet",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WalletUnlockerServer).UnlockWallet(ctx, req.(*UnlockWalletRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// WalletUnlocker_ServiceDesc is the grpc.ServiceDesc for WalletUnlocker service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var WalletUnlocker_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "lnrpc.WalletUnlocker",
	HandlerType: (*WalletUnlockerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GenSeed",
			Handler:    _WalletUnlocker_GenSeed_Handler,
		},
		{
			MethodName: "InitWallet",
			Handler:    _WalletUnlocker_InitWallet_Handler,
		},
		{
			MethodName: "UnlockWallet",
			Handler:    _WalletUnlocker_UnlockWallet_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "walletunlocker.proto",
}

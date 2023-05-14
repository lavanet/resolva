package main

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lavanet/resolva/nameresolver"
	"google.golang.org/grpc/reflection"
	"log"
	"math/big"
	"net"

	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"
	"google.golang.org/grpc"
)

type grpcServer struct {
	ethClient *ethclient.Client
	nameresolver.UnimplementedNameResolverServer
}

func (s *grpcServer) Resolve(context context.Context, req *nameresolver.ResolveRequest) (*nameresolver.ResolveReplay, error) {
	address, err := ens.Resolve(s.ethClient, req.Domain)
	if err != nil {
		return nil, err
	}
	return &nameresolver.ResolveReplay{Address: address.Bytes()}, nil
}

func (s *grpcServer) ReverseResolve(context context.Context, req *nameresolver.ReverseResolveRequest) (*nameresolver.ReverseResolveReplay, error) {
	address := common.BytesToAddress(req.Address)
	domain, err := ens.ReverseResolve(s.ethClient, address)
	if err != nil {
		return nil, err
	}
	return &nameresolver.ReverseResolveReplay{Domain: domain}, nil
}

func (s *grpcServer) GetBlockNumber(ctx context.Context, empty *nameresolver.Empty) (*nameresolver.BlockNumberReply, error) {
	height, err := s.ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	return &nameresolver.BlockNumberReply{Height: height}, nil
}

func (s *grpcServer) GetBlockByNumber(ctx context.Context, request *nameresolver.BlockByNumberRequest) (*nameresolver.BlockByNumberReplay, error) {
	height := big.NewInt(request.GetHeight())
	block, err := s.ethClient.BlockByNumber(ctx, height)

	if err != nil {
		return nil, err
	}

	return &nameresolver.BlockByNumberReplay{Hash: block.Hash().Hex()}, nil
}

func main() {

	client, err := ethclient.Dial("https://g.w.lavanet.xyz:443/gateway/eth/rpc-http/e7ffcc99b6bba339b0752aa98affe920")
	if err != nil {
		panic(err)
	}

	resolveServer := grpcServer{ethClient: client}

	port := ":8080"
	listen, err := net.Listen("tcp", port)

	server := grpc.NewServer()
	nameresolver.RegisterNameResolverServer(server, &resolveServer)
	reflection.Register(server)

	if err := server.Serve(listen); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

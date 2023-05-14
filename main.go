package main

import (
	"context"
	"fmt"
	"github.com/Zilliqa/gozilliqa-sdk/provider"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lavanet/resolva/nameresolver"
	"github.com/unstoppabledomains/resolution-go/v3"
	"github.com/unstoppabledomains/resolution-go/v3/namingservice"
	"google.golang.org/grpc/reflection"
	"log"
	"math/big"
	"net"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"
	"google.golang.org/grpc"
)

type grpcServer struct {
	ethClient      *ethclient.Client
	polygonClient  *ethclient.Client
	namingServices map[string]resolution.NamingService
	nameresolver.UnimplementedNameResolverServer
}

func NewGRPCServer(ethURL string, polyginURL string, zilliqaURL string) (*grpcServer, error) {
	client, err := ethclient.Dial(ethURL)
	if err != nil {
		panic(err)
	}
	polClient, err := ethclient.Dial(polyginURL)
	unsBuilder := resolution.NewUnsBuilder()
	unsBuilder.SetContractBackend(client)
	unsBuilder.SetL2ContractBackend(polClient)
	zilliqaProvider := provider.NewProvider(zilliqaURL)
	zns, err := resolution.NewZnsBuilder().SetProvider(zilliqaProvider).Build()
	if err != nil {
		return nil, err
	}
	uns, err := unsBuilder.Build()
	if err != nil {
		fmt.Println("ERROR", err)
		return nil, err
	}

	namingServices := map[string]resolution.NamingService{namingservice.UNS: uns, namingservice.ZNS: zns}
	return &grpcServer{polygonClient: polClient, ethClient: client, namingServices: namingServices}, nil
}

func (s *grpcServer) Resolve(context context.Context, req *nameresolver.ResolveRequest) (*nameresolver.ResolveReplay, error) {

	domain := req.GetDomain()

	if strings.HasSuffix(domain, ".eth") {
		address, err := ens.Resolve(s.ethClient, req.Domain)
		if err != nil {
			return nil, err
		}
		return &nameresolver.ResolveReplay{Address: address.Bytes()}, nil
	}

	if strings.HasSuffix(domain, ".crypto") || strings.HasSuffix(domain, ".zil") {
		namingServiceName, err := resolution.DetectNamingService(domain)
		if err != nil {
			return nil, err
		}

		if s.namingServices[namingServiceName] != nil {
			var ticker string
			if namingServiceName == "UNS" {
				ticker = "ETH"
			} else {
				ticker = "BTC"
			}

			resolvedAddress, err := s.namingServices[namingServiceName].Addr(domain, ticker)
			if err != nil {
				return nil, err
			}
			return &nameresolver.ResolveReplay{Address: []byte(resolvedAddress)}, nil
		}
	}

	return nil, fmt.Errorf("unsupported domain suffix: %s", req.Domain)
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

	resolveServer, err := NewGRPCServer("https://g.w.lavanet.xyz:443/gateway/eth/rpc-http/e7ffcc99b6bba339b0752aa98affe920",
		"https://g.w.lavanet.xyz:443/gateway/polygon1/rpc-http/e7ffcc99b6bba339b0752aa98affe920",
		"https://api.zilliqa.com",
	)

	if err != nil {
		panic(err)
	}

	port := ":8080"
	listen, err := net.Listen("tcp", port)

	server := grpc.NewServer()
	nameresolver.RegisterNameResolverServer(server, resolveServer)
	reflection.Register(server)

	if err := server.Serve(listen); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

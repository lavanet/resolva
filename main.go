package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Zilliqa/gozilliqa-sdk/provider"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lavanet/resolva/nameresolver"
	"github.com/unstoppabledomains/resolution-go/v3"
	"github.com/unstoppabledomains/resolution-go/v3/namingservice"
	"google.golang.org/grpc/reflection"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"
	ens "github.com/wealdtech/go-ens/v3"
	"google.golang.org/grpc"
)

type grpcServer struct {
	ethClient      *ethclient.Client
	polygonClient  *ethclient.Client
	stargazeURL    string
	osmoURL        string
	namingServices map[string]resolution.NamingService
	nameresolver.UnimplementedNameResolverServer
}

func NewGRPCServer(ethURL string, polyginURL string, zilliqaURL string, stargazeURL string, osmoURL string) (*grpcServer, error) {
	ethClient, err := ethclient.Dial(ethURL)
	if err != nil {
		panic(err)
	}
	polClient, err := ethclient.Dial(polyginURL)
	unsBuilder := resolution.NewUnsBuilder()
	unsBuilder.SetContractBackend(ethClient)
	unsBuilder.SetL2ContractBackend(polClient)
	zilliqaProvider := provider.NewProvider(zilliqaURL)
	zns, err := resolution.NewZnsBuilder().SetProvider(zilliqaProvider).Build()
	if err != nil {
		return nil, err
	}
	uns, err := unsBuilder.Build()
	if err != nil {
		return nil, err
	}

	namingServices := map[string]resolution.NamingService{
		namingservice.UNS: uns,
		namingservice.ZNS: zns,
	}
	return &grpcServer{
		polygonClient:  polClient,
		ethClient:      ethClient,
		stargazeURL:    stargazeURL,
		osmoURL:        osmoURL,
		namingServices: namingServices,
	}, nil
}

func (s *grpcServer) Resolve(ctx context.Context, req *nameresolver.ResolveRequest) (*nameresolver.ResolveReplay, error) {
	domain := req.GetDomain()
	fmt.Println("Got Domain:", domain)

	if strings.HasSuffix(domain, ".eth") {
		address, err := ens.Resolve(s.ethClient, req.Domain)
		if err != nil {
			return nil, err
		}
		return &nameresolver.ResolveReplay{Address: address.Bytes()}, nil
	}

	if strings.HasSuffix(domain, ".cosmos") {
		address, err := s.queryStargazeNames(domain)

		if err != nil {
			return &nameresolver.ResolveReplay{Address: []byte(*address)}, nil
		}
	}

	address, err := s.queryICNS(domain)

	if err != nil {
		return nil, err
	}

	if address != nil {
		return &nameresolver.ResolveReplay{Address: []byte(*address)}, nil
	}

	address, err = s.queryUnstoppable(domain)

	if err != nil {
		return nil, err
	}

	return &nameresolver.ResolveReplay{Address: []byte(*address)}, nil
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

func (s *grpcServer) queryUnstoppable(domain string) (*string, error) {
	namingServiceName, err := resolution.DetectNamingService(domain)
	if err != nil {
		return nil, fmt.Errorf("unsupported domain suffix: %s", domain)
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
		return &resolvedAddress, nil
	}

	return nil, fmt.Errorf("unsupported domain suffix %s", domain)
}

func (s *grpcServer) queryICNS(domain string) (*string, error) {
	query := fmt.Sprintf("{\"address_by_icns\": {\"icns\": \"%s\"}}", domain)
	resJson, err := s.queryWasmSmartContract(s.osmoURL, "osmo1xk0s8xgktn9x5vwcgtjdxqzadg88fgn33p8u9cnpdxwemvxscvast52cdd", query)

	if err != nil {
		return nil, err
	}

	if _, ok := (*resJson)["code"]; ok {
		return nil, fmt.Errorf("ERROR from osmo: %v", (*resJson)["message"])
	}

	address := (*resJson)["bech32_address"]

	if address == nil {
		addressStr := ""
		return &addressStr, nil
	}

	addressStr := address.(string)
	return &addressStr, nil
}

func (s *grpcServer) queryStargazeNames(domain string) (*string, error) {
	name := strings.TrimSuffix(domain, ".cosmos")
	query := fmt.Sprintf("{\"associated_address\":{\"name\":\"%s\"}}", name)
	resJson, err := s.queryWasmSmartContract(s.stargazeURL, "stars1fx74nkqkw2748av8j7ew7r3xt9cgjqduwn8m0ur5lhe49uhlsasszc5fhr", query)

	if err != nil {
		return nil, err
	}

	if _, ok := (*resJson)["code"]; !ok {
		address := (*resJson)["data"].(string)
		return &address, nil
	}

	return nil, nil
}

func (s *grpcServer) queryWasmSmartContract(chainURL string, contractAddres string, query string) (*map[string]interface{}, error) {
	queryData := []byte(query)
	encQuery := url.QueryEscape(base64.StdEncoding.EncodeToString(queryData))
	fullURL := fmt.Sprintf("%s/cosmwasm/wasm/v1/contract/%s/smart/%s", chainURL, contractAddres, encQuery)
	response, err := http.Get(fullURL)
	if err != nil {
		return nil, err
	}
	resBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var resJson map[string]interface{}
	json.Unmarshal(resBody, &resJson)
	return &resJson, nil
}

func main() {

	resolveServer, err := NewGRPCServer("https://g.w.lavanet.xyz:443/gateway/eth/rpc-http/e7ffcc99b6bba339b0752aa98affe920",
		"https://g.w.lavanet.xyz:443/gateway/polygon1/rpc-http/e7ffcc99b6bba339b0752aa98affe920",
		"https://api.zilliqa.com",
		"https://rest.stargaze-apis.com",
		"https://g.w.lavanet.xyz:443/gateway/cos3/rest/e7ffcc99b6bba339b0752aa98affe920",
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

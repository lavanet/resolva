# Resolva
DNS Resolver over Lava

## How to Run:

```shell
go run main.go
```

## How to test with Lava
1. Run Lava locally
```shell
ignite chain serve -v -r  2>&1 | grep -e lava_ -e ERR_ -e STARPORT] -e !
```

2. Add resolva spec
```shell
GASPRICE="0.000000001ulava"
lavad tx gov submit-proposal spec-add spec.json -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx gov vote 1 yes -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE 
```
3. Stake providers and consumer
```shell
CLIENTSTAKE="500000000000ulava"
PROVIDERSTAKE="500000000000ulava"
lavad tx pairing stake-client "RESOLVA" $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing bulk-stake-provider "RESOLVA" $PROVIDERSTAKE "$PROVIDER1_LISTENER,1" 1 -y --from servicer1 --provider-moniker "dummyMoniker" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
```

4. Run rpc-provider
```shell
PROVIDER1_LISTENER="127.0.0.1:2221"
lavad rpcprovider $PROVIDER1_LISTENER RESOLVA grpc '127.0.0.1:8080' --geolocation 1 --log_level debug --from servicer1 2>&1
```
5. Run rpc-consumer
```shell
lavad rpcconsumer 127.0.0.1:3333 RESOLVA grpc --geolocation 1 --log_level debug --from user1 2>&1
```
6. Test with grpcucrl
```shell
grpcurl --plaintext -proto ./proto/nameresolver.proto -d '{"domain": "sean8.eth"}' 127.0.0.1:3333 nameresolver.NameResolver/Resolve
```

## Development
Compile protobuffs:
```shell
protoc ./proto/*.proto \
--go_out=. \
--go_opt=paths=source_relative \
--go-grpc_out=. \
--go-grpc_opt=paths=source_relative \
--proto_path=./proto/
```

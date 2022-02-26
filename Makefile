.PHONY: check-cdb-env deps ensure-proto-deps lint lint-check-deps migrate-check-deps mocks proto run-cdb-migrations test 

define dsn_missing_error

CDB_DSN envvar is undefined. To run the migrations this envvar
must point to a cockroach db instance. For example, if you are
running a local cockroachdb (with --insecure) and have created
a database called 'etherscan' you can define the envvar by 
running:

export CDB_DSN='postgresql://root@localhost:26257/etherscan?sslmode=disable'

endef
export dsn_missing_error

check-cdb-env:
ifndef CDB_DSN
	$(error ${dsn_missing_error})
endif

deps: 
	@if [ "$(go mod help | echo 'no-mod')" = "no-mod" ] || [ "${GO111MODULE}" = "off" ]; then \
		echo "[dep] fetching package dependencies";\
		go get -u github.com/golang/dep/cmd/dep;\
		dep ensure;\
	fi

ensure-proto-deps:
	@echo "[go get] ensuring protoc packages are available"
	@go get github.com/gogo/protobuf/protoc-gen-gofast
	@go get github.com/gogo/protobuf/proto
	@go get github.com/gogo/protobuf/jsonpb
	@go get github.com/gogo/protobuf/protoc-gen-gogo
	@go get github.com/gogo/protobuf/gogoproto

lint: lint-check-deps
	@echo "[golangci-lint] linting sources"
	@golangci-lint run \
		-E misspell \
		-E golint \
		-E gofmt \
		-E unconvert \
		--exclude-use-default=false \
		./...

lint-check-deps:
	@if [ -z `which golangci-lint` ]; then \
		echo "[go get] installing golangci-lint";\
		GO111MODULE=on go get -u github.com/golangci/golangci-lint/cmd/golangci-lint;\
	fi

migrate-check-deps:
	@if [ -z `which migrate` ]; then \
		echo "[go get] installing golang-migrate cmd with cockroachdb support";\
		if [ "${GO111MODULE}" = "off" ]; then \
			echo "[go get] installing github.com/golang-migrate/migrate/cmd/migrate"; \
			go get -tags 'cockroachdb postgres' -u github.com/golang-migrate/migrate/cmd/migrate;\
		else \
			echo "[go get] installing github.com/golang-migrate/migrate/v4/cmd/migrate"; \
			go get -tags 'cockroachdb postgres' -u github.com/golang-migrate/migrate/v4/cmd/migrate;\
		fi \
	fi

mocks:
	mockgen -package mocks -destination scanner/mocks/mocks.go github.com/moratsam/etherscan/scanner ETHClient,Graph
	mockgen -package mocks -destination txgraphapi/mocks/mock.go github.com/moratsam/etherscan/txgraphapi/proto TxGraphClient,TxGraph_BlocksClient,TxGraph_WalletTxsClient,TxGraph_WalletsClient

proto: ensure-proto-deps
	@echo "[protoc] generating protos for API"
	@protoc --gofast_out=\
	Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,\
	plugins=grpc:. \
	txgraphapi/proto/api.proto


run-cdb-migrations: migrate-check-deps check-cdb-env
	migrate -source file://txgraph/store/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' up


test: 
	@echo "[go test] running tests and collecting coverage metrics"
	@go test -v -tags all_tests -race -coverprofile=coverage.txt -covermode=atomic ./...


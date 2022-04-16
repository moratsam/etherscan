.PHONY: check-cdb-env cdb-connect cdb-start deps docker-run docker-build docker-build-and-push ensure-proto-deps k8s-cdb-connect k8s-delete-monolith k8s-deploy-monolith k8s-pprof-port-forward migrate-check-deps mocks pprof-cpu pprof-mem proto push run-monolith run-cdb-migrations tags test 

define dsn_missing_error

CDB_DSN envvar is undefined. To run the migrations this envvar
must point to a cockroach db instance. For example, if you are
running a local cockroachdb (with --insecure) and have created
a database called 'etherscan' you can define the envvar by 
running:

export CDB_DSN='postgresql://root@localhost:26257/etherscan?sslmode=disable'
endef

export dsn_missing_error

CDB_IMAGE = cdb-schema
MONOLITH_IMAGE = etherscan-monolith
SHA = $(shell git rev-parse --short HEAD)

ifeq ($(origin PRIVATE_REGISTRY),undefined)
PRIVATE_REGISTRY := $(shell minikube ip 2>/dev/null):5000
endif

ifneq ($(PRIVATE_REGISTRY),)
	PREFIX:=${PRIVATE_REGISTRY}/
endif


check-cdb-env:
ifndef CDB_DSN
	$(error ${dsn_missing_error})
endif

cdb-connect:
	@psql postgresql://root@127.0.0.1:26257?sslmode=disable

cdb-start:
	@cockroach start-single-node  --store /usr/local/cockroach/cockroach-data/ --insecure --advertise-addr 127.0.0.1:26257 &
	@sleep 3
	@cockroach sql --insecure -e 'create database etherscan;'


deps: 
	@if [ "$(go mod help | echo 'no-mod')" = "no-mod" ] || [ "${GO111MODULE}" = "off" ]; then \
		echo "[dep] fetching package dependencies";\
		go get -u github.com/golang/dep/cmd/dep;\
		dep ensure;\
	fi

docker-build:
	@echo "[docker build] building ${MONOLITH_IMAGE} (tags: ${PREFIX}${MONOLITH_IMAGE}:latest, ${PREFIX}${MONOLITH_IMAGE}:${SHA})"
	@docker build --file ./depl/monolith/Dockerfile \
		--tag ${PREFIX}${MONOLITH_IMAGE}:latest \
		--tag ${PREFIX}${MONOLITH_IMAGE}:${SHA} \
		. 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker build] building ${CDB_IMAGE} (tags: ${PREFIX}${CDB_IMAGE}:latest, ${PREFIX}${CDB_IMAGE}:${SHA})"
	@docker build --file ./depl/cdb-schema/Dockerfile \
		--tag ${PREFIX}${CDB_IMAGE}:latest \
		--tag ${PREFIX}${CDB_IMAGE}:${SHA} \
		. 2>&1 | sed -e "s/^/ | /g"

docker-build-and-push: docker-build push

docker-run:
	@docker run -it --rm -p 6060:6060 -p 48855:48855 192.168.39.133:5000/etherscan-monolith:latest

ensure-proto-deps:
	@echo "[go get] ensuring protoc packages are available"
	@go get github.com/gogo/protobuf/protoc-gen-gofast
	@go get github.com/gogo/protobuf/proto
	@go get github.com/gogo/protobuf/jsonpb
	@go get github.com/gogo/protobuf/protoc-gen-gogo
	@go get github.com/gogo/protobuf/gogoproto

k8s-cdb-connect:
	@kubectl run -it --rm cockroach-client --image=cockroachdb/cockroach --restart=Never -- sql --insecure --host=cdb-cockroachdb-public.etherscan-data

k8s-delete-monolith:
	@kubectl delete -f depl/monolith/k8s/03-etherscan-monolith.yaml
	@kubectl delete -f depl/monolith/k8s/02-cdb-schema.yaml
	#@helm -n=etherscan-data uninstall cdb
	#@kubectl delete -f depl/monolith/k8s/01-namespaces.yaml

k8s-deploy-monolith:
	#@kubectl apply -f depl/monolith/k8s/01-namespaces.yaml
	#@helm install cdb --namespace=etherscan-data --values depl/monolith/k8s/chart-settings/cdb-settings.yaml stable/cockroachdb
	@kubectl apply -f depl/monolith/k8s/02-cdb-schema.yaml
	@kubectl apply -f depl/monolith/k8s/03-etherscan-monolith.yaml

k8s-pprof-port-forward:
	@kubectl -n etherscan port-forward etherscan-monolith-instance-0 6060

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
	go generate ./...

pprof-cpu:
	 @go tool pprof -http=":55488" http://localhost:6060/debug/pprof/profile

pprof-mem:
	@ go tool pprof -http=":55489" http://localhost:6060/debug/pprof/heap

proto: ensure-proto-deps
	@echo "[protoc] generating protos for txgraph API"
	@protoc --gofast_out=\
	Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,\
	plugins=grpc:. \
	txgraphapi/proto/api.proto
	@echo "[protoc] generating protos for scorestore API"
	@protoc --gofast_out=\
	plugins=grpc:. \
	scorestoreapi/proto/api.proto

push:
	@echo "[docker push] pushing ${PREFIX}${MONOLITH_IMAGE}:latest"
	@docker push ${PREFIX}${MONOLITH_IMAGE}:latest 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${MONOLITH_IMAGE}:${SHA}"
	@docker push ${PREFIX}${MONOLITH_IMAGE}:${SHA} 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${CDB_IMAGE}:latest"
	@docker push ${PREFIX}${CDB_IMAGE}:latest 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${CDB_IMAGE}:${SHA}"
	@docker push ${PREFIX}${CDB_IMAGE}:${SHA} 2>&1 | sed -e "s/^/ | /g"

run-monolith:
	@go run depl/monolith/main.go --tx-graph-uri "postgresql://root@127.0.0.1:26257/etherscan?sslmode=disable" --score-store-uri "postgresql://root@127.0.0.1:26257/etherscan?sslmode=disable" --partition-detection-mode "single" --gravitas-update-interval "1m"


run-cdb-migrations: migrate-check-deps check-cdb-env
	migrate -source file://txgraph/store/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' up
	migrate -source file://scorestore/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' up

tags:
	@ctags -R

test: 
	@echo "[go test] running tests and collecting coverage metrics"
	@go test -v -tags all_tests -race -coverprofile=coverage.txt -covermode=atomic ./...


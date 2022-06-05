.PHONY: cdb-check-env cdb-connect cdb-dashboard cdb-migrate-down cdb-migrate-up cdb-start deps docker-build-cdb docker-build-microservices docker-build-monolith docker-build-and-push-microservices docker-build-and-push-monolith docker-push-cdb docker-push-microservices docker-push-monolith docker-run ensure-proto-deps grafana k8s-cdb-connect k8s-cdb-dashboard k8s-grafana k8s-microservices-delete k8s-monolith-delete k8s-microservices-deploy k8s-monolith-deploy k8s-monolith-pprof-port-forward migrate-check-deps mocks pprof-cpu pprof-mem proto run-monolith tags test

define dsn_missing_error

CDB_DSN envvar is undefined. To run the migrations this envvar
must point to a cockroach db instance. For example, if you are
running a local cockroachdb (with --insecure) and have created
a database called 'etherscan' you can define the envvar by 
running:

export CDB_DSN='postgresql://root@localhost:26257/etherscan?sslmode=disable'
endef

#minikube:
#minikube start --network-plugin=cni --cni=calico
#minikube config set insecure-registry true
# Write to /etc/docker/daemon.json
#{
#  "storage-driver": "overlay2",
#  "insecure-registries": ["192.168.39.204:5000"] #this is $(minikube ip)
#}
# systemctl restart docker
# prometheus link: $(minikube --namespace monitoring service prometheus-service --url)/config
# grafana link: $(minikube --namespace monitoring service grafana-service --url) admin admin


export dsn_missing_error

CDB_IMAGE = cdb-schema
MONOLITH_IMAGE = etherscan-monolith
MICROSERVICES_LIST = etherscan-blockinserter etherscan-frontend etherscan-gravitas etherscan-scanner etherscan-scorestore etherscan-txgraph
SHA = $(shell git rev-parse --short HEAD)

ifeq ($(origin PRIVATE_REGISTRY),undefined)
PRIVATE_REGISTRY := $(shell minikube ip 2>/dev/null):5000
endif

ifneq ($(PRIVATE_REGISTRY),)
	PREFIX:=${PRIVATE_REGISTRY}/
endif


cdb-check-env:
ifndef CDB_DSN
	$(error ${dsn_missing_error})
endif

cdb-connect:
	@psql postgresql://root@127.0.0.1:26257?sslmode=disable

cdb-dashboard:
	@firefox http://localhost:8080

cdb-migrate-down: migrate-check-deps cdb-check-env
	migrate -source file://scorestore/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' down
	migrate -source file://txgraph/store/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' down

cdb-migrate-up: migrate-check-deps cdb-check-env
	migrate -source file://txgraph/store/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' up
	migrate -source file://scorestore/cdb/migrations -database '$(subst postgresql,cockroach,${CDB_DSN})' up

cdb-start:
	@cockroach start-single-node  --store /usr/local/cockroach-data/ --insecure --advertise-addr 127.0.0.1:26257 --cache 1G --background
	@sleep 3
	@cockroach sql --insecure -e 'create database etherscan;'


deps: 
	@if [ "$(go mod help | echo 'no-mod')" = "no-mod" ] || [ "${GO111MODULE}" = "off" ]; then \
		echo "[dep] fetching package dependencies";\
		go get -u github.com/golang/dep/cmd/dep;\
		dep ensure;\
	fi

docker-build-cdb:
	@echo "[docker build] building ${CDB_IMAGE} (tags: ${PREFIX}${CDB_IMAGE}:latest, ${PREFIX}${CDB_IMAGE}:${SHA})"
	@docker build --file ./depl/cdb-schema/Dockerfile \
		--tag ${PREFIX}${CDB_IMAGE}:latest \
		--tag ${PREFIX}${CDB_IMAGE}:${SHA} \
		. 2>&1 | sed -e "s/^/ | /g"

docker-build-microservices: 
	for image in $(MICROSERVICES_LIST); do \
		path=$$(echo $${image} | cut -d '-' -f2); \
		path=$$(echo "./depl/microservices/$${path}/Dockerfile");\
		echo "[docker build] building $${image} (tags: ${PREFIX}$${image}:latest, ${PREFIX}$${image}:${SHA}) found at: $${path}"; \
		docker build --file $${path} \
			--tag ${PREFIX}$${image}:latest \
			--tag ${PREFIX}$${image}:${SHA} \
			. 2>&1 | sed -e "s/^/ | /g" ; \
	done

docker-build-monolith: docker-build-cdb
	@echo "[docker build] building ${MONOLITH_IMAGE} (tags: ${PREFIX}${MONOLITH_IMAGE}:latest, ${PREFIX}${MONOLITH_IMAGE}:${SHA})"
	@docker build --file ./depl/monolith/Dockerfile \
		--tag ${PREFIX}${MONOLITH_IMAGE}:latest \
		--tag ${PREFIX}${MONOLITH_IMAGE}:${SHA} \
		. 2>&1 | sed -e "s/^/ | /g"

docker-build-and-push-microservices: docker-build-microservices docker-push-microservices

docker-build-and-push-monolith: docker-build-monolith docker-push-monolith

docker-push-cdb:
	@echo "[docker push] pushing ${PREFIX}${CDB_IMAGE}:latest"
	@docker push ${PREFIX}${CDB_IMAGE}:latest 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${CDB_IMAGE}:${SHA}"
	@docker push ${PREFIX}${CDB_IMAGE}:${SHA} 2>&1 | sed -e "s/^/ | /g"

docker-push-microservices: docker-push-cdb
	for image in $(MICROSERVICES_LIST); do \
		echo "[docker push] pushing ${PREFIX}$${image}:latest"; \
		docker push ${PREFIX}$${image}:latest 2>&1 | sed -e "s/^/ | /g"; \
		echo "[docker push] pushing ${PREFIX}$${image}:${SHA}"; \
		docker push ${PREFIX}$${image}:${SHA} 2>&1 | sed -e "s/^/ | /g"; \
	done

docker-push-monolith: docker-push-cdb
	@echo "[docker push] pushing ${PREFIX}${MONOLITH_IMAGE}:latest"
	@docker push ${PREFIX}${MONOLITH_IMAGE}:latest 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${MONOLITH_IMAGE}:${SHA}"
	@docker push ${PREFIX}${MONOLITH_IMAGE}:${SHA} 2>&1 | sed -e "s/^/ | /g"

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

k8s-cdb-dashboard:
	@kubectl -n etherscan-data port-forward service/cdb-cockroachdb-public 8081:8080
	@firefox http://localhost:8081

k8s-grafana:
	@firefox $$(minikube --namespace monitoring service grafana-service --url)

k8s-microservices-delete:
	@kubectl delete -f depl/microservices/k8s/02-cdb-schema.yml
	@kubectl delete -f depl/microservices/k8s/03-net-policy.yml
	@kubectl delete -f depl/microservices/k8s/04-etherscan-blockinserter.yml
	@kubectl delete -f depl/microservices/k8s/05-etherscan-frontend.yml
	@kubectl delete -f depl/microservices/k8s/06-etherscan-gravitas.yml
	@kubectl delete -f depl/microservices/k8s/07-etherscan-scanner.yml
	@kubectl delete -f depl/microservices/k8s/08-etherscan-scorestore.yml
	@kubectl delete -f depl/microservices/k8s/09-etherscan-txgraph.yml
	@kubectl delete -f depl/microservices/k8s/10-prometheus.yml
	@kubectl delete -f depl/microservices/k8s/11-grafana-dashboards.yml
	@kubectl delete -f depl/microservices/k8s/12-grafana.yml
	#@kubectl delete -f depl/microservices/k8s/01-namespaces.yml
	#@helm -n=etherscan-data uninstall cdb

k8s-microservices-deploy:
	#@helm install cdb --namespace=etherscan-data --values depl/monolith/k8s/chart-settings/cdb-settings.yml stable/cockroachdb
	@kubectl apply -f depl/microservices/k8s

k8s-monolith-delete:
	@kubectl delete -f depl/monolith/k8s/06-grafana.yml
	@kubectl delete -f depl/monolith/k8s/05-grafana-dashboards.yml
	@kubectl delete -f depl/monolith/k8s/04-prometheus.yml
	@kubectl delete -f depl/monolith/k8s/03-etherscan-monolith.yml
	@kubectl delete -f depl/monolith/k8s/02-cdb-schema.yml
	#@helm -n=etherscan-data uninstall cdb
	#@kubectl delete -f depl/monolith/k8s/01-namespaces.yml

k8s-monolith-deploy:
	#@helm install cdb --namespace=etherscan-data --values depl/monolith/k8s/chart-settings/cdb-settings.yml stable/cockroachdb
	@kubectl apply -f depl/monolith/k8s


k8s-monolith-pprof-port-forward:
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
	@echo "[protoc] generating protos for dbspgraph API"
	@protoc --gofast_out=\
	Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types,\
	plugins=grpc:. \
	dbspgraph/proto/api.proto

run-monolith:
	@go run depl/monolith/main.go --tx-graph-uri "postgresql://root@127.0.0.1:26257/etherscan?sslmode=disable" --score-store-uri "postgresql://root@127.0.0.1:26257/etherscan?sslmode=disable" --partition-detection-mode "single" --gravitas-update-interval "20m" --scanner-num-workers 6 --gravitas-tx-fetchers 10 --gravitas-num-workers 4


tags:
	@ctags -R

test: 
	#@echo "[go test] running tests and collecting coverage metrics"
	#@go test -v -tags all_tests -coverprofile=coverage.txt -covermode=atomic ./...
	@go test ./...


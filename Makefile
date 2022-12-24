.PHONY: all
all: build

.PHONY: build
build:
	@echo "+ $@"
	@cd ./cmd/mqtt2ping && go build -v

.PHONY: run
run: build
	@echo "+ $@"
	@cd ./dist && ./mqtt2ping

.PHONY: fmt
fmt:
	@echo "+ $@"
	@# find . -wholename "*.go" -not -path "./vendor/*"
	gofmt -l -s -w ./cmd/mqtt2ping/main.go
	gofmt -l -s -w ./internal/manager/manager.go
	gofmt -l -s -w ./internal/mqtt_agent/mqtt_agent.go

.PHONY: lint
lint: fmt
	@echo "+ $@"
	@golangci-lint run -v

.PHONY: build-docker
build-docker:
	@echo "+ $@"
	@docker build -t mqtt2ping .

.PHONY: run-docker
run-docker: build-docker
	@echo "+ $@"
	@docker run -e DEBUG=1 -e BROKERURL="tcp://192.168.2.238:1883" -e CONFIG="/data/config.yaml" --rm -v ${PWD}/data:/data:ro --name mqtt2ping mqtt2ping

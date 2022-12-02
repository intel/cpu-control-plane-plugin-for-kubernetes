DOCKER_IMAGE_VERSION=0.1

msg:
	echo "Building resourcemanagement.controlplane"

proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pkg/ctlplaneapi/controlplane.proto

coverage:
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	
image:
	docker build -t ctlplane:${DOCKER_IMAGE_VERSION} -f docker/Dockerfile .

build:
	CGO_ENABLED=0 go build -o bin/ctlplane cmd/ctlplane-agent.go cmd/ctlplane.go

utest:
	go test -count=1 -v ./...

race:
	go test -count=1 -race -v ./...

itest:
	go test -count=1 -tags=integration -v ./pkg/integrationtests

fuzz:
	hack/fuzz_all.sh

clean:
	go clean --cache

golangci:
	golangci-lint run ./pkg/...

all: msg build

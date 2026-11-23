BINARY := docker-machine-driver-nebius

.PHONY: build test lint clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./nebius/... ./cmd/...

lint:
	go vet ./nebius/... ./cmd/...

clean:
	rm -rf bin

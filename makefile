GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod
BINARY_NAME=navmesh-server
CLIENT_BINARY_NAME=navmesh-client
PACKAGE=./
CLIENT_PACKAGE=./cmd/navmesh-client

all: build client

build:
	$(GOBUILD) -o $(BINARY_NAME) $(PACKAGE)

client:
	$(GOBUILD) -o $(CLIENT_BINARY_NAME) $(CLIENT_PACKAGE)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(CLIENT_BINARY_NAME)

run: build
	./$(BINARY_NAME)

tidy:
	$(GOMOD) tidy

.PHONY: all build client clean run tidy

BINARY := dtingest
GO     := go

.PHONY: build install test lint clean

build:
	$(GO) build -o $(BINARY) .

install:
	$(GO) install .

test:
	$(GO) test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

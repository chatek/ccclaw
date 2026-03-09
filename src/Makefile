BIN := dist/bin/ccclaw

.PHONY: build test tidy clean

build:
	@mkdir -p dist/bin
	go build -o $(BIN) ./cmd/ccclaw

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf dist/bin/ccclaw

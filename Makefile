.PHONY: generate clean build build-debug

generate:
	protoc --go_out=. --go_opt=paths=source_relative \
			--go-grpc_out=. --go-grpc_opt=paths=source_relative \
			protos/*.proto

clean:
	rm -rf dist
	mkdir -p dist

build: clean
	go build -ldflags="-w -s" -o dist/keyval .

build-debug: clean
	go build -o dist/keyval .

.PHONY: generate clean build build-debug

clean:
	rm -rf dist
	mkdir -p dist

build: clean
	go build -ldflags="-w -s" -o dist/keyval .

build-debug: clean
	go build -o dist/keyval .

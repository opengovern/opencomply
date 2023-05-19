.PHONY: build build-all docker clean compliance

build-all:
	export GOOS=linux
	export GOARCH=amd64
	ls cmd | xargs -I{} bash -c 'GOOS=linux GOARCH=amd64 go build -tags musl -v -ldflags "-w -extldflags -static" -o ./build/ ./cmd/{}'

build:
	export GOOS=linux
	export GOARCH=amd64
	./scripts/list_services > ./services
	cat ./services
	cat ./services | xargs -P 4 -I{} bash -c 'GOOS=linux GOARCH=amd64 go build -tags musl -v -ldflags "-w -extldflags -static" -o ./build/ ./cmd/{}'

clean:
	rm -r ./build

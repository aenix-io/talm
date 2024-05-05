VERSION=$(shell git describe --tags)

generate:
	go generate

build:
	go build -ldflags="-X 'main.Version=$(VERSION)'"

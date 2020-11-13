entgen: *.go
	gofmt -w -s -l .
	go build

test: entgen
	go test

.PHONY: test

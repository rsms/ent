PATH := $(PWD)/../../entgen:$(PATH)

redis: ents.gen.go
	go build

ents.gen.go: main.go
	@PATH=$(PATH) entgen -v

dev:
	autorun ../../*.go ../../redis/*.go *.go -- \
		"go fmt . ../.. ../../redis && ../../entgen/entgen && go build && ./redis"

.PHONY: dev

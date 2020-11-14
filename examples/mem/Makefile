PATH := $(PWD)/../../entgen:$(PATH)

mem: ents.gen.go
	go build

ents.gen.go: main.go
	@PATH=$(PATH) entgen -v .

dev:
	autorun ../../*.go ../../mem/*.go *.go -- \
		"go fmt . ../.. ../../mem && ../../entgen/entgen && go build && ./mem"

.PHONY: dev

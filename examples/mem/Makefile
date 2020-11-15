PATH := $(PWD)/../../entgen:$(PATH)

mem: ents.gen.go
	go build

ents.gen.go: main.go
	@PATH=$(PATH) entgen -v .

dev:
	autorun main.go ../../*.go ../../entgen/*.go ../../mem/*.go -- \
		"go fmt . ../.. ../../entgen ../../mem && \
		 make -C ../../entgen && ../../entgen/entgen -v && go build && ./mem"

.PHONY: dev

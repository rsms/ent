PATH := $(PWD)/../../entgen:$(PATH)

tutorial: ents.gen.go
	go build

ents.gen.go: main.go
	@PATH=$(PATH) entgen -v .

dev:
	autorun main.go ../../*.go ../../entgen/*.go ../../mem/*.go -- \
		"go fmt . ../.. ../../entgen ../../mem && \
		 make -C ../../entgen && ../../entgen/entgen -v && go build && ./tutorial"

.PHONY: dev

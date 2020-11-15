PATH := $(PWD)/../../entgen:$(PATH)

redis: ents.gen.go
	go build

ents.gen.go: main.go
	@PATH=$(PATH) entgen -v

dev:
	autorun main.go ../../*.go ../../entgen/*.go ../../redis/*.go -- \
		"go fmt . ../.. ../../entgen ../../redis && \
		 make -C ../../entgen && make ents.gen.go && go build && ./redis"

.PHONY: dev

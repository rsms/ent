test:
	go test
	cd entgen && go test

entgen:
	@$(MAKE) -C entgen

fmt:
	gofmt -w -s -l .

doc:
	@echo "open http://localhost:6060/pkg/github.com/rsms/ent/"
	@bash -c '[ "$$(uname)" == "Darwin" ] && \
	         (sleep 1 && open "http://localhost:6060/pkg/github.com/rsms/ent/") &'
	godoc -http=localhost:6060

example:
	cd example

.PHONY: test entgen fmt doc example

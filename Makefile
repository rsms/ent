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

dist:
	@echo "Release checklist:"
	@echo "  VERSION=$$(git tag -l 'v*' | tail -n1 | sed -E 's/^v//')  # <- bump this version"
	@echo "  sed -i '' -E 's/= \".+\"/= \"'\$$VERSION'\"/' entgen/version.go"
	@echo "  git commit -m 'version bump' entgen/version.go"
	@echo "  git tag v\$$VERSION"
	@echo "  git push origin master v\$$VERSION"

.PHONY: test entgen fmt doc

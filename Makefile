all: minigolf test format

minigolf: _FORCE_
	go build -o minigolf main.go

test: _FORCE_
	go test -count=1  ./...

format: _FORCE_
	test -f /usr/local/bin/ci-l && ci-l *.go */*.go */*.golf */*.want *.sh doc/*.md
	gofmt -w *.go */*.go */*.golf
	test -f /usr/local/bin/ci-l && ci-l *.go */*.go */*.golf */*.want *.sh doc/*.md

clean: _FORCE_
	rm -f minigolf

_FORCE_:

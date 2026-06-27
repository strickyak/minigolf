all: minigolf test format

minigolf: _FORCE_
	go build -o minigolf main.go

test: _FORCE_
	go test -count=1  ./... 2>&1 | tee _test_out

format: _FORCE_
	test -f /usr/local/bin/ci-l && ci-l [a-z]*.go [a-z]*/*.go [a-z]*/*.golf [a-z]*/*.want [a-z]*.sh doc/*.md
	gofmt -w [a-z]*.go [a-z]*/*.go [a-z]*/*.golf
	test -f /usr/local/bin/ci-l && ci-l [a-z]*.go [a-z]*/*.go [a-z]*/*.golf [a-z]*/*.want [a-z]*.sh doc/*.md

clean: _FORCE_
	rm -f minigolf

_FORCE_:

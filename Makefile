hash:= $(shell git rev-parse --short HEAD)
buildTime:= $(shell date -u '+%Y-%m-%d %H:%M:%S')


install: cli

cli:
	go build -ldflags\
	 "-X 'github.com/D8-X/d8x-cli/internal/version.buildTime=$(buildTime)' -X 'github.com/D8-X/d8x-cli/internal/version.commit=$(hash)' "\
	 -o d8x main.go

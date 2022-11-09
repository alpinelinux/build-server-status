exec >&2

CGO_ENABLED=0 go build -o "$3" -ldflags="-s -w" ./cmd/

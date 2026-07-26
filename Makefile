.PHONY: build run test vet fmt frontend-dev frontend-build clean

build:
	go build -o bin/golo ./cmd/golo

run: build
	./bin/golo

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

frontend-dev:
	cd apps/web && npm run dev

frontend-build:
	cd apps/web && npm run build

clean:
	rm -f bin/golo

.PHONY: run build test vet clean

run:
	go run .

build:
	go build -o trading .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f trading

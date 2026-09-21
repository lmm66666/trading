.PHONY: run-workbench run-updater build image-updater image-workbench test vet clean

WORKBENCH_CONFIG ?= config.yaml
UPDATER_CONFIG ?= config.updater.yaml

run-workbench:
	go run . -service workbench -config $(WORKBENCH_CONFIG)

run-updater:
	go run . -service updater -config $(UPDATER_CONFIG)

build:
	go build -o trading .

image-updater:
	docker buildx build --platform linux/amd64 --target updater -t trading-updater:latest --load .

image-workbench:
	docker buildx build --platform linux/amd64 --target workbench -t trading-workbench:latest --load .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f trading

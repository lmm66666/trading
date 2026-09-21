.PHONY: run-workbench run-updater build image-updater image-updater-configured image-workbench dev test vet clean

WORKBENCH_CONFIG ?= config.yaml
UPDATER_CONFIG ?= config.updater.yaml

run-workbench:
	go run . -service workbench -config $(WORKBENCH_CONFIG)

run-updater:
	go run . -service updater -config $(UPDATER_CONFIG)

# Start workbench backend and Vite dev server together; Ctrl+C stops both.
# Open http://localhost:5173 (proxies /api to 127.0.0.1:8080).
dev:
	trap 'kill 0' EXIT; \
	go run . -service workbench -config $(WORKBENCH_CONFIG) & \
	npm --prefix web run dev & \
	wait

build:
	go build -o trading .

image-updater:
	docker buildx build --platform linux/amd64 --target updater -t trading-updater:latest --load .
	docker save -o trading-updater.tar trading-updater:latest

image-updater-configured:
	docker buildx build --platform linux/amd64 --target updater-configured --no-cache-filter updater-configured --secret "id=updater_config,src=$(UPDATER_CONFIG)" -t trading-updater:configured --load .
	docker tag trading-updater:configured trading-updater:latest
	docker save -o trading-updater.tar trading-updater:latest

image-workbench:
	docker buildx build --platform linux/amd64 --target workbench -t trading-workbench:latest --load .
	docker save -o trading-workbench.tar trading-workbench:latest

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f trading

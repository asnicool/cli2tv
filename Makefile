VERSION=$(shell cat version.txt)
LDFLAGS="-s -w"

BUILD_DIR=build
BIN=$(BUILD_DIR)/cli2tv

.PHONY: webui build install uninstall clean test

webui:
	npm run build:webui

build:
	mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags $(LDFLAGS) -o $(BIN) ./cmd/cli2tv

install: build
	mkdir -vp /usr/local/bin/
	cp $(BIN) /usr/local/bin/

uninstall:
	rm -vf /usr/local/bin/cli2tv

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

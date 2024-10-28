.PHONY: all build-mac clean run help build-linux build-win build-metric-server-linux-amd64 build-metric-agent-linux-amd64

WEBRTC_SIGNAL_SERVER_BIN=release/webrtc-signal-server
WEBRTC_SIGNAL_SERVER_SOURCE=signal-server/main.go

WEBRTC_WIN_CLIENT_BIN=release/webrtc-win-client
WEBRTC_WIN_CLIENT_SOURCE=windows-client/main.go



all: build clean

build-mac:
	go build -o -ldflags="-w" $(BIN_FILE) $(SOURCE_FILE)

clean:
	go clean -i -n

run:
	./$(BIN_FILE) $(GROUP_ID) $(MANIFEST_XML_PATH)

build-webrtc-cleint-win:
	CGO_ENABLED=1 \
    CC=x86_64-w64-mingw32-gcc \
    CXX=x86_64-w64-mingw32-gcc  \
  	GOOS=windows GOARCH=amd64 go build -x -v -ldflags="-s -w"  \
  	-o $(WEBRTC_WIN_CLIENT_BIN).exe $(WEBRTC_WIN_CLIENT_SOURCE)

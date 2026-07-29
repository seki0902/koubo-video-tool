@echo off
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o koubo-video-tool.exe .
echo Build complete: koubo-video-tool.exe
dir koubo-video-tool.exe

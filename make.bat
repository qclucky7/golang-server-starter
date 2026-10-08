@echo off
REM golang-server-starter Windows 命令包装，用法：make.bat <target> [env]
REM 可用 target 与 Makefile 保持一致：run / build / docs / fmt / vet / test / tidy / check / clean
REM run 支持第二个参数指定环境，例如：make.bat run test

setlocal
set APP_NAME=golang-server-starter
set MAIN_PKG=./cmd/server
set ENV=%~2
if "%ENV%"=="" set ENV=dev

if "%~1"=="" goto help
if /I "%~1"=="help"   goto help
if /I "%~1"=="run"    goto run
if /I "%~1"=="build"  goto build
if /I "%~1"=="build-linux"   goto build_linux
if /I "%~1"=="build-windows" goto build_windows
if /I "%~1"=="docs"   goto docs
if /I "%~1"=="fmt"    goto fmt
if /I "%~1"=="vet"    goto vet
if /I "%~1"=="test"   goto test
if /I "%~1"=="tidy"   goto tidy
if /I "%~1"=="check"  goto check
if /I "%~1"=="clean"  goto clean

echo [ERROR] 未知命令: %~1
goto help

:help
echo golang-server-starter 可用命令:
echo   make.bat run [env]  本地启动服务（env 默认 dev，如 make.bat run test）
echo   make.bat build      编译到 bin/
echo   make.bat build-linux    交叉编译 Linux amd64 到 bin/
echo   make.bat build-windows  交叉编译 Windows amd64 到 bin/
echo   make.bat docs       生成 Swagger 文档到 docs/
echo   make.bat fmt        格式化代码
echo   make.bat vet        静态检查
echo   make.bat test       运行单元测试
echo   make.bat tidy       整理依赖
echo   make.bat check      格式化 + 静态检查 + 测试
echo   make.bat clean      清理构建产物
exit /b 0

:run
go run %MAIN_PKG% -e %ENV%
exit /b %ERRORLEVEL%

:build
if not exist bin mkdir bin
go build -trimpath -ldflags "-s -w" -o bin/%APP_NAME%.exe %MAIN_PKG%
exit /b %ERRORLEVEL%

:build_linux
if not exist bin mkdir bin
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o bin/%APP_NAME%-linux-amd64 %MAIN_PKG%
exit /b %ERRORLEVEL%

:build_windows
if not exist bin mkdir bin
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o bin/%APP_NAME%.exe %MAIN_PKG%
exit /b %ERRORLEVEL%

:docs
swag init -g cmd/server/main.go -o docs --parseInternal
exit /b %ERRORLEVEL%

:fmt
go fmt ./...
exit /b %ERRORLEVEL%

:vet
go vet ./...
exit /b %ERRORLEVEL%

:test
go test ./... -count=1
exit /b %ERRORLEVEL%

:tidy
go mod tidy
exit /b %ERRORLEVEL%

:check
go fmt ./... && go vet ./... && go test ./... -count=1
exit /b %ERRORLEVEL%

:clean
if exist bin rmdir /s /q bin
if exist coverage.out del /q coverage.out
exit /b 0

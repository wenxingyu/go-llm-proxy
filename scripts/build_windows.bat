@echo off
echo ========================================
echo Go LLM Proxy - Windows Build Script
echo ========================================

:: 设置变量
set APP_NAME=go-llm-proxy
set VERSION=%1
if "%VERSION%"=="" set VERSION=1.0.0
set BUILD_TIME=%date% %time%
set GO_VERSION=%GOVERSION%

:: 显示构建信息
echo Building %APP_NAME% v%VERSION%
echo Build Time: %BUILD_TIME%
echo Go Version: %GO_VERSION%
echo.

:: 检查Go环境
echo Checking Go environment...
go version
if %errorlevel% neq 0 (
    echo ERROR: Go is not installed or not in PATH
    exit /b 1
)

:: 清理旧的构建文件
echo Cleaning old build files...
if exist %APP_NAME%.exe del %APP_NAME%.exe
if exist dist rmdir /s /q dist

:: 创建dist目录
echo Creating dist directory...
mkdir dist

:: 构建Windows可执行文件
echo Building Windows executable...
go build -o dist\%APP_NAME%.exe cmd/server/main.go
if %errorlevel% neq 0 (
    echo ERROR: Build failed
    exit /b 1
)

:: 复制配置文件
echo Copying configuration files...
if exist configs copy configs dist\configs\
if exist README.md copy README.md dist\
if exist LICENSE copy LICENSE dist\

:: 创建logs目录
echo Creating logs directory...
mkdir dist\logs

:: 显示构建结果
echo.
echo ========================================
echo Build completed successfully!
echo ========================================
echo Executable: dist\%APP_NAME%.exe
echo Size: 
dir dist\%APP_NAME%.exe | find "bytes"
echo.
echo To run the application:
echo   cd dist
echo   %APP_NAME%.exe
echo.
echo To run with custom config:
echo   %APP_NAME%.exe -f configs\custom-config.yml
echo ======================================== 
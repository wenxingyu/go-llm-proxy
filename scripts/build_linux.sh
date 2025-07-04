#!/bin/bash

# ========================================
# Go LLM Proxy - Linux Build Script
# ========================================

# 设置变量
APP_NAME="go-llm-proxy"
VERSION=${1:-"1.0.0"}
BUILD_TIME=$(date '+%Y%m%d%H%M%S')
GO_VERSION=$(go version | awk '{print $3}')

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 显示构建信息
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Go LLM Proxy - Linux Build Script${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Building ${GREEN}${APP_NAME}${NC} v${YELLOW}${VERSION}${NC}"
echo -e "Build Time: ${YELLOW}${BUILD_TIME}${NC}"
echo -e "Go Version: ${YELLOW}${GO_VERSION}${NC}"
echo

# 检查Go环境
echo -e "${BLUE}Checking Go environment...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}ERROR: Go is not installed or not in PATH${NC}"
    exit 1
fi

go version
echo

# 清理旧的构建文件
echo -e "${BLUE}Cleaning old build files...${NC}"
rm -f ${APP_NAME}
rm -rf dist

# 创建dist目录
echo -e "${BLUE}Creating dist directory...${NC}"
mkdir -p dist

# 设置构建标志
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime='${BUILD_TIME}' -s -w"

# 构建Linux可执行文件
echo -e "${BLUE}Building Linux executable...${NC}"
GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o dist/${APP_NAME} cmd/server/main.go
if [ $? -ne 0 ]; then
    echo -e "${RED}ERROR: Build failed${NC}"
    exit 1
fi

# 复制配置文件
echo -e "${BLUE}Copying configuration files...${NC}"
if [ -d "configs" ]; then
    cp -r configs dist/
fi
if [ -f "README.md" ]; then
    cp README.md dist/
fi
if [ -f "LICENSE" ]; then
    cp LICENSE dist/
fi

# 创建logs目录
echo -e "${BLUE}Creating logs directory...${NC}"
mkdir -p dist/logs

# 设置可执行权限
chmod +x dist/${APP_NAME}

# 显示构建结果
echo
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Build completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "Executable: ${YELLOW}dist/${APP_NAME}${NC}"
echo -e "Size: ${YELLOW}$(du -h dist/${APP_NAME} | cut -f1)${NC}"
echo
echo -e "${BLUE}To run the application:${NC}"
echo -e "  cd dist"
echo -e "  ./${APP_NAME}"
echo
echo -e "${BLUE}To run with custom config:${NC}"
echo -e "  ./${APP_NAME} -f configs/custom-config.yml"
echo -e "${GREEN}========================================${NC}"

# 可选：创建tar.gz包
if [ "$2" = "package" ]; then
    echo
    echo -e "${BLUE}Creating distribution package...${NC}"
    cd dist
    tar -czf ../${APP_NAME}-${VERSION}-linux-amd64.tar.gz *
    cd ..
    echo -e "${GREEN}Package created: ${YELLOW}${APP_NAME}-${VERSION}-linux-amd64.tar.gz${NC}"
fi 
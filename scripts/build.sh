#!/bin/bash

# ========================================
# Go LLM Proxy - Cross Platform Build Script
# ========================================

# 设置变量
APP_NAME="go-llm-proxy"
VERSION=${1:-"1.0.0"}
BUILD_TIME=$(date '+%Y-%m-%d %H:%M:%S')
GO_VERSION=$(go version | awk '{print $3}')

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 显示构建信息
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Go LLM Proxy - Cross Platform Build${NC}"
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
rm -f ${APP_NAME}.exe
rm -rf dist

# 创建dist目录
echo -e "${BLUE}Creating dist directory...${NC}"
mkdir -p dist

# 设置构建标志
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime='${BUILD_TIME}' -s -w"

# 构建函数
build_for_platform() {
    local GOOS=$1
    local GOARCH=$2
    local SUFFIX=$3
    
    echo -e "${BLUE}Building for ${YELLOW}${GOOS}/${GOARCH}${NC}..."
    
    # 设置环境变量
    export GOOS=${GOOS}
    export GOARCH=${GOARCH}
    
    # 构建可执行文件
    go build -ldflags "${LDFLAGS}" -o dist/${APP_NAME}${SUFFIX} cmd/server/main.go
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Built successfully: ${YELLOW}dist/${APP_NAME}${SUFFIX}${NC}"
        
        # 设置可执行权限（Linux/macOS）
        if [ "$GOOS" != "windows" ]; then
            chmod +x dist/${APP_NAME}${SUFFIX}
        fi
        
        # 显示文件大小
        if command -v du &> /dev/null; then
            echo -e "  Size: ${YELLOW}$(du -h dist/${APP_NAME}${SUFFIX} | cut -f1)${NC}"
        fi
    else
        echo -e "${RED}✗ Build failed for ${GOOS}/${GOARCH}${NC}"
        return 1
    fi
    
    echo
}

# 构建不同平台
echo -e "${BLUE}Building for multiple platforms...${NC}"

# 当前平台
CURRENT_OS=$(go env GOOS)
CURRENT_ARCH=$(go env GOARCH)

echo -e "${YELLOW}Current platform: ${CURRENT_OS}/${CURRENT_ARCH}${NC}"
echo

# 构建当前平台
if [ "$CURRENT_OS" = "windows" ]; then
    build_for_platform "windows" "amd64" ".exe"
else
    build_for_platform "$CURRENT_OS" "amd64" ""
fi

# 询问是否构建其他平台
echo -e "${BLUE}Do you want to build for other platforms? (y/n)${NC}"
read -r response

if [[ "$response" =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}Building for all platforms...${NC}"
    
    # Windows
    build_for_platform "windows" "amd64" ".exe"
    build_for_platform "windows" "386" ".exe"
    
    # Linux
    build_for_platform "linux" "amd64" ""
    build_for_platform "linux" "386" ""
    build_for_platform "linux" "arm64" ""
    
    # macOS
    build_for_platform "darwin" "amd64" ""
    build_for_platform "darwin" "arm64" ""
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

# 显示构建结果
echo
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Build completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "Build directory: ${YELLOW}dist/${NC}"
echo
echo -e "${BLUE}Available executables:${NC}"
ls -la dist/${APP_NAME}* 2>/dev/null | grep -v "\.md$\|\.yml$\|LICENSE$\|configs$\|logs$" || echo "No executables found"

echo
echo -e "${BLUE}To run the application:${NC}"
if [ "$CURRENT_OS" = "windows" ]; then
    echo -e "  cd dist"
    echo -e "  ${APP_NAME}.exe"
    echo -e "  ${APP_NAME}.exe -f configs/custom-config.yml"
else
    echo -e "  cd dist"
    echo -e "  ./${APP_NAME}"
    echo -e "  ./${APP_NAME} -f configs/custom-config.yml"
fi

echo -e "${GREEN}========================================${NC}" 
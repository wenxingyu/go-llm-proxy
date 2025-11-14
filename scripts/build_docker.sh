#!/bin/bash

# ========================================
# Go LLM Proxy - Docker Build Script
# ========================================

# 设置变量
APP_NAME="go-llm-proxy"
VERSION=${1:-"1.0.0"}
IMAGE_NAME=${2:-"${APP_NAME}"}
IMAGE_TAG=${3:-"${VERSION}"}
FULL_IMAGE_NAME="${IMAGE_NAME}:${IMAGE_TAG}"
DOCKERFILE_PATH="./Dockerfile"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 显示构建信息
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Go LLM Proxy - Docker Build Script${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Building Docker image: ${GREEN}${FULL_IMAGE_NAME}${NC}"
echo

# 检查Docker环境
echo -e "${BLUE}Checking Docker environment...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}ERROR: Docker is not installed or not in PATH${NC}"
    exit 1
fi

docker --version
echo

# 检查Dockerfile是否存在
if [ ! -f "${DOCKERFILE_PATH}" ]; then
    echo -e "${RED}ERROR: Dockerfile not found in current directory${NC}"
    echo -e "${RED}Please make sure you are running this script from the dist directory${NC}"
    exit 1
fi

# 检查可执行文件是否存在
if [ ! -f "./${APP_NAME}" ]; then
    echo -e "${RED}ERROR: ${APP_NAME} executable not found in current directory${NC}"
    echo -e "${RED}Please make sure you are running this script from the dist directory${NC}"
    exit 1
fi

# 构建Docker镜像
echo -e "${BLUE}Building Docker image...${NC}"
echo -e "Image: ${YELLOW}${FULL_IMAGE_NAME}${NC}"
echo -e "Dockerfile: ${YELLOW}${DOCKERFILE_PATH}${NC}"
echo -e "Build context: ${YELLOW}$(pwd)${NC}"
echo

docker build -f Dockerfile -t ${FULL_IMAGE_NAME} .
BUILD_STATUS=$?

if [ ${BUILD_STATUS} -ne 0 ]; then
    echo -e "${RED}ERROR: Docker build failed${NC}"
    exit 1
fi

# 显示构建结果
echo
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Docker image built successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "Image: ${YELLOW}${FULL_IMAGE_NAME}${NC}"
echo -e "Size: ${YELLOW}$(docker images ${FULL_IMAGE_NAME} --format '{{.Size}}')${NC}"
echo

# 显示Docker命令
echo -e "${BLUE}To run the container:${NC}"
echo -e "  docker run -d --name ${APP_NAME} -p 8080:8080 ${FULL_IMAGE_NAME}"
echo
echo -e "${BLUE}To run with custom config:${NC}"
echo -e "  docker run -d --name ${APP_NAME} -p 8080:8080 -v \$(pwd)/configs:/app/go-llm-proxy/config ${FULL_IMAGE_NAME}"
echo
echo -e "${BLUE}To view logs:${NC}"
echo -e "  docker logs -f ${APP_NAME}"
echo
echo -e "${BLUE}To stop the container:${NC}"
echo -e "  docker stop ${APP_NAME}"
echo
echo -e "${BLUE}To remove the container:${NC}"
echo -e "  docker rm ${APP_NAME}"
echo
echo -e "${GREEN}========================================${NC}"

# 可选：推送镜像到仓库
if [ "$4" = "push" ]; then
    echo
    echo -e "${BLUE}Pushing Docker image to registry...${NC}"
    docker push ${FULL_IMAGE_NAME}
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}Image pushed successfully${NC}"
    else
        echo -e "${RED}ERROR: Failed to push image${NC}"
        exit 1
    fi
fi


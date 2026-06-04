#!/bin/bash

# 获取当前脚本所在的绝对路径
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# 确定夜莺主项目的根目录
N9E_ROOT_DIR="$( cd "${SCRIPT_DIR}/.." && pwd )"

echo -e "\033[36m>>> Building Nightingale Docker Image...\033[0m"
echo "Context: ${N9E_ROOT_DIR}"
echo "Dockerfile: ${SCRIPT_DIR}/Dockerfile"

# 构建镜像，指定项目根目录为上下文
docker build -t nightingale:latest -f "${SCRIPT_DIR}/Dockerfile" "${N9E_ROOT_DIR}"

if [ $? -ne 0 ]; then
    echo -e "\033[31mDocker Build Failed!\033[0m"
    exit 1
fi

echo -e "\n\033[36m>>> Starting Nightingale Container...\033[0m"

LOCAL_CONFIG="${N9E_ROOT_DIR}/etc/config.toml"
if [ ! -f "${LOCAL_CONFIG}" ]; then
    echo -e "\033[33mWarning: Local config.toml not found at ${LOCAL_CONFIG}. Container will run with packaged config.\033[0m"
    docker run -d --name nightingale -p 17000:17000 nightingale:latest
else
    echo "Mapping config: ${LOCAL_CONFIG} -> /app/etc/config.toml"
    docker run -d --name nightingale -p 17000:17000 -v "${LOCAL_CONFIG}:/app/etc/config.toml" nightingale:latest
fi

if [ $? -eq 0 ]; then
    echo -e "\n\033[32m>>> Nightingale started successfully!\033[0m"
    echo -e "You can access it at http://localhost:17000"
    echo "To check logs, run: docker logs -f nightingale"
else
    echo -e "\033[31mFailed to start Docker container!\033[0m"
    exit 2
fi

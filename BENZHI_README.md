# BENZHI_README

基于 Go 实现的seed-vigor-workbench Web 项目，一款后端服务，用于支持seed-vigor-workbench的核心业务流程。

## 项目说明
- 项目：benzhi-project-97f8663a-4593-4e67-894d-10c74dec4dca
- 项目用途：用于支持seed-vigor-workbench的核心业务流程。
- Go 工具链：`golang:1.23.0`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/seed-vigor-web -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-97f8663a-4593-4e67-894d-10c74dec4dca-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-97f8663a-4593-4e67-894d-10c74dec4dca-arm64 linux/arm64
docker run -it benzhi-project-97f8663a-4593-4e67-894d-10c74dec4dca-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/seed-vigor-web -selfcheck -addr=127.0.0.1:19081`

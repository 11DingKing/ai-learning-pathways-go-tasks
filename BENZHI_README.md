# BENZHI_README

## 项目说明

- 项目：11DingKing/ai-learning-pathways-go-tasks
- 项目用途：AI Learning Pathways is a Go backend for governing artificial-intelligence education across primary, secondary, university, vocational, and lifelong-learning programs.
- Go 工具链：`golang:1.26.0`
- 前端工具链：无

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/server

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-53-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-53-arm64 linux/arm64
docker run -it benzhi-task-53-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-53-arm64:latest
```

## 题目验证命令

1. 预期退出码 0：`go test ./internal/httpapi -run '^TestUnknownBearerTokenKeepsAuthenticationStatus$' -count=1`
2. 预期退出码 0：`go test ./...`
3. 预期退出码 0：`GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./...`

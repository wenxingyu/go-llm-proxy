FROM docker.1ms.run/redis:8.2.3-alpine

WORKDIR /app/go-llm-proxy

# 默认的数据库连接配置，可通过环境变量覆盖
ENV DB_HOST=localhost \
    DB_PORT=5432 \
    DB_USER=appuser \
    DB_PASSWORD=apppassword \
    DB_NAME=appdb

# 默认的 Redis 连接配置，可通过环境变量覆盖
ENV REDIS_HOST=localhost \
    REDIS_PORT=6379 \
    REDIS_PASSWORD=apppassword \
    REDIS_DB=0

# 预创建可挂载的配置与日志目录
RUN mkdir -p /app/go-llm-proxy/config /app/go-llm-proxy/log

# 声明可挂载的卷，方便外部挂载配置与日志
VOLUME ["/app/go-llm-proxy/config", "/app/go-llm-proxy/log"]

# 将项目文件拷贝到容器中（如有需要可按实际情况调整）
COPY . /app/go-llm-proxy

RUN chmod +x go-llm-proxy

# 默认启动命令（可根据实际应用调整）
CMD ["./go-llm-proxy"]

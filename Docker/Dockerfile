# 使用 Ubuntu 20.04 作为基础镜像
FROM ubuntu:20.04

# 设置环境变量以避免在安装过程中出现交互提示
ENV DEBIAN_FRONTEND=noninteractive

# 更新包列表并安装 Coturn 和 SQLite3
RUN apt-get update && \
    apt-get install -y coturn && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 创建必要的目录
RUN mkdir -p /etc/coturn /var/tmp

# 复制本地的 turnserver.conf 到容器内
COPY turnserver.conf /etc/coturn/turnserver.conf


# 暴露必要的端口
EXPOSE 23478/tcp 23478/udp
EXPOSE 49160-49200/udp

# 启动 Coturn 服务器，以守护进程模式运行并保持前台
CMD ["turnserver", "-c", "/etc/coturn/turnserver.conf", "--no-cli", "-v"]

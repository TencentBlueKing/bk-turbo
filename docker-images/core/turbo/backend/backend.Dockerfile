FROM blueking/jdk:0.0.1

LABEL maintainer="Tencent BlueKing Devops"

ENV TURBO_HOME=/data/workspace \
    TURBO_LOGS_DIR=/data/workspace/logs \
    TURBO_SERVICE_PREFIX=turbo- \
    TURBO_PROFILE=dev 


COPY ./ /data/workspace/
RUN ln -snf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo 'Asia/Shanghai' > /etc/timezone && \
    chmod +x /data/workspace/startup.sh
WORKDIR /data/workspace
CMD /data/workspace/startup.sh
ARG  BASE_IMAGE

FROM ${BASE_IMAGE}

COPY start.sh /data/bcss/bk-dist-worker/start.sh

RUN chmod a+x /data/bcss/bk-dist-worker/start.sh

CMD ["sh", "-c", "/data/bcss/bk-dist-worker/start.sh"]
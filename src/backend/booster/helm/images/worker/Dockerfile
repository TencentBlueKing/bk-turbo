ARG  BASE_IMAGE

FROM ${BASE_IMAGE}

COPY * /data/bcss/bk-dist-worker/

RUN chmod a+x /data/bcss/bk-dist-worker/start.sh

CMD ["sh", "-c", "/data/bcss/bk-dist-worker/start.sh"]
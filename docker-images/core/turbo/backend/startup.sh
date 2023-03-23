#! /bin/sh

mkdir -p $TURBO_LOGS_DIR
chmod 777 $TURBO_LOGS_DIR

java -server \
     -Dsun.jnu.encoding=UTF-8 \
     -Dfile.encoding=UTF-8 \
     -Xloggc:$TURBO_LOGS_DIR/gc.log \
     -XX:+PrintTenuringDistribution \
     -XX:+PrintGCDetails \
     -XX:+PrintGCDateStamps \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=oom.hprof \
     -XX:ErrorFile=$TURBO_LOGS_DIR/error_sys.log \
     -Djasypt.encryptor.bootstrap=false \
     -Dspring.profiles.active=$TURBO_PROFILE \
     -Dservice.prefix=$TURBO_SERVICE_PREFIX \
     -Dservice.fullname=$TURBO_SERVICE_PREFIX-turbo \
     -Dturbo.thirdparty.propdir=/data/workspace \
     $TURBO_JVM_OPTION \
     -jar /data/workspace/app.jar

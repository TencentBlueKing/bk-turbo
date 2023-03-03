#!/bin/bash
# 用途：批量修改并推送worker镜像

# 安全模式
set -euo pipefail

# 通用脚本框架变量
PROGRAM=$(basename "$0")
EXITCODE=0

SOURCEVERSION=latest
DESTVERSION=latest
PUSH=0
BASEIMAGE=
TARGETIMAGE=
DOCKERFILE=.
OUTPUTFILE=output

usage () {
    cat <<EOF
用法:
    $PROGRAM [OPTIONS]...
            [ -i --input            [必选] 源worker镜像列表的文件 ]
            [ -o --output           [可选] 目的worker镜像列表的文件，默认./output ]
            [ -d --dockerfile       [可选] dockerfile路径，默认当前路径，Dockerfile中必须包含BASE_IMAGE参数 ]
            [ -sv, --source-version [必选] 镜像版本tag, 默认latest ]
            [ -dv, --dest-version   [必选] 镜像版本tag, 默认latest ]
            [ -p, --push            [可选] 推送镜像到docker远程仓库，默认不推送 ]
            [ -h, --help            [可选] 查看脚本帮助 ]
EOF
}

usage_and_exit () {
    usage
    exit "$1"
}

log () {
    echo "$@"
}

error () {
    echo "$@" 1>&2
    usage_and_exit 1
}

warning () {
    echo "$@" 1>&2
    EXITCODE=$((EXITCODE + 1))
}

# 解析命令行参数，长短混合模式
(( $# == 0 )) && usage_and_exit 1
while (( $# > 0 )); do
    case "$1" in
        -sv | --source-version )
            shift
            SOURCEVERSION=$1
            ;;
        -dv | --dest-version )
            shift
            DESTVERSION=$1
            ;;
        -d | --dockerfile )
            shift
            DOCKERFILE=$1
            ;;
        -p | --push )
            PUSH=1
            ;;
        -i | --input )
            shift
            INPUTFILE=$1
            ;;
        -o | --output )
            shift
            OUTPUTFILE=$1
            ;;        
        --help | -h | '-?' )
            usage_and_exit 0
            ;;
        -*)
            error "不可识别的参数: $1"
            ;;
        *)
            break
            ;;
    esac
    shift
done

VERSION=1.0.1
rm -rf $OUTPUTFILE
for line in `cat $INPUTFILE`
do
    BASEIMAGE=$line:$SOURCEVERSION
    TARGETIMAGE=$line:$DESTVERSION
    echo "docker build -f $DOCKERFILE/Dockerfile --build-arg BASE_IMAGE=$BASEIMAGE -t $TARGETIMAGE . --no-cache --network=host"
    docker build -f $DOCKERFILE/Dockerfile --build-arg BASE_IMAGE=$BASEIMAGE -t $TARGETIMAGE . --no-cache --network=host
    if [[ $PUSH -eq 1 ]];then
        docker push $TARGETIMAGE
    fi 
    echo $TARGETIMAGE >> $OUTPUTFILE
done
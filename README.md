![LOGO](docs/resource/img/bkturbo_cn.png)
---
[![license](https://img.shields.io/badge/license-mit-brightgreen.svg?style=flat)](https://github.com/TencentBlueKing/bk-turbo/blob/master/LICENSE.txt) [![Release Version](https://img.shields.io/github/v/release/TencentBlueKing/bk-turbo?include_prereleases)](https://github.com/TencentBlueKing/bk-turbo/releases) 

> **重要提示**: `master` 分支在开发过程中可能处于 *不稳定或者不可用状态* 。
请通过[releases](https://github.com/TencentBlueKing/bk-turbo/releases) 而非 `master` 去获取稳定的二进制文件。

编译构建是项目开发和发布过程中的重要环节，同时也是非常耗时的环境，有些项目执行一次完整的构建需要几十分钟甚至几个小时，各种编译构建加速工具都能在一定程度上减小构建时长。bk-turbo是一个跨平台统一分布式编译加速服务，目前已经支持linux C++编译，UE4多平台C++，shader编译加速，并可快速扩展支持不同编译场景。

## Overview

![image](docs/resource/img/turbo_arch.png)

disttask各模块功能介绍如下：
1： remoter worker 运行在分布式环境中，负责接收，执行和返回分布式任务
2： local server 运行在构建机上，实现分布式任务底层基础功能，并可扩展不同应用场景的分布式任务实现
3： disttask_executor 通用任务执行器，接管实际编译中的编译命令(如 gcc命令，clang命令)，是构建工具和分布式基础服务之间的桥梁
4： 基于disttask提供的接口，根据实际场景需要，实现独立的构建工具

## Features
- 支持linux C/C++编译加速，不受构建工具和构建脚本实现方式限制
- 支持UE4 linux C++编译加速
- 支持UE4 mac C++和shader编译加速
- 支持UE4 windows C++和shader编译加速，跨平台出包等
- 构建过程可视化展示
- 支持扩展实现多种平台的分布式任务，不局限于编译构建任务
- 支持linux，windows平台下容器资源调度
- 支持linux，windows，mac平台主机资源管理
- 支持快速对接不同资源管理平台
- 支持资源分组管理和调配，并支持用户自建资源组加入和使用
- 支持make，cmake，bazel，blade，ninja，vs，ue4，xcodebuild等多种构建工具接入
- 支持修改构建文件和命令的接入方式和系统hook命令方式
- 支持IP,mac地址,业务token等多种访问控制手段


## Experience
- [bk-ci in docker](https://hub.docker.com/r/blueking/bk-ci)
- [bk-repo in docker](https://hub.docker.com/r/bkrepo/bkrepo)

## Getting started
- [下载与编译](docs/overview/source_compile.md)
- [一分钟安装部署](docs/overview/installation.md)
- [独立部署制品库](docs/storage/README.md)

## Support
1. [GitHub讨论区](https://github.com/Tencent/bk-ci/discussions)
2. QQ群：495299374

## BlueKing Community
- [BK-BCS](https://github.com/Tencent/bk-bcs)：蓝鲸容器管理平台是以容器技术为基础，为微服务业务提供编排管理的基础服务平台。
- [BK-CMDB](https://github.com/Tencent/bk-cmdb)：蓝鲸配置平台（蓝鲸CMDB）是一个面向资产及应用的企业级配置管理平台。
- [BK-JOB](https://github.com/Tencent/bk-job)：蓝鲸作业平台(Job)是一套运维脚本管理系统，具备海量任务并发处理能力。
- [BK-PaaS](https://github.com/Tencent/bk-PaaS)：蓝鲸PaaS平台是一个开放式的开发平台，让开发者可以方便快捷地创建、开发、部署和管理SaaS应用。
- [BK-SOPS](https://github.com/Tencent/bk-sops)：蓝鲸标准运维（SOPS）是通过可视化的图形界面进行任务流程编排和执行的系统，是蓝鲸体系中一款轻量级的调度编排类SaaS产品。

## Contributing
- 关于 bk-ci 分支管理、issue 以及 pr 规范，请阅读 [Contributing](CONTRIBUTING.md)
- [腾讯开源激励计划](https://opensource.tencent.com/contribution) 鼓励开发者的参与和贡献，期待你的加入


## License
BK-CI 是基于 MIT 协议， 详细请参考 [LICENSE](LICENSE.txt)

我们承诺未来不会更改适用于交付给任何人的当前项目版本的开源许可证（MIT 协议）。

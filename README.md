![LOGO](docs/resource/img/bkci_cn.png)
---
[![license](https://img.shields.io/badge/license-mit-brightgreen.svg?style=flat)](https://github.com/TencentBlueKing/bk-turbo/blob/master/LICENSE.txt) [![Release Version](https://img.shields.io/github/v/release/TencentBlueKing/bk-turbo?include_prereleases)](https://github.com/TencentBlueKing/bk-turbo/releases) 

[English](README_EN.md) | 简体中文

> **重要提示**: `master` 分支在开发过程中可能处于 *不稳定或者不可用状态* 。
请通过[releases](https://github.com/TencentBlueKing/bk-turbo/releases) 而非 `master` 去获取稳定的二进制文件。

编译构建是项目开发和发布过程中的重要环节，同时也是非常耗时的环境，有些项目执行一次完整的构建需要几十分钟甚至几个小时，各种编译构建加速工具都能在一定程度上减小构建时长。bk-turbo是一个跨平台统一分布式编译加速服务，目前已经支持linux C++编译，UE4多平台C++，shader编译加速，并可快速扩展支持不同编译场景。

## Overview

![image](https://user-images.githubusercontent.com/25241966/222373415-94172897-fd7d-4132-a438-9d539c82ba08.png)

- [架构设计](docs/overview/architecture.md)
- [代码目录](docs/overview/code_framework.md)
- [设计理念](docs/overview/design.md)

## Features
- 持续集成和持续交付: 由于框架的可扩展性，bk-ci既可以用作简单的CI场景，也可以成为企业内所有项目的持续交付中心
- 所见即所得:  bk-ci提供了灵活的可视化编排流水线，动动指尖，将研发流程描述与此
- 架构平行可扩展: 灵活的架构设计可以随意横向扩容，满足企业大规模使用
- 分布式: bk-ci可以便捷的管控多台构建机，助你更快的跨多平台构建、测试和部署
- 流水线插件: bk-ci拥有完善的插件开发体系，其具备了低门槛、灵活可扩展等特性
- 流水线模板: 流水线模板将是企业内部推行研发规范的一大助力
- 代码检查规则集：沉淀团队的代码要求，并能跨项目共享和升级
- 制品库：单一可信源，统一制品仓库，方便管理，提供软件供应链保护

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

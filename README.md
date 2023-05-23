<p align="center">
  <a href="https://github.com/ccfos/nightingale">
    <img src="doc/img/nightingale_logo_h.png" alt="nightingale - cloud native monitoring" width="240" /></a>
</p>

<p align="center">
<img alt="GitHub latest release" src="https://img.shields.io/github/v/release/ccfos/nightingale"/>
<a href="https://n9e.github.io">
  <img alt="Docs" src="https://img.shields.io/badge/docs-get%20started-brightgreen"/></a>
<a href="https://hub.docker.com/u/flashcatcloud">
  <img alt="Docker pulls" src="https://img.shields.io/docker/pulls/flashcatcloud/nightingale"/></a>
<img alt="GitHub Repo stars" src="https://img.shields.io/github/stars/ccfos/nightingale">
<img alt="GitHub Repo issues" src="https://img.shields.io/github/issues/ccfos/nightingale">
<img alt="GitHub Repo issues closed" src="https://img.shields.io/github/issues-closed/ccfos/nightingale">
<img alt="GitHub forks" src="https://img.shields.io/github/forks/ccfos/nightingale">
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img alt="GitHub contributors" src="https://img.shields.io/github/contributors-anon/ccfos/nightingale"/></a>
<a href="https://n9e-talk.slack.com/">
  <img alt="GitHub contributors" src="https://img.shields.io/badge/join%20slack-%23n9e-brightgreen.svg"/></a>
<img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"/>
</p>
<p align="center">
  告警管理专家，一体化开源观测平台！
</p>

[English](./README_en.md) | [中文](./README.md)

## 资料

- 文档：[https://flashcat.cloud/docs/](https://flashcat.cloud/docs/)
- 论坛提问：[https://answer.flashcat.cloud/](https://answer.flashcat.cloud/)
- 报Bug：[https://github.com/ccfos/nightingale/issues](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml)
- 商业版本：[企业版](https://mp.weixin.qq.com/s/FOwnnGPkRao2ZDV574EHrw) | [专业版](https://mp.weixin.qq.com/s/uM2a8QUDJEYwdBpjkbQDxA) 感兴趣请 [联系我们交流试用](https://flashcat.cloud/contact/)

## 功能和特点

- **统一接入各种时序库**：支持对接 Prometheus、VictoriaMetrics、Thanos、Mimir、M3DB 等多种时序库，实现统一告警管理
- **专业告警能力**：内置支持多种告警规则，可以扩展支持所有通知媒介，支持告警屏蔽、告警抑制、告警自愈、告警事件管理
- **无缝搭配 [FlashDuty](https://flashcat.cloud/product/flashcat-duty/)**：实现告警聚合收敛、认领、升级、排班、IM集成，确保告警处理不遗漏，减少打扰，更好协同
- **支持所有常见采集器**：支持 categraf、telegraf、grafana-agent、datadog-agent、给类 exporter 作为采集器，没有什么数据是不能监控的
- **统一的观测平台**：从 v6 版本开始，支持接入 ElasticSearch、Jaeger 数据源，逐步实现日志、链路、指标的一体化观测

## 产品示意图

https://user-images.githubusercontent.com/792850/216888712-2565fcea-9df5-47bd-a49e-d60af9bd76e8.mp4


## 加入交流群

欢迎加入 QQ 交流群，群号：479290895，也可以扫下方二维码加入微信交流群：

<img src="doc/img/wecom.png" width="240">

## Stargazers over time
[![Stargazers over time](https://starchart.cc/ccfos/nightingale.svg)](https://starchart.cc/ccfos/nightingale)

## Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
[Apache License V2.0](https://github.com/didi/nightingale/blob/main/LICENSE)

## 社区管理

[夜莺开源项目和社区治理架构（草案）](./doc/community-governance.md)


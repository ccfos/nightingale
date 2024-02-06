<p align="center">
  <a href="https://github.com/ccfos/nightingale">
    <img src="doc/img/Nightingale_L_V.png" alt="nightingale - cloud native monitoring" width="100" /></a>
</p>
<p align="center">
  <b>开源告警管理专家 一体化的可观测平台</b>
</p>

<p align="center">
<a href="https://flashcat.cloud/docs/">
  <img alt="Docs" src="https://img.shields.io/badge/docs-get%20started-brightgreen"/></a>
<a href="https://hub.docker.com/u/flashcatcloud">
  <img alt="Docker pulls" src="https://img.shields.io/docker/pulls/flashcatcloud/nightingale"/></a>
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img alt="GitHub contributors" src="https://img.shields.io/github/contributors-anon/ccfos/nightingale"/></a>
<img alt="GitHub Repo stars" src="https://img.shields.io/github/stars/ccfos/nightingale">
<img alt="GitHub forks" src="https://img.shields.io/github/forks/ccfos/nightingale">
<br/><img alt="GitHub Repo issues" src="https://img.shields.io/github/issues/ccfos/nightingale">
<img alt="GitHub Repo issues closed" src="https://img.shields.io/github/issues-closed/ccfos/nightingale">
<img alt="GitHub latest release" src="https://img.shields.io/github/v/release/ccfos/nightingale"/>
<img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"/>
<a href="https://n9e-talk.slack.com/">
  <img alt="GitHub contributors" src="https://img.shields.io/badge/join%20slack-%23n9e-brightgreen.svg"/></a>
</p>



[English](./README_en.md) | [中文](./README.md)

## 夜莺 Nightingale 是什么
夜莺 Nightingale 是中国计算机学会接受捐赠并托管的第一个开源项目，是一个 All-in-One 的云原生监控工具，集合了 Prometheus 和 Grafana 的优点，你可以在 WebUI 上管理和配置告警策略，也可以对分布在多个 Region 的指标、日志、链路追踪数据进行统一的可视化和分析。夜莺融入了一线互联网公司可观测性最佳实践，沉淀了众多社区专家经验，开箱即用。[了解更多...](https://flashcat.cloud/product/nightingale/)


## 快速开始
- [文档](https://flashcat.cloud/docs/)
- [提问](https://answer.flashcat.cloud/)
- [下载](https://flashcat.cloud/download/nightingale/)
- [安装](https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v6/install/intro/)
- [报告 Bug](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml)
- 为了提供更快速的访问体验，上述文档和下载站点托管于 [FlashcatCloud](https://flashcat.cloud)

## 功能特点

- 对接多种时序库：支持对接 Prometheus、VictoriaMetrics、Thanos、Mimir、M3DB、TDengine 等多种时序库，实现统一告警管理。
- 专业告警能力：内置支持多种告警规则，可以扩展支持常见通知媒介，支持告警屏蔽/抑制/聚合/自愈、告警事件管理。
- 高性能可视化引擎：支持多种图表样式，内置众多 Dashboard 模版，也可导入 Grafana 模版，开箱即用，开源协议商业友好。
- 无缝搭配 [Flashduty](https://flashcat.cloud/product/flashcat-duty/)：实现告警聚合收敛、认领、升级、排班、IM集成，确保告警处理不遗漏，减少打扰，高效协同。
- 支持常见采集器：支持 [Categraf](https://flashcat.cloud/product/categraf)、Telegraf、Grafana-agent、Datadog-agent、各种 Exporter 作为采集器，没有什么数据是不能监控的。
- 一体化观测平台：从 V6 版本开始，支持对接 ElasticSearch、Jaeger 数据源，实现日志、链路、指标多维度的统一可观测。


## 功能演示
![演示](https://fcpub-1301667576.cos.ap-nanjing.myqcloud.com/n9e/n9e-demo.gif)

## 部署架构
<p align=center>中心化部署</p>

![中心化部署](https://fcpub-1301667576.cos.ap-nanjing.myqcloud.com/flashcat/images/blog/n9e-opensource-china/8.png)

<p align=center>多机房部署</p>

![多机房部署](https://fcpub-1301667576.cos.ap-nanjing.myqcloud.com/flashcat/images/blog/n9e-opensource-china/9.png)

## 交流渠道
- 问题讨论，优先推荐访问[夜莺Answer论坛](https://answer.flashcat.cloud/)
- 反馈Bug，优先推荐提交[夜莺GitHub Issue](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml)
- 推荐完整浏览[夜莺文档站点](https://flashcat.cloud/docs/)，了解更多信息
- 推荐搜索关注夜莺公众号，第一时间获取社区动态：`夜莺监控Nightingale`
- 欢迎加入 QQ 交流群，群号：`479290895` 群友互助交流学习

## 广受关注
[![Stargazers over time](https://api.star-history.com/svg?repos=ccfos/nightingale&type=Date)](https://star-history.com/#ccfos/nightingale&Date)


## 社区共建
- 请阅读浏览[夜莺开源项目和社区治理架构草案](./doc/community-governance.md)，真诚欢迎每一位用户、开发者、公司以及组织，使用夜莺监控、积极反馈 Bug、提交功能需求、分享最佳实践，共建专业、活跃的夜莺开源社区。
- 夜莺贡献者❤️
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
- [Apache License V2.0](https://github.com/didi/nightingale/blob/main/LICENSE)
<p align="center">
  <a href="https://github.com/ccfos/nightingale">
    <img src="doc/img/Nightingale_L_V.png" alt="nightingale - cloud native monitoring" width="240" /></a>
</p>

<p align="center">
<a href="https://flashcat.cloud/docs/">
  <img alt="Docs" src="https://img.shields.io/badge/docs-get%20started-brightgreen"/></a>
<a href="https://hub.docker.com/u/flashcatcloud">
  <img alt="Docker pulls" src="https://img.shields.io/docker/pulls/flashcatcloud/nightingale"/></a>
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img alt="GitHub contributors" src="https://img.shields.io/github/contributors-anon/ccfos/nightingale"/></a>
<img alt="GitHub Repo stars" src="https://img.shields.io/github/stars/ccfos/nightingale">
<br/><img alt="GitHub Repo issues" src="https://img.shields.io/github/issues/ccfos/nightingale">
<img alt="GitHub Repo issues closed" src="https://img.shields.io/github/issues-closed/ccfos/nightingale">
<img alt="GitHub forks" src="https://img.shields.io/github/forks/ccfos/nightingale">
<img alt="GitHub latest release" src="https://img.shields.io/github/v/release/ccfos/nightingale"/>
<img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"/>
<a href="https://n9e-talk.slack.com/">
  <img alt="GitHub contributors" src="https://img.shields.io/badge/join%20slack-%23n9e-brightgreen.svg"/></a>
</p>

<p align="center">
  告警管理专家，一体化的开源可观测平台
</p>

[English](./README_en.md) | [中文](./README.md)

夜莺Nightingale是中国计算机学会接受捐赠并托管的第一个开源项目，是一个 All-in-One 的云原生监控工具，集合了 Prometheus 和 Grafana 的优点，你可以在 WebUI 上管理和配置告警策略，也可以对分布在多个 Region 的指标、日志、链路追踪数据进行统一的可视化和分析。夜莺融入了顶级互联网公司可观测性最佳实践，沉淀了众多社区专家经验，开箱即用[【了解更多】](https://flashcat.cloud/product/nightingale/)


## 资料

- 文档：[flashcat.cloud/docs](https://flashcat.cloud/docs/)
- 提问：[answer.flashcat.cloud](https://answer.flashcat.cloud/)
- 反馈Bug：[github.com/ccfos/nightingale/issues](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml)


## 功能和特点

- **统一接入各种时序库**：支持对接 Prometheus、VictoriaMetrics、Thanos、Mimir、M3DB 等多种时序库，实现统一告警管理
- **专业告警能力**：内置支持多种告警规则，可以扩展支持所有通知媒介，支持告警屏蔽、告警抑制、告警自愈、告警事件管理
- **高性能可视化引擎**：支持多种图表样式，内置众多Dashboard模版，也可导入Grafana模版，开箱即用，开源协议商业友好
- **无缝搭配 [Flashduty](https://flashcat.cloud/product/flashcat-duty/)**：实现告警聚合收敛、认领、升级、排班、IM集成，确保告警处理不遗漏，减少打扰，更好协同
- **支持所有常见采集器**：支持 [Categraf](https://flashcat.cloud/product/categraf)、Telegraf、Grafana-agent、Datadog-agent、各种 Exporter 作为采集器，没有什么数据是不能监控**的
- **一体化观测平台**：从 v6 版本开始，支持接入 ElasticSearch、Jaeger 数据源，实现日志、链路、指标多维度的统一可观测


## 产品演示

![演示](doc/img/n9e-screenshot-gif-v6.gif)

## 部署架构

![架构](doc/img/n9e-arch-latest.png)

## 交流群

1. 问题讨论，优先推荐访问[夜莺Answer论坛](https://answer.flashcat.cloud/)；
2. 反馈Bug，优先推荐通过提交[夜莺Github Issue](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml)
3. 推荐浏览[夜莺文档站点](https://flashcat.cloud/docs/)，了解更多信息；
4. 推荐搜索关注夜莺公众号：**夜莺监控Nightingale**
4. 欢迎加入 QQ 交流群，群号：479290895，群友互助；

## Stargazers over time

[![Stargazers over time](https://api.star-history.com/svg?repos=ccfos/nightingale&type=Date)](https://star-history.com/#ccfos/nightingale&Date)


## Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## 社区治理
[夜莺开源项目和社区治理架构（草案）](./doc/community-governance.md)

## License
[Apache License V2.0](https://github.com/didi/nightingale/blob/main/LICENSE)

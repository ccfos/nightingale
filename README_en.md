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
  An open-source cloud-native monitoring system that is <b>all-in-one</b> <br/>
  <b>Out-of-the-box</b>, it integrates data collection, visualization, and monitoring alert <br/>
  We recommend upgrading your <b>Prometheus + AlertManager + Grafana</b> combination to Nightingale!
</p>

[English](./README.md) | [中文](./README_ZH.md)


## Highlighted Features

- **Out-of-the-box**
  - Supports multiple deployment methods such as **Docker, Helm Chart, and cloud services**, integrates data collection, monitoring, and alerting into one system, and comes with various monitoring dashboards, quick views, and alert rule templates. **It greatly reduces the construction cost, learning cost, and usage cost of cloud-native monitoring systems**.
- **Professional Alerting**
  - Provides visual alert configuration and management, supports various alert rules, offers the ability to configure silence and subscription rules, supports multiple alert delivery channels, and has features such as alert self-healing and event management.
- **Cloud-Native**
  - Quickly builds an enterprise-level cloud-native monitoring system through a turnkey approach, supports multiple collectors such as [Categraf](https://github.com/flashcatcloud/categraf), Telegraf, and Grafana-agent, supports multiple data sources such as Prometheus, VictoriaMetrics, M3DB, ElasticSearch, and Jaeger, and is compatible with importing Grafana dashboards. **It seamlessly integrates with the cloud-native ecosystem**.
- **High Performance and High Availability**
  - Due to the multi-data-source management engine of Nightingale and its excellent architecture design, and utilizing a high-performance time-series database, it can handle data collection, storage, and alert analysis scenarios with billions of time-series data, saving a lot of costs.
  - Nightingale components can be horizontally scaled with no single point of failure. It has been deployed in thousands of enterprises and tested in harsh production practices. Many leading Internet companies have used Nightingale for cluster machines with hundreds of nodes, processing billions of time-series data.
- **Flexible Extension and Centralized Management**
  - Nightingale can be deployed on a 1-core 1G cloud host, deployed in a cluster of hundreds of machines, or run in Kubernetes. Time-series databases, alert engines, and other components can also be decentralized to various data centers and regions, balancing edge deployment with centralized management. **It solves the problem of data fragmentation and lack of unified views**.


#### If you are using Prometheus and have one or more of the following requirement scenarios, it is recommended that you upgrade to Nightingale:

- Multiple systems such as Prometheus, Alertmanager, Grafana, etc. are fragmented and lack a unified view and cannot be used out of the box;
- The way to manage Prometheus and Alertmanager by modifying configuration files has a big learning curve and is difficult to collaborate;
- Too much data to scale-up your Prometheus cluster;
- Multiple Prometheus clusters running in production environments, which faced high management and usage costs;

#### If you are using Zabbix and have the following scenarios, it is recommended that you upgrade to Nightingale:

- Monitoring too much data and wanting a better scalable solution;
- A high learning curve and a desire for better efficiency of collaborative use in a multi-person, multi-team model;
- Microservice and cloud-native architectures with variable monitoring data lifecycles and high monitoring data dimension bases, which are not easily adaptable to the Zabbix data model;


#### If you are using [open-falcon](https://github.com/open-falcon/falcon-plus), we recommend you to upgrade to Nightingale：
- For more information about open-falcon and Nightingale, please refer to read [Ten features and trends of cloud-native monitoring](https://mp.weixin.qq.com/s?__biz=MzkzNjI5OTM5Nw==&mid=2247483738&idx=1&sn=e8bdbb974a2cd003c1abcc2b5405dd18&chksm=c2a19fb0f5d616a63185cd79277a79a6b80118ef2185890d0683d2bb20451bd9303c78d083c5#rd)。

## Getting Started

[English Doc](https://n9e.github.io/) |  [中文文档](http://n9e.flashcat.cloud/)

## Screenshots

https://user-images.githubusercontent.com/792850/216888712-2565fcea-9df5-47bd-a49e-d60af9bd76e8.mp4

## Architecture

<img src="doc/img/arch-product.png" width="600">

Nightingale monitoring can receive monitoring data reported by various collectors (such as [Categraf](https://github.com/flashcatcloud/categraf) , telegraf, grafana-agent, Prometheus, etc.) and write them to various popular time-series databases (such as Prometheus, M3DB, VictoriaMetrics, Thanos, TDEngine, etc.). It provides configuration capabilities for alert rules, silence rules, and subscription rules, as well as the ability to view monitoring data. It also provides automatic alarm self-healing mechanisms (such as automatically calling back to a webhook address or executing a script after an alarm is triggered), and the ability to store and manage historical alarm events and view them in groups.

If the performance of a standalone time-series database (such as Prometheus) has bottlenecks or poor disaster recovery, we recommend using [VictoriaMetrics](https://github.com/VictoriaMetrics/VictoriaMetrics). The VictoriaMetrics architecture is relatively simple, has excellent performance, and is easy to deploy and maintain. The architecture diagram is as shown above. For more detailed documentation on VictoriaMetrics, please refer to its [official website](https://victoriametrics.com/).

**We welcome you to participate in the Nightingale open-source project and community in various ways, including but not limited to**：
- Adding and improving documentation => [n9e.github.io](https://n9e.github.io/)
- Sharing your best practices and experience in using Nightingale monitoring => [Article sharing]((https://n9e.github.io/docs/prologue/share/))
- Submitting product suggestions => [github issue](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Ffeature&template=enhancement.md)
- Submitting code to make Nightingale monitoring faster, more stable, and easier to use => [github pull request](https://github.com/didi/nightingale/pulls)


**Respecting, recognizing, and recording the work of every contributor** is the first guiding principle of the Nightingale open-source community. We advocate effective questioning, which not only respects the developer's time but also contributes to the accumulation of knowledge in the entire community
- Before asking a question, please first refer to the [FAQ](https://www.gitlink.org.cn/ccfos/nightingale/wiki/faq) 
- We use [GitHub Discussions](https://github.com/ccfos/nightingale/discussions) as the communication forum. You can search and ask questions here.
- We also recommend that you join ours [Slack channel](https://n9e-talk.slack.com/) to exchange experiences with other Nightingale users.


## Who is using Nightingale
You can register your usage and share your experience by posting on **[Who is Using Nightingale](https://github.com/ccfos/nightingale/issues/897)**.

## Stargazers over time
[![Stargazers over time](https://starchart.cc/ccfos/nightingale.svg)](https://starchart.cc/ccfos/nightingale)

## Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
[Apache License V2.0](https://github.com/didi/nightingale/blob/main/LICENSE)
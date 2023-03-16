<img src="doc/img/ccf-n9e.png" width="240">

 Nightingale is an enterprise-level cloud-native monitoring system, which can be used as drop-in replacement of Prometheus for alerting and management.   

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

## Quickstart
- [n9e.github.io/quickstart](https://n9e.github.io/docs/install/compose/)

## Documentation
- [n9e.github.io](https://n9e.github.io/)

## Example of use

<img src="doc/img/intro.gif" width="680">

## System Architecture
#### A typical Nightingale deployment architecture:
<img src="doc/img/arch-system.png" width="680">

#### Typical deployment architecture using VictoriaMetrics as storage:
<img src="doc/img/install-vm.png" width="680">

## Contact us and feedback questions
- We recommend that you use [github issue](https://github.com/ccfos/nightingale/issues) as the preferred channel for issue feedback and requirement submission;
- You can join our WeChat group

<img src="doc/img/wecom.png" width="120">


## Contributing
We welcome your participation in the Nightingale open source project and open source community in a variety of ways:
- Feedback on problems and bugs  => [github issue](https://github.com/ccfos/nightingale/issues)
- Additional and improved documentation => [n9e.github.io](https://n9e.github.io/)
- Share your best practices and insights on using Nightingale => [User Story](https://github.com/ccfos/nightingale/issues/897)
- Join our community events => [Nightingale wechat group](https://s3-gz01.ccfosstatic.com/n9e-pub/image/n9e-wx.png)
- Submit code to make Nightingale better =>[github PR](https://github.com/ccfos/nightingale/pulls)

## Star History
[![Star History Chart](https://api.star-history.com/svg?repos=ccfos/nightingale&type=Date)](https://star-history.com/#ccfos/nightingale)
## Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
Nightingale with [Apache License V2.0](https://github.com/ccfos/nightingale/blob/main/LICENSE) open source license.

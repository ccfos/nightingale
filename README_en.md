<p align="center">
  <a href="https://github.com/ccfos/nightingale">
    <img src="doc/img/Nightingale_L_V.png" alt="nightingale - cloud native monitoring" width="100" /></a>
</p>
<p align="center">
  <b>Open-source Alert Management Expert, an Integrated Observability Platform</b>
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

## What is Nightingale

Nightingale is an open-source project focused on alerting. Similar to Grafana's data source integration approach, Nightingale also connects with various existing data sources. However, while Grafana focuses on visualization, Nightingale focuses on alerting engines.

Originally developed and open-sourced by Didi, Nightingale was donated to the China Computer Federation Open Source Development Committee (CCF ODC) on May 11, 2022, becoming the first open-source project accepted by the CCF ODC after its establishment. 


## Quick Start

- 👉 [Documentation](https://flashcat.cloud/docs/) | [Download](https://flashcat.cloud/download/nightingale/)
- ❤️ [Report a Bug](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=&projects=&template=question.yml)
- ℹ️ For faster access, the above documentation and download sites are hosted on [FlashcatCloud](https://flashcat.cloud).

## Features

- **Integration with Multiple Time-Series Databases:** Supports integration with various time-series databases such as Prometheus, VictoriaMetrics, Thanos, Mimir, M3DB, and TDengine, enabling unified alert management.
- **Advanced Alerting Capabilities:** Comes with built-in support for multiple alerting rules, extensible to common notification channels. It also supports alert suppression, silencing, subscription, self-healing, and alert event management.
- **High-Performance Visualization Engine:** Offers various chart styles with numerous built-in dashboard templates and the ability to import Grafana templates. Ready to use with a business-friendly open-source license.
- **Support for Common Collectors:** Compatible with [Categraf](https://flashcat.cloud/product/categraf), Telegraf, Grafana-agent, Datadog-agent, and various exporters as collectors—there's no data that can't be monitored.
- **Seamless Integration with [Flashduty](https://flashcat.cloud/product/flashcat-duty/):** Enables alert aggregation, acknowledgment, escalation, scheduling, and IM integration, ensuring no alerts are missed, reducing unnecessary interruptions, and enhancing efficient collaboration.


## Screenshots

You can switch languages and themes in the top right corner. We now support English, Simplified Chinese, and Traditional Chinese. 

![18n switch](doc/img/readme/n9e-switch-i18n.png)

### Instant Query

Similar to the built-in query analysis page in Prometheus, Nightingale offers an ad-hoc query feature with UI enhancements. It also provides built-in PromQL metrics, allowing users unfamiliar with PromQL to quickly perform queries.

![Instant Query](doc/img/readme/20240513103305.png)

### Metric View

Alternatively, you can use the Metric View to access data. With this feature, Instant Query becomes less necessary, as it caters more to advanced users. Regular users can easily perform queries using the Metric View.

![Metric View](doc/img/readme/20240513103530.png)

### Built-in Dashboards

Nightingale includes commonly used dashboards that can be imported and used directly. You can also import Grafana dashboards, although compatibility is limited to basic Grafana charts. If you’re accustomed to Grafana, it’s recommended to continue using it for visualization, with Nightingale serving as an alerting engine.

![Built-in Dashboards](doc/img/readme/20240513103628.png)

### Built-in Alert Rules

In addition to the built-in dashboards, Nightingale also comes with numerous alert rules that are ready to use out of the box.

![Built-in Alert Rules](doc/img/readme/20240513103825.png)



## Architecture

In most community scenarios, Nightingale is primarily used as an alert engine, integrating with multiple time-series databases to unify alert rule management. Grafana remains the preferred tool for visualization. As an alert engine, the product architecture of Nightingale is as follows:

![Product Architecture](doc/img/readme/20240221152601.png)

For certain edge data centers with poor network connectivity to the central Nightingale server, we offer a distributed deployment mode for the alert engine. In this mode, even if the network is disconnected, the alerting functionality remains unaffected.

![Edge Deployment Mode](doc/img/readme/20240222102119.png)


## Communication Channels

- **Report Bugs:** It is highly recommended to submit issues via the [Nightingale GitHub Issue tracker](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Fbug&projects=&template=bug_report.yml).
- **Documentation:** For more information, we recommend thoroughly browsing the [Nightingale Documentation Site](https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v7/introduction/).

## Stargazers over time

[![Stargazers over time](https://api.star-history.com/svg?repos=ccfos/nightingale&type=Date)](https://star-history.com/#ccfos/nightingale&Date)

## Community Co-Building

- ❇️ Please read the [Nightingale Open Source Project and Community Governance Draft](./doc/community-governance.md). We sincerely welcome every user, developer, company, and organization to use Nightingale, actively report bugs, submit feature requests, share best practices, and help build a professional and active open-source community.
-  ❤️ Nightingale Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
- [Apache License V2.0](https://github.com/didi/nightingale/blob/main/LICENSE)

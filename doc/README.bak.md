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
  An <b>all-in-one</b> open-source observability platform <br/>
  <b>Ready out of the box</b>, combining data collection, visualization, and monitoring & alerting <br/>
  We recommend upgrading your <b>Prometheus + AlertManager + Grafana + ELK + Jaeger</b> stack to Nightingale!
</p>

[English](./README_en.md) | [Chinese](./README.md)



## Features and Highlights

- **Ready out of the box**
  - Supports Docker, Helm Chart, cloud services, and other deployment options. It combines data collection, monitoring & alerting, and visualization in one place, and ships with built-in dashboards, quick views, and alert rule templates that work as soon as you import them, **greatly reducing the build, learning, and operating cost of a cloud-native monitoring system**;
- **Professional alerting**
  - Visual alert configuration and management, a rich set of alert rules, configurable mute and subscription rules, multiple alert delivery channels, alert self-healing, alert event management, and more;
  - **We recommend pairing Nightingale with [FlashDuty](https://flashcat.cloud/product/flashcat-duty/) for alert aggregation and deduplication, acknowledgement, escalation, on-call scheduling, and collaboration — so alerts reach the right people efficiently and nothing slips through unhandled**.
- **Cloud native**
  - Build an enterprise-grade cloud-native monitoring stack turnkey. Supports collectors such as [Categraf](https://github.com/flashcatcloud/categraf), Telegraf, and Grafana-agent, and data sources such as Prometheus, VictoriaMetrics, M3DB, ElasticSearch, and Jaeger. It can also import Grafana dashboards, **integrating seamlessly with the cloud-native ecosystem**;
- **High performance, high availability**
  - Thanks to Nightingale's multi-datasource management engine and the solid architecture on the engine side, plus a high-performance time-series database, it can handle collection, storage, and alert analysis for hundreds of millions of time series while saving significant cost;
  - Every Nightingale component scales horizontally with no single point of failure. It has been deployed in thousands of companies and proven under demanding production conditions. Many leading internet companies run Nightingale clusters of a hundred machines handling hundreds of millions of time series;
- **Flexible scaling, centralized management**
  - Nightingale runs on a 1-core / 1 GB cloud host, scales to a cluster of hundreds of machines, and runs in Kubernetes. You can also push components such as the time-series database and the alerting engine down into each data center or region, combining edge deployment with centralized management to **solve the problem of fragmented data and the lack of a unified view**;
- **Open community**
  - Hosted by the [Open Source Development Committee of the China Computer Federation](https://www.ccf.org.cn/kyfzwyh/), with sustained investment from [Flashcat](https://flashcat.cloud) and many other companies, active participation from thousands of community users, and a clear project positioning — all of which keep the Nightingale open-source community healthy for the long run. Active, expert community users keep iterating on the product and folding more best practices into it;

## Use Cases
1. **If you want to manage and view Metrics, Logging, and Tracing data on a single platform, we recommend Nightingale**:
   - Further reading: [More than monitoring: Nightingale V6 becomes an open-source observability platform](http://flashcat.cloud/blog/nightingale-v6-release/)
2. **If you use Prometheus and hit one or more of the following, we recommend a seamless upgrade to Nightingale**:
   - Prometheus, Alertmanager, and Grafana feel fragmented, there is no unified view, and nothing works out of the box;
   - Managing Prometheus and Alertmanager by editing configuration files has a steep learning curve and makes collaboration hard;
   - The data volume has grown beyond what your Prometheus cluster can scale to;
   - Running multiple Prometheus clusters in production is expensive to manage and use;
3. **If you use Zabbix and run into the following, we recommend upgrading to Nightingale**:
   - The monitoring data volume is too large and you want a solution that scales better;
   - The learning curve is steep, and you want better collaboration efficiency across multiple people and teams;
   - Under microservice and cloud-native architectures, monitoring data has a volatile lifecycle and high-cardinality dimensions that the Zabbix data model does not adapt to well;
   - For a deeper comparison between Zabbix and Nightingale, see [Zabbix vs. Nightingale: choosing a monitoring system](https://flashcat.cloud/blog/zabbx-vs-nightingale/)
4. **If you use [Open-Falcon](https://github.com/open-falcon/falcon-plus), we recommend upgrading to Nightingale:**
   - For a detailed comparison of Open-Falcon and Nightingale, see [Ten characteristics and trends of cloud-native monitoring](http://flashcat.cloud/blog/10-trends-of-cloudnative-monitoring/)
   - For the difference between a monitoring system and an observability platform, see [From monitoring system to observability platform: how big is the gap?
](https://flashcat.cloud/blog/gap-of-monitoring-to-o11y/)
5. **We recommend [Categraf](https://github.com/flashcatcloud/categraf) as your first-choice collector**:
   - [Categraf](https://github.com/flashcatcloud/categraf) is Nightingale's default collector. Built around an open plugin mechanism and an all-in-one design, it collects metrics, logs, traces, and events. Categraf gathers system-level metrics such as CPU, memory, and network, and also integrates collection for many open-source components and the Kubernetes ecosystem. It ships with matching dashboards and alert rules that work out of the box.

## Documentation

[English Doc](https://n9e.github.io/) |  [Chinese Doc](https://flashcat.cloud/docs/)

## Product Walkthrough

https://user-images.githubusercontent.com/792850/216888712-2565fcea-9df5-47bd-a49e-d60af9bd76e8.mp4

## Architecture

Nightingale receives monitoring data reported by various collectors (such as [Categraf](https://github.com/flashcatcloud/categraf), telegraf, grafana-agent, and Prometheus) and writes it into a range of popular time-series databases (Prometheus, M3DB, VictoriaMetrics, Thanos, TDEngine, and others). It lets you configure alert rules, mute rules, and subscription rules, view monitoring data, and use alert self-healing (calling a webhook or running a script automatically once an alert fires). It also stores historical alert events and lets you browse them by group.

### Centralized deployment

![Centralized deployment](https://download.flashcat.cloud/ulric/20230327133406.png)

Nightingale has a single module, n9e. You can deploy several n9e instances as a cluster. n9e depends on two data stores: a database and Redis. For the database you can pick either MySQL or Postgres, whichever suits you.

n9e exposes an HTTP interface, so the load balancer in front of it can be layer 4 or layer 7. Nginx is usually a fine choice.

Once the n9e module receives data, it needs to forward it to a backing time-series database. The relevant configuration is:

```toml
[Pushgw]
LabelRewrite = true
[[Pushgw.Writers]] 
Url = "http://127.0.0.1:9090/api/v1/write"
```

> Note: even though data sources can be configured in the UI, the reporting and forwarding path still has to be specified in the configuration file.

Agents in every data center (Categraf, Telegraf, Grafana-agent, Datadog-agent, and so on) push data directly to n9e. This is the simplest architecture with the lowest maintenance cost — provided the network links between data centers are good, typically over dedicated lines. If the links are poor, use the deployment described below instead.

### Mixed deployment with edge components

![Mixed deployment with edge components](https://download.flashcat.cloud/ulric/20230327135615.png)

This diagram illustrates three different situations. Data center A has a good link to the center, so Categraf can report data directly to the central n9e module. Another data center has a poor link, so the time-series database has to be deployed at the edge; once it is, the alerting engine and the forwarding gateway must follow, so that data never crosses data-center boundaries and the setup stays stable. Heartbeats still go to the center, though — otherwise the machines' CPU and memory usage will not show up in the object list. In yet another case you may be onboarding an existing Prometheus whose collection does not go through Categraf; then you only need to add that Prometheus as a Nightingale data source. You can view graphs and configure alert rules in Nightingale, but the hosts will not appear in the object list and alert self-healing is unavailable — not a big deal, since none of the core features are affected.

When deploying the time-series database, alerting engine, and forwarding gateway at an edge data center, note that the alerting engine depends on the database, because it has to sync alert rules, and the forwarding gateway depends on it too, because it registers objects into the database. Make sure the network path to the database is open. Neither the alerting engine nor the forwarding gateway uses Redis, so no network path to Redis is needed.

### VictoriaMetrics cluster architecture
<img src="doc/img/install-vm.png" width="600">

If a single-node time-series database (such as Prometheus) becomes a performance bottleneck or offers poor disaster recovery, we recommend [VictoriaMetrics](https://github.com/VictoriaMetrics/VictoriaMetrics). VictoriaMetrics has a fairly simple architecture, excellent performance, and is easy to deploy and operate; its architecture is shown above. For more detailed VictoriaMetrics documentation, see its [official site](https://victoriametrics.com/).

## Community

An open-source project only stays alive through an open governance structure and a steady stream of developers and users taking part. We are committed to building an open, neutral governance structure that brings in more developers from companies, universities, and elsewhere who are interested in and enthusiastic about cloud-native monitoring, so we can grow a vibrant Nightingale open-source community together. For the *Nightingale Open Source Project and Community Governance (Draft)*, see [COMMUNITY GOVERNANCE](./doc/community-governance.md).

**We welcome you to take part in the Nightingale open-source project and community in any way you like, including but not limited to**:
- Extending and improving the documentation => [n9e.github.io](https://n9e.github.io/)
- Sharing your best practices and experience using Nightingale => [Shared articles](https://flashcat.cloud/docs/content/flashcat-monitor/nightingale/share/)
- Submitting product suggestions => [GitHub issue](https://github.com/ccfos/nightingale/issues/new?assignees=&labels=kind%2Ffeature&template=enhancement.md)
- Submitting code to make Nightingale faster, more stable, and easier to use => [GitHub pull request](https://github.com/ccfos/nightingale/pulls)

**Respecting, recognizing, and recording the work of every contributor** is the first guiding principle of the Nightingale open-source community. We encourage **asking questions efficiently**: it respects developers' time and contributes to the community's shared knowledge:
- Check the [FAQ](https://www.gitlink.org.cn/ccfos/nightingale/wiki/faq) before asking
- We use the [forum](https://answer.flashcat.cloud/) for discussion; search there or post your question


## Who is using Nightingale

You can register your usage under **[Who is Using Nightingale](https://github.com/ccfos/nightingale/issues/897)** and share your experience.

## Stargazers over time
[![Stargazers over time](https://starchart.cc/ccfos/nightingale.svg)](https://starchart.cc/ccfos/nightingale)

## Contributors
<a href="https://github.com/ccfos/nightingale/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ccfos/nightingale" />
</a>

## License
[Apache License V2.0](https://github.com/ccfos/nightingale/blob/main/LICENSE)

## Join the chat group

<img src="doc/img/wecom.png" width="120">

# LLM应用指标及Trace采集

针对LLM应用的监控，这里支持满足OpenTelementry[相关协议规范](https://opentelemetry.io/docs/specs/semconv/gen-ai/)的采集插件，支持采集LLM应用的相关指标及Trace数据。

目前主流的OpenTelementry LLM采集插件包括：

- OpenLLMetry：一个基于OpenTelemetry的开源项目，专门用于观测LLM应用，其LLM语义约定已被OTel项目官方采纳；
- OpenLIT：提供与OpenLLMetry同级别的OTel原生自动埋点能力，支持更广的模型覆盖面；

相关的插件使用示例，请参考内置文档-最佳实践-数据采集部分。
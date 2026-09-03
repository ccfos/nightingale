# LLM Application Metrics and Trace Collection

For monitoring LLM applications, collection plugins that comply with the OpenTelemetry [GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/) are supported, enabling collection of metrics and trace data from LLM applications.

The mainstream OpenTelemetry LLM collection plugins currently include:

- OpenLLMetry: an OpenTelemetry-based open-source project dedicated to observing LLM applications; its LLM semantic conventions have been officially adopted by the OTel project;
- OpenLIT: provides OTel-native auto-instrumentation capabilities on par with OpenLLMetry, with broader model coverage;

For usage examples of these plugins, please refer to the built-in documentation, under Best Practices - Data Collection.

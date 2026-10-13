# Nightingale architecture diagram

The English README uses [architecture-overseas.svg](architecture-overseas.svg).
Edit this SVG directly: text, shapes, and connectors remain vector elements, so
labels stay sharp when the diagram is resized. Integration marks are embedded
PNG assets; the SVG requires no external fonts, scripts, or image requests.

The flat diagram replaces the earlier generated illustration. It preserves the
same data sources, alerting stages, notification channels, and built-in AI/MCP
capabilities. The left arrow represents query results returned to Nightingale;
monitoring data remains in the existing data sources.

## Export and preview

From the repository root, use `rsvg-convert` (librsvg) to update the 2× PNG:

```sh
rsvg-convert -w 2400 \
  -o doc/img/readme/architecture-overseas.png \
  doc/img/readme/architecture-overseas.svg
```

The PNG is 2400 × 1172. Check readability at a typical README width as well:

```sh
rsvg-convert -w 900 \
  -o /tmp/nightingale-architecture-preview.png \
  doc/img/readme/architecture-overseas.svg
```

## Asset sources

- Nightingale: [existing integration icon](../../../integrations/N9E/icon/nightingale.png).
- VictoriaMetrics: [existing integration icon](../../../integrations/VictoriaMetrics/icon/VictoriaMetrics.png).
- Other integration marks: the corresponding PNG files under
  [`n9e/fe/public/image/logos`](https://github.com/n9e/fe/tree/58e8e242e88d834282bbeb8da9a5dd8d38100d17/public/image/logos)
  (`prometheus`, `elasticsearch`, `loki`, `ck`, `pgsql`, `slack`, `pagerduty`,
  `discord`, `telegram`, `jira`).
- Email: a simple vector envelope drawn in the SVG.

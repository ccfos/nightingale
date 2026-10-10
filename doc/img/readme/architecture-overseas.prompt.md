# Nightingale architecture illustration

Generated with the built-in `image_gen` tool for the English README.

Source references:

- [Previous architecture diagram](20240221152601.png)
- [Nightingale logo](../Nightingale_L_V.png)

## Generation prompt

Use case: infographic-diagram.
Asset: a polished landscape architecture illustration for the English GitHub README of Nightingale, an open-source alerting product.

Edit and completely redesign the supplied old architecture diagram into a premium, contemporary version for an international engineering audience. Image 1 is the old diagram to redesign. Image 2 is the authoritative Nightingale logo to preserve in the center: retain the recognizable purple/blue circular bird silhouette and the Nightingale name, not a generic bird or a new logo. Produce a new composition, not an annotated screenshot.

Canvas: wide landscape about 2:1, at least 2000 pixels wide. Opaque, near-white background with the faintest lavender ambient tint. Elegant restrained 2.5D product illustration: crisp front-facing cards, subtle soft shadows and very slight depth, fine light-gray borders, precise grid alignment, generous whitespace. Purple is the architectural accent; integrations keep their familiar brand colors. No dramatic perspective that makes labels difficult to read. All text must be sharp, high contrast, natural sans-serif, correctly spelled and legible when the image is displayed 900px wide in a README.

Three clear columns, left to right, following the original's architecture:
LEFT heading exactly "DATA SOURCES". Six well-balanced compact cards with recognizable brand marks and these exact labels, each once: "Prometheus", "VictoriaMetrics", "Elasticsearch", "Loki", "ClickHouse", "PostgreSQL". A clean arrangement of two columns by three rows is fine. Fine connectors from these source cards merge toward Nightingale, with arrows representing query results flowing into the engine. Label this connection only "Query results".
CENTER: one prominent elevated card, beautifully detailed in restrained translucent lavender/white. Place the preserved Nightingale bird mark prominently, with "Nightingale" below it. Below that, show three readable capability lines exactly "Alert rules", "Event processing", "Notification routing". Add a tasteful small integrated purple chip exactly "Built-in AI agent" and a separate small "MCP" chip. These are capabilities INSIDE Nightingale, not external data sources or notification services.
RIGHT heading exactly "NOTIFICATION CHANNELS". Six compact cards with recognizable brand marks or email icon and these exact labels, each once: "Slack", "PagerDuty", "Discord", "Email", "Telegram", "Jira". Keep all six equal in visual quality; make Slack and PagerDuty the top row. Connect Nightingale to these destination cards with thin branching lines and subtle arrowheads flowing to the right. Label the outgoing connection only "Alerts".

The diagram must remain a clear functional architecture, with every destination connected only through Nightingale. Do not connect unrelated cards to each other. No fake user interfaces. No Chinese text. No FlashDuty, WeCom, DingTalk, Feishu, Microsoft Teams, flags, company customer logos, on-call functionality inside Nightingale, or extra services. No giant title, marketing slogan, footer, watermark, decorative paragraphs, duplicate labels, or heavy glow. The final should feel crafted for an established developer tool: attractive branded illustration with disciplined information design.

#!/usr/bin/env python3
"""Generate before/after PNG evidence for Nightingale PR #3371 (DingTalk newlines)."""
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).resolve().parents[1] / "docs" / "pr-evidence" / "3371"


def _font(size: int):
    for name in ("msyh.ttc", "arial.ttf", "DejaVuSans.ttf"):
        try:
            return ImageFont.truetype(name, size)
        except OSError:
            continue
    return ImageFont.load_default()


def write_card(path: Path, title: str, lines: list[str]) -> None:
    width, height = 920, 80 + 32 * len(lines)
    img = Image.new("RGB", (width, height), (248, 249, 251))
    draw = ImageDraw.Draw(img)
    title_font = _font(22)
    body_font = _font(18)
    draw.text((24, 20), title, fill=(17, 24, 39), font=title_font)
    y = 64
    for line in lines:
        draw.text((24, y), line, fill=(55, 65, 81), font=body_font)
        y += 32
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path)


def main() -> None:
    write_card(
        OUT / "before.png",
        "Before — DingTalk message (issue #3371)",
        [
            "级别状态: S3 Triggered   \\n规则标题: 服务器通用告警-主机内存不足",
            "规则备注: 主机 xx 内存使用率为 85.75%     \\n监控指标: [cluster=xx]",
            "(客户端显示字面量 \\n，不是换行)",
        ],
    )
    write_card(
        OUT / "after.png",
        "After — same template with fix/message-tpl-newlines-im-channels",
        [
            "级别状态: S3 Triggered",
            "规则标题: 服务器通用告警-主机内存不足",
            "规则备注: 主机 xx 内存使用率为 85.75%",
            "监控指标: [cluster=xx]",
        ],
    )


if __name__ == "__main__":
    main()

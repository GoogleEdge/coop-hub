#!/usr/bin/env python3
"""生成揭东一初联机大厅图标素材（CI 内运行，依赖 Pillow）。

用法：
  python gen_icons.py            # 默认：带校名
  python gen_icons.py --plain    # 不含校名，仅图形
产出（写入仓库根 assets/ 目录，供文档与发布说明使用）：
  assets/logo.png         512x512 主图
  assets/logo-256.png     256x256
  assets/logo.ico         多尺寸 ICO
"""
import argparse
import os

from PIL import Image, ImageDraw, ImageFont

PRIMARY = (46, 90, 172)      # #2E5AAC 深蓝
ACCENT = (58, 143, 108)      # #3A8F6C 绿（frp 标识色）
SCHOOL = "揭东一初"
SUB = "联机大厅"


def base_logo(size: int, plain: bool) -> Image.Image:
    """画主图：深蓝圆角底 + 白色圆环（虚拟局域网的“环”意象）。"""
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    r = int(size * 0.12)
    d.rounded_rectangle([0, 0, size - 1, size - 1], radius=r, fill=PRIMARY + (255,))
    cx = cy = size / 2
    ring_r = size * 0.30
    width = max(4, int(size * 0.06))
    d.ellipse([cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
              outline=(255, 255, 255, 255), width=width)
    # 圆环上的两个节点，象征两台互联的电脑
    node = size * 0.05
    for fx, fy in [(0.5, 0.20), (0.5, 0.80)]:
        px, py = cx, cy + (fy - 0.5) * 2 * ring_r
        d.ellipse([px - node, py - node, px + node, py + node],
                  fill=ACCENT + (255,))
    if not plain and size >= 256:
        # 校名居中
        try:
            font = ImageFont.truetype("wqy-zenhei.ttc", int(size * 0.18))
        except OSError:
            try:
                font = ImageFont.truetype("DejaVuSans.ttf", int(size * 0.18))
            except OSError:
                font = ImageFont.load_default()
        d.text((cx, cy), SCHOOL, font=font, fill=(255, 255, 255, 255), anchor="mm")
    return img


def main() -> None:
    ap = argparse.ArgumentParser(description="生成大厅图标素材")
    ap.add_argument("--plain", action="store_true", help="不含校名，仅图形")
    args = ap.parse_args()

    out_dir = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "assets")
    os.makedirs(out_dir, exist_ok=True)

    base_logo(512, args.plain).save(os.path.join(out_dir, "logo.png"))
    base_logo(256, args.plain).save(os.path.join(out_dir, "logo-256.png"))

    ico_sizes = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    imgs = [base_logo(s, args.plain) for s, _ in ico_sizes]
    # 用 256px 基底保存：Pillow 的 sizes 只向下缩放，从 16px 基底出发只会得到单个 16x16 条目
    imgs[-1].save(os.path.join(out_dir, "logo.ico"), sizes=ico_sizes)
    print(f"图标已生成到 {out_dir}")


if __name__ == "__main__":
    main()

"""生成 rtp 应用图标：服务端（靛蓝）、客户端（青绿）、通用（紫）。
输出 assets/icons/ 下的多尺寸 PNG 与 .ico，供 Windows 资源与文档使用。
"""
import os
from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # 仓库根
OUT = os.path.join(ROOT, "assets", "icons")
os.makedirs(OUT, exist_ok=True)

SIZES = [16, 24, 32, 48, 64, 128, 256]

THEMES = {
    "server": ("#4F46E5", "#7C3AED"),   # 靛蓝 → 紫：服务端
    "client": ("#0D9488", "#22D3EE"),   # 青绿 → 青：客户端
    "rtp":    ("#4F46E5", "#A78BFA"),   # 品牌紫
}


def draw_icon(size, c1, c2, badge=None):
    """画一个圆角渐变方块 + 白色隧道口与箭头。badge: 'S' 服务端 / 'C' 客户端。"""
    S = size * 8  # 超采样
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    r = int(S * 0.22)
    # 圆角矩形渐变底
    grad = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gd = ImageDraw.Draw(grad)
    a1 = tuple(int(c1[i:i + 2], 16) for i in (1, 3, 5)) + (255,)
    a2 = tuple(int(c2[i:i + 2], 16) for i in (1, 3, 5)) + (255,)
    for y in range(S):
        t = y / max(S - 1, 1)
        gd.line([(0, y), (S, y)],
                fill=tuple(int(a1[i] + (a2[i] - a1[i]) * t) for i in range(4)))
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=r, fill=255)
    img.paste(grad, (0, 0), mask)

    W = (255, 255, 255, 255)
    lw = max(2, int(S * 0.055))

    # 隧道口：左侧竖条 + 上下横条组成的「C」形通道
    x0, y0 = int(S * 0.30), int(S * 0.30)
    x1, y1 = int(S * 0.70), int(S * 0.72)
    d.rounded_rectangle([x0, y0, x0 + lw, y1], radius=lw // 2, fill=W)
    d.rounded_rectangle([x0, y0, x1, y0 + lw], radius=lw // 2, fill=W)
    d.rounded_rectangle([x0, y1 - lw, x1, y1], radius=lw // 2, fill=W)

    # 箭头：向右
    ax = int(S * 0.62)
    ay = int(S * 0.50)
    aw = int(S * 0.10)
    ah = int(S * 0.15)
    d.polygon([(ax, ay - ah), (ax + aw * 2, ay), (ax, ay + ah)], fill=W)

    # 角标：服务端 S / 客户端 C
    if badge:
        bs = int(S * 0.40)
        bx, by = S - bs - int(S * 0.04), S - bs - int(S * 0.04)
        bd = ImageDraw.Draw(img)
        bd.ellipse([bx, by, bx + bs, by + bs], fill=(15, 23, 42, 235))
        try:
            from PIL import ImageFont
            f = ImageFont.load_default(int(bs * 0.62))
            t = badge
            tb = bd.textbbox((0, 0), t, font=f)
            bd.text((bx + (bs - (tb[2] - tb[0])) / 2 - tb[0],
                     by + (bs - (tb[3] - tb[1])) / 2 - tb[1]), t, font=f, fill=W)
        except Exception:
            pass

    return img.resize((size, size), Image.LANCZOS)


for name, (c1, c2) in THEMES.items():
    badge = {"server": "S", "client": "C"}.get(name)
    base = draw_icon(256, c1, c2, badge)
    for s in SIZES:
        base.resize((s, s), Image.LANCZOS).save(os.path.join(OUT, f"{name}-{s}.png"))
    base.save(os.path.join(OUT, f"{name}.png"))
    base.save(os.path.join(OUT, f"{name}.ico"),
              sizes=[(s, s) for s in SIZES])
    print("generated", name)

# 文档用横向 logo
for name, (c1, c2) in THEMES.items():
    badge = {"server": "S", "client": "C"}.get(name)
    sq = draw_icon(256, c1, c2, badge)
    W, H = 1280, 320
    canvas = Image.new("RGBA", (W, H), (255, 255, 255, 0))
    canvas.paste(sq.resize((256, 256), Image.LANCZOS), (32, 32), sq.resize((256, 256), Image.LANCZOS))
    d = ImageDraw.Draw(canvas)
    from PIL import ImageFont
    f = ImageFont.load_default(150)
    d.text((330, 80), "rtp", font=f, fill=(15, 23, 42, 255))
    f2 = ImageFont.load_default(52)
    label = {"server": "服务端", "client": "客户端", "rtp": "内网穿透"}[name]
    d.text((560, 150), label, font=f2, fill=(100, 116, 139, 255))
    canvas.save(os.path.join(OUT, f"logo-{name}.png"))
    print("logo", name)
print("done ->", os.path.normpath(OUT))
"""Regenerate app/web icons from the user-provided RGBA logo (requires Pillow)."""
from pathlib import Path
from PIL import Image

assets = Path(__file__).resolve().parent
source = Image.open(assets / "logo-original.png").convert("RGBA")
bounds = source.getchannel("A").getbbox()
if bounds is None:
    raise ValueError("Logo is fully transparent")
mark = source.crop(bounds)
mark.thumbnail((448, 448), Image.Resampling.LANCZOS)
canvas = Image.new("RGBA", (512, 512))
canvas.alpha_composite(mark, ((512 - mark.width) // 2, (512 - mark.height) // 2))
canvas.save(assets / "logo.png", optimize=True)
for name, size in (("foliochest.png", 256), ("favicon-32.png", 32), ("apple-touch-icon.png", 180)):
    canvas.resize((size, size), Image.Resampling.LANCZOS).save(assets / name, optimize=True)
canvas.save(assets / "foliochest.ico", sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])

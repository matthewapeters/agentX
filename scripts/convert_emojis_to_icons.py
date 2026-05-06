from PIL import Image, ImageDraw, ImageFont
import os
from agentx.config import get_icon_path


def convert_emojis_to_icons(emojis, output_dir, font_path, size=128):
    """
    Convert a list of emojis to PNG icon files.

    :param emojis: List of emojis to convert.
    :param output_dir: Directory to save the icon files.
    :param font_path: Path to the TTF font file.
    :param size: Size of the output icons (width and height).
    """
    if not os.path.exists(output_dir):
        os.makedirs(output_dir)

    font = ImageFont.truetype(
        font_path, size=164
    )  # Specify a fixed font size explicitly

    for emoji in emojis:
        # Create a blank image with a transparent background
        image = Image.new("RGBA", (size, size), (255, 255, 255, 0))
        draw = ImageDraw.Draw(image)

        # Calculate the position to center the emoji
        text_width, text_height = draw.textsize(emoji, font=font)
        x = (size - text_width) / 2
        y = (size - text_height) / 2

        # Draw the emoji
        draw.text((x, y), emoji, font=font, fill=(0, 0, 0, 255))

        # Save the image
        emoji_name = f"emoji_{ord(emoji):x}.png"
        output_path = os.path.join(output_dir, emoji_name)
        image.save(output_path)
        print(f"Saved: {output_path}")


def convert_svg_to_png(svg_name, output_dir, size=128):
    """
    Convert an SVG icon to a PNG file.

    :param svg_name: Name of the SVG file to convert.
    :param output_dir: Directory to save the PNG file.
    :param size: Size of the output PNG (width and height).
    """
    import cairosvg

    if not os.path.exists(output_dir):
        os.makedirs(output_dir)

    svg_path = get_icon_path(svg_name)
    png_path = os.path.join(output_dir, f"{os.path.splitext(svg_name)[0]}.png")

    cairosvg.svg2png(
        url=svg_path, write_to=png_path, output_width=size, output_height=size
    )
    print(f"Converted {svg_name} to {png_path}")


if __name__ == "__main__":
    # Example usage for emojis
    emojis = ["😀", "😂", "😍", "👍", "🔥"]
    output_dir = "assets/icons"
    font_path = "/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf"

    convert_emojis_to_icons(emojis, output_dir, font_path)

    # Example usage for SVG icons
    svg_icons = ["1F600.svg", "1F602.svg", "1F60D.svg", "1F44D.svg", "1F525.svg"]
    output_dir_svg = "assets/icons/png"

    for svg_icon in svg_icons:
        convert_svg_to_png(svg_icon, output_dir_svg)

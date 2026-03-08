import tkinter as tk
import os


def test_emoji_rendering():
    root = tk.Tk()
    root.title("Emoji Rendering Test")

    # Locate the font file
    package_dir = os.path.dirname(__file__)
    emoji_font_path = os.path.join(package_dir, "src/agentx/fonts/NotoColorEmoji.ttf")

    if os.path.exists(emoji_font_path):
        print(f"Using font: {emoji_font_path}")
        text_font = (emoji_font_path, 20)
    else:
        print("Font file not found. Falling back to default font.")
        text_font = ("Terminal", 20)

    # Create a Text widget to display the emoji
    text_widget = tk.Text(root, font=text_font, wrap=tk.WORD, height=5, width=30)
    text_widget.insert(tk.END, "Testing 🏠 emoji rendering\n")
    text_widget.pack()

    root.mainloop()


if __name__ == "__main__":
    test_emoji_rendering()

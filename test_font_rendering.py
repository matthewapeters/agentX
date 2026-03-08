import tkinter as tk
from tkinter import font


def test_font_rendering():
    root = tk.Tk()
    root.title("Font Rendering Test")

    # Use the font family name
    custom_font = font.Font(family="Noto Color Emoji", size=20)

    # Create a label to display an emoji
    label = tk.Label(root, text="😀 Emoji Test", font=custom_font)
    label.pack()

    # Print debug information
    print("Using font family: Noto Color Emoji")

    # Run the Tkinter main loop
    root.mainloop()


if __name__ == "__main__":
    test_font_rendering()

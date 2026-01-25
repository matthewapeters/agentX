"""
Docstring for layout
"""

import tkinter as tk
from tkinter import font

# import emoji


def layout(root):
    """
    Sets up the layout for the tkinter root window.
    """
    text_font = ("Terminal", 10)
    enter_emoji_unicode = "⏎"
    print(enter_emoji_unicode)
    root.title("AgentX Application")
    root.geometry("800x600")
    root.output_display = tk.Frame(root, bg="white")
    root.output_text = tk.Text(root.output_display, wrap=tk.WORD, font=text_font)
    root.output_text.pack(expand=True, fill=tk.BOTH)
    root.system_status = tk.Frame(root, bg="lightblue")
    root.system_status_text = tk.Text(root.system_status, wrap=tk.WORD, font=text_font)
    root.system_status_text.pack(expand=True, fill=tk.BOTH)
    root.user_input = tk.Frame(root, bg="lightgrey")
    root.user_input_text = tk.Text(root.user_input, wrap=tk.WORD, font=text_font)
    root.user_input_text.place(relx=0, rely=0, relwidth=0.94, relheight=1.0)
    root.user_submit = tk.Button(root.user_input, text=enter_emoji_unicode)
    root.user_submit.place(relx=0.94, rely=0, relwidth=0.06, relheight=0.25)
    root.output_display.place(relx=0.001, rely=0.001, relwidth=0.79, relheight=0.79)
    root.system_status.place(relx=0.80, rely=0.001, relwidth=0.2, relheight=0.79)
    root.user_input.place(relx=0.001, rely=0.80, relwidth=1.0, relheight=0.2)

from logging import root
import tkinter as tk
from .layout import layout


def main():
    root = tk.Tk()
    layout(root)
    root.mainloop()

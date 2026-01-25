from logging import root
import tkinter as tk
from .layout import layout
from .config import load_config
from .ollama_client import perform_service_handshake

def main():
    root = tk.Tk()
    config = load_config()

    # Perform service handshake before initializing the layout
    try:
        perform_service_handshake(config)
    except RuntimeError as e:
        print(e)
        return

    layout(root, config=config)
    root.mainloop()

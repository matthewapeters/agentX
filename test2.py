import tkinter as tk

root = tk.Tk()
try:
    # Query Tcl for its font system configuration
    font_system = root.tk.call('::tk::pkgconfig', 'get', 'fontsystem')
    print(f"Font System in use: {font_system}")
except Exception as e:
    print(f"Could not determine font system: {e}")
root.destroy()

"""
Docstring for layout
"""

import tkinter as tk

from .ollama_client import interrupt_streaming, stream_ollama_response


def layout(root, config):
    """
    Sets up the layout for the tkinter root window.
    """
    text_font = ("Terminal", 10)
    enter_emoji_unicode = "^⏎"

    # Get screen dimensions
    screen_width = root.winfo_screenwidth()
    screen_height = root.winfo_screenheight()

    # Determine screen side from config (default to "right")
    screen_side = config["agentx"].get("screen_side", "right").lower()
    if screen_side == "left":
        root.geometry(f"{screen_width // 2}x{screen_height}+0+0")
    else:  # Default to "right"
        root.geometry(f"{screen_width // 2}x{screen_height}+{screen_width // 2}+0")

    root.title("AgentX - the Ollama Agent")

    # Output display with scrollbar
    root.output_display = tk.Frame(root, bg="white")
    root.output_scrollbar = tk.Scrollbar(root.output_display)
    root.output_text = tk.Text(
        root.output_display,
        wrap=tk.WORD,
        font=text_font,
        yscrollcommand=root.output_scrollbar.set,
    )
    root.output_scrollbar.config(command=root.output_text.yview)
    root.output_text.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)
    root.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

    root.system_status = tk.Frame(root, bg="lightblue")
    root.system_status_text = tk.Text(root.system_status, wrap=tk.WORD, font=text_font)
    root.system_status_text.pack(expand=True, fill=tk.BOTH)

    # User input with scrollbar
    root.user_input = tk.Frame(root, bg="lightgrey")
    root.input_scrollbar = tk.Scrollbar(root.user_input)
    root.user_input_text = tk.Text(
        root.user_input,
        wrap=tk.WORD,
        font=text_font,
        yscrollcommand=root.input_scrollbar.set,
    )
    root.input_scrollbar.config(command=root.user_input_text.yview)
    root.user_input_text.place(
        relx=0, rely=0, relwidth=0.90, relheight=1.0
    )  # Adjusted width to make space for scrollbar
    root.input_scrollbar.place(
        relx=0.90, rely=0, relheight=1.0
    )  # Positioned at the right edge of user_input_text

    root.user_submit = tk.Button(
        root.user_input,
        text=enter_emoji_unicode,
        command=lambda: stream_ollama_response(root, config),
    )
    root.user_submit.place(relx=0.92, rely=0, relwidth=0.07, relheight=0.25)

    # Add a break button below the submit button
    root.user_break = tk.Button(
        root.user_input,
        text="❌",
        command=lambda: interrupt_streaming(),
        state=tk.DISABLED,
    )
    root.user_break.place(relx=0.92, rely=0.26, relwidth=0.07, relheight=0.25)

    root.output_display.place(relx=0.001, rely=0.001, relwidth=0.79, relheight=0.79)
    root.system_status.place(relx=0.80, rely=0.001, relwidth=0.2, relheight=0.79)
    root.user_input.place(relx=0.001, rely=0.80, relwidth=1.0, relheight=0.2)

    # Bind Ctrl-Enter to trigger the user_submit button
    root.user_input_text.bind(
        "<Control-Return>", lambda event: root.user_submit.invoke()
    )

    # Bind Ctrl-Space globally to trigger the user_break button
    root.bind_all("<Control-space>", lambda event: root.user_break.invoke())

    # Setup the Ollama client with the loaded configuration
    # Adds text styling tags to the output_text widget.
    user_prompt_bg = config["agentx"].get("user_prompt_font_background", "lightblue")
    agent_response_bg = config["agentx"].get("agent_response_font_background", "white")
    agent_thinking_bg = config["agentx"].get(
        "agent_thinking_font_background", "lightgray"
    )
    system_bg = config["agentx"].get("system_font_background", "#ffffff")

    root.output_text.tag_config(
        "gray", foreground="gray", font=("Terminal", 10, "italic")
    )
    root.output_text.tag_config(
        "user_prompt", background=user_prompt_bg, font=("Terminal", 10, "bold")
    )
    root.output_text.tag_config(
        "agent_response", background=agent_response_bg, font=("Terminal", 10, "normal")
    )
    root.output_text.tag_config(
        "agent_thinking", background=agent_thinking_bg, font=("Terminal", 10, "italic")
    )
    root.output_text.tag_config(
        "system_space", background=system_bg, font=("Terminal", 10, "normal")
    )

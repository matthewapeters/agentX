import asyncio
import tkinter as tk

import httpx
from ollama import Client


def stream_ollama_response(root, config):
    """
    Streams the response from the Ollama server and updates the output_text widget.
    """
    # Load configuration
    ollama_host = config["agentx"]["ollama_host"]
    ollama_model = config["agentx"]["ollama_model"]

    # Get the prompt from the user_input_text widget
    prompt = root.user_input_text.get("1.0", tk.END).strip()
    if not prompt:
        root.output_text.insert(tk.END, "No input provided.\n")
        return

    # Display the user prompt in the output_text widget
    root.user_input_text.delete("1.0", tk.END)  # Clear the user input text
    root.output_text.insert(tk.END, prompt + "\n", ("user_prompt",))
    root.output_text.insert(tk.END, "\n", ("system_space",))  # Add extra whitespace with system background
    root.output_text.see(tk.END)  # Auto-scroll to the end

    # Define the message payload
    message = {"role": "user", "content": prompt}

    try:
        # Use the AsyncClient to stream responses
        last_channel = ""
        client = Client(host=f"http://{ollama_host}")
        for part in client.chat(
            model=ollama_model, 
            messages=[message], 
            stream=True): 
            #print(f"Received part: {part}")  # Debugging for received part
            channels = [k for k,v in part.message.__dict__.items() if v and k not in ["role", ""]]
            if channels:
                channel = channels[0]
                # print(f"Received channel: {channel}: {part.message[channel]}")  # Debugging for received channel
                if last_channel and channel != last_channel:
                    root.output_text.insert(tk.END, "\n\n", ("system_space",))  # Add spacing between different channels
                match channel:
                    case "thinking":
                        # Handle agent thinking content
                        root.output_text.insert(tk.END, part.message.thinking, ("agent_thinking",))
                        root.output_text.see(tk.END)  # Auto-scroll to the end
                        last_channel = channel
                    case "content":
                        # Handle agent response content
                        root.output_text.insert(tk.END, part.message.content, ("agent_response",))
                        root.output_text.see(tk.END)  # Auto-scroll to the end
                        last_channel = channel
                    case "tool_name":
                        # Handle tool_name (currently pass)
                        print(f"Tool name received: {part.message.tool_name}")  # Debugging for tool_name
                        last_channel = channel
                    case "tool_calls":
                        # Handle tool_calls (currently pass)
                        print(f"Tool calls received: {part.message.tool_calls}")  # Debugging for tool_calls
                        last_channel = channel
                    case "images":
                        # Handle images (currently pass)
                        print(f"Images received: {part.message.images}")  # Debugging for images
                        last_channel = channel
                    case _:
                        print(f"Unknown channel received: {channel}")  # Debugging for unknown channels
                        last_channel = channel
    except Exception as e:
        root.output_text.insert(tk.END, f"Error: {e}\n")
        print(f"Request error: {e}")


def setup_ollama_client(root, config):
    """
    Sets up the button to trigger the Ollama client and adds text styling.
    """
    add_text_styling(root, config)  # Ensure text styling tags are added
    root.user_submit.config(
        command=lambda: stream_ollama_response(root, config)
    )


def perform_service_handshake(config):
    """
    Performs a handshake with the Ollama server and ensures the model is loaded.
    """
    ollama_host = config["agentx"]["ollama_host"]
    ollama_model = config["agentx"]["ollama_model"]
    timeout_seconds = config["agentx"].get("ollama_initial_load_timeout_seconds", 120)

    url = f"http://{ollama_host}/api/chat"
    headers = {"Content-Type": "application/json"}
    payload = {
        "model": ollama_model,
        "prompt": "",
    }  # Empty prompt to trigger model load

    try:
        with httpx.Client(timeout=timeout_seconds) as client:
            response = client.post(url, json=payload, headers=headers)
            response.raise_for_status()
            print("Service handshake and model invocation successful.")
    except httpx.RequestError as e:
        raise RuntimeError(
            f"Failed to perform service handshake and model invocation: {e}"
        )


def add_text_styling(root, config):
    """
    Adds text styling tags to the output_text widget.
    """
    user_prompt_bg = config["agentx"].get("user_prompt_font_background", "lightblue")
    agent_response_bg = config["agentx"].get("agent_response_font_background", "white")
    agent_thinking_bg = config["agentx"].get("agent_thinking_font_background", "lightgray")
    system_bg = config["agentx"].get("system_font_background", "#ffffff")

    root.output_text.tag_config("gray", foreground="gray", font=("Terminal", 10, "italic"))
    root.output_text.tag_config("user_prompt", background=user_prompt_bg, font=("Terminal", 10, "bold"))
    root.output_text.tag_config("agent_response", background=agent_response_bg, font=("Terminal", 10, "normal"))
    root.output_text.tag_config("agent_thinking", background=agent_thinking_bg, font=("Terminal", 10, "italic"))
    root.output_text.tag_config("system_space", background=system_bg, font=("Terminal", 10, "normal"))

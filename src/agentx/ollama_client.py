import tkinter as tk
import httpx
import asyncio
import json

async def stream_ollama_response(root, config):
    """
    Streams the response from the Ollama server and updates the output_text widget.
    """
    # Load configuration
    ollama_host = config['agentx']['ollama_host']
    ollama_model = config['agentx']['ollama_model']

    # Get the prompt from the user_input_text widget
    prompt = root.user_input_text.get("1.0", tk.END).strip()
    if not prompt:
        root.output_text.insert(tk.END, "No input provided.\n")
        return

    # Define the streaming endpoint
    url = f"http://{ollama_host}/api/chat"
    headers = {"Content-Type": "application/json"}
    payload = {"model": ollama_model, "prompt": prompt}

    try:
        # Make a streaming request to the Ollama server
        async with httpx.AsyncClient() as client:
            async with client.stream("POST", url, json=payload, headers=headers) as response:
                response.raise_for_status()
                root.output_text.insert(tk.END, "Thinking...\n", ("gray",))
                async for line in response.aiter_lines():
                    if line:
                        try:
                            data = json.loads(line)  # Corrected to use the standard `json` module
                            content = data.get("content", "")
                            root.output_text.insert(tk.END, content, ("bold",))
                            root.output_text.insert(tk.END, "\n")
                            root.output_text.see(tk.END)  # Auto-scroll to the end
                        except ValueError:
                            continue
    except httpx.RequestError as e:
        root.output_text.insert(tk.END, f"Error: {e}\n")

def setup_ollama_client(root, config):
    """
    Sets up the button to trigger the Ollama client and adds text styling.
    """
    add_text_styling(root)  # Ensure text styling tags are added
    root.user_submit.config(command=lambda: asyncio.run(stream_ollama_response(root, config)))

def perform_service_handshake(config):
    """
    Performs a handshake with the Ollama server and ensures the model is loaded.
    """
    ollama_host = config['agentx']['ollama_host']
    ollama_model = config['agentx']['ollama_model']
    timeout_seconds = config['agentx'].get('ollama_initial_load_timeout_seconds', 120)

    url = f"http://{ollama_host}/api/chat"
    headers = {"Content-Type": "application/json"}
    payload = {"model": ollama_model, "prompt": ""}  # Empty prompt to trigger model load

    try:
        with httpx.Client(timeout=timeout_seconds) as client:
            response = client.post(url, json=payload, headers=headers)
            response.raise_for_status()
            print("Service handshake and model invocation successful.")
    except httpx.RequestError as e:
        raise RuntimeError(f"Failed to perform service handshake and model invocation: {e}")

def add_text_styling(root):
    """
    Adds text styling tags to the output_text widget.
    """
    root.output_text.tag_config("gray", foreground="gray", font=("Terminal", 10, "italic"))
    root.output_text.tag_config("bold", foreground="black", font=("Terminal", 10, "bold"))
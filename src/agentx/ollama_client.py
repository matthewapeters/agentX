import threading
import tkinter as tk

import httpx
from ollama import Client

from .session import AgentXSession
from .message import Message
# Global variable to manage streaming state
is_streaming = threading.Event()
streaming_thread = None


def _stream_ollama_response_worker(session: AgentXSession ):
    """
    Worker function that streams the response from the Ollama server and updates the output_text widget.
    This runs in a separate thread to keep the GUI responsive.
    """
    root = session.root
    config = session.config
    global is_streaming  # Ensure we use the global is_streaming instance
    is_streaming.set()
    root.user_break.config(state=tk.NORMAL)  # Enable the break button
    root.update_idletasks()

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
    root.output_text.insert(tk.END, f"User: {prompt}\n", ("user_prompt",))
    root.output_text.see(tk.END)  # Auto-scroll to the end
    root.update_idletasks()

    try:
        # Define the message payload
        message = Message(role="user", content=prompt)
        agent_thinking_message = message(role="assistant", content="")
        agent_thinking_message.enabled = False
        agent_response_message = message(role="assistant", content="")
        session.add_message_to_context(message)
        # Use the AsyncClient to stream responses
        last_channel = ""
        client = Client(host=f"http://{ollama_host}")
        for part in client.chat(model=ollama_model, messages=[message], stream=True):
            if not is_streaming.is_set():
                break  # Exit the loop if streaming is interrupted
            # print(f"Received part: {part}")  # Debugging for received part
            channels = [
                k
                for k, v in part.message.__dict__.items()
                if v and k not in ["role", ""]
            ]
            if channels:
                channel = channels[0]
                # print(f"Received channel: {channel}: {part.message[channel]}")  # Debugging for received channel
                if channel != last_channel:
                    match channel:
                        case "thinking":
                            root.output_text.insert(
                                tk.END, "\n", ("system_space",)
                            )  # Add spacing between different channels
                            root.output_text.insert(
                                tk.END,
                                "(Agent is thinking...)\n\n",
                                ("agent_thinking",),
                            )
                            root.output_text.see(tk.END)  # Auto-scroll to the end
                        case "content":
                            session.add_message_to_context(agent_thinking_message)
                            root.output_text.insert(
                                tk.END, "\n", ("agent_thinking",)
                            )  # end of line for thinking
                            root.output_text.insert(
                                tk.END, "\n", ("system_space",)
                            )  # Add spacing between different channels
                            root.output_text.insert(
                                tk.END, "Agent:\n\n", ("agent_response",)
                            )
                            root.update_idletasks()
                        case _:
                            pass  # For other channels, no special header
                match channel:
                    case "thinking":
                        # Handle agent thinking content
                        root.output_text.insert(
                            tk.END, part.message.thinking, ("agent_thinking",)
                        )
                        agent_thinking_message.content += part.message.thinking
                        root.output_text.see(tk.END)  # Auto-scroll to the end
                        last_channel = channel
                    case "content":
                        # Handle agent response content
                        root.output_text.insert(
                            tk.END, part.message.content, ("agent_response",)
                        )
                        agent_response_message.content += part.message.content
                        root.output_text.see(tk.END)  # Auto-scroll to the end
                        last_channel = channel
                    case "tool_name":
                        # Handle tool_name (currently pass)
                        print(
                            f"Tool name received: {part.message.tool_name}"
                        )  # Debugging for tool_name
                        last_channel = channel
                    case "tool_calls":
                        # Handle tool_calls (currently pass)
                        print(
                            f"Tool calls received: {part.message.tool_calls}"
                        )  # Debugging for tool_calls
                        last_channel = channel
                    case "images":
                        # Handle images (currently pass)
                        print(
                            f"Images received: {part.message.images}"
                        )  # Debugging for images
                        last_channel = channel
                    case _:
                        print(
                            f"Unknown channel received: {channel}"
                        )  # Debugging for unknown channels
                        last_channel = channel
                root.update_idletasks()
        # After streaming is complete, add spacing
        root.output_text.insert(
            tk.END, "\n\n", ("system_space",)
        )  # Add spacing between different channels
        session.add_message_to_context(agent_response_message)
        root.update_idletasks()

    except Exception as e:
        root.output_text.insert(tk.END, f"Error: {e}\n")
        print(f"Request error: {e}")
    finally:
        is_streaming.clear()
        root.user_break.config(state=tk.DISABLED)  # Disable the break button


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


def interrupt_streaming():
    """
    Interrupts the ongoing streaming process.
    """
    global is_streaming
    print("Interrupting streaming...")
    is_streaming.clear()


def stream_ollama_response(root, config):
    """
    Initiates streaming response in a separate thread to keep the GUI responsive.
    """
    global streaming_thread
    if streaming_thread and streaming_thread.is_alive():
        print("Streaming already in progress")
        return
    streaming_thread = threading.Thread(
        target=_stream_ollama_response_worker, args=(root, config), daemon=False
    )
    streaming_thread.start()

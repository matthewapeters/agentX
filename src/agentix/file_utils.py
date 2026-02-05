# File I/O utilities for Agentix CLI

import sys


def load_file(file_path: str) -> str:
    """Load the raw contents of a file."""
    with open(file_path, "r", encoding="utf-8") as file:
        return file.read()


def get_file(file_path: str) -> str:
    """Load a file and return it with formatted output wrapper."""
    try:
        content = load_file(file_path)
        return f"[FILE: {file_path}]\n{content}\n[END OF FILE]\n\n"
    except Exception as e:
        print(f"Error loading file {file_path}: {e}", file=sys.stderr)
        return ""


def get_attachments(args) -> list:
    """Load multiple file attachments from file paths provided in args."""
    attachments = []
    for attachment_path in args.file_path or []:
        try:
            attachments.append(get_file(attachment_path))
        except Exception as e:
            print(f"Error loading attachment {attachment_path}: {e}", file=sys.stderr)
    return attachments


def replace_file_content(args, attachment: dict) -> None:
    """replace the content of a file with the provided attachment data."""
    encoding = attachment.get("encoding", "utf-8")
    language = attachment.get("language", "text")
    name = attachment.get("name", args.file_path[0] if args.file_path else "")
    data = attachment.get("data", "")
    print(f"Replacing content of {language} file: {name}", file=sys.stderr)
    with open(name, "w", encoding=encoding) as f:
        f.write(data.decode(encoding))

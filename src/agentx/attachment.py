"""
Docstring for agentx.attachment
"""
from dataclasses import dataclass
@dataclass
class Attachment:
    """
    Docstring for Attachment
    """
    file_path: str
    content_type: str  
    enabled: bool = True
    content: str


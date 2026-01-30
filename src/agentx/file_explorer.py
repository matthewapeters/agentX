"""
Docstring for agentx.file_explorer
"""

import tkinter as tk
import os
from pathlib import Path
from glob import glob


# TODO: Implement the file explorer functionality here in the style of context.py and history.py
# the objective is to allow the user to explore files and directories within the agentx application,
# providing a user-friendly interface for navigating the file system, and then selecting files to be used as input for the agent
# in the form of user message attchments.


class FileExplorer:
    """
    Docstring for FileExplorer
    """

    def __init__(self, start_path: str  = str(Path.home())):
        """
        Docstring for __init__

        :param start_path: The initial path to start exploring from.
        """
        self.current_path = start_path


    def list_directory(self) -> list[str]:
        """
        List the contents of a directory.

        :param path: The directory path to list.
        :return: A list of file and directory names in the specified path.
        """
        dir = glob(os.path.join(self.current_path , '*'))
        pass  # TODO: Implement directory listing logic

    def change_directory(self, new_path: str):
        """
        Change the current directory.

        :param new_path: The new directory path to change to.
        """
        pass  # TODO: Implement change directory logic

    def get_current_path(self) -> str:
        """
        Get the current directory path.

        :return: The current directory path.
        """
        return self.current_path    
    def open_file(self, file_path: str) -> str:
        """
        Open and read the contents of a file.
        :param file_path: The path of the file to open.
        :return: The contents of the file as a string.
        """
        pass  # TODO: Implement file opening and reading logic 

    def to_gui(self, parent_frame: tk.Frame) -> tk.Frame:
        """
        Docstring for to_gui

        Create a GUI frame for the file explorer.

        :param parent_frame: The parent Tkinter frame to attach the file explorer GUI to.
        :return: A Tkinter frame containing the file explorer GUI.
        """
        frame = tk.Frame(parent_frame)
        # TODO: Implement GUI representation of the file explorer 
        return frame



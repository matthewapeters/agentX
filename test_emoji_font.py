import unittest
import os
import tkinter as tk
from src.agentx.session import AgentXSession
from src.agentx.file_explorer import FileExplorer


class TestEmojiFontLoading(unittest.TestCase):

    def setUp(self):
        # Set up a Tkinter root for testing
        self.root = tk.Tk()
        self.config = {"agentx": {"screen_side": "right"}}
        self.session = AgentXSession(self.root, self.config)
        self.file_explorer = FileExplorer()

    def tearDown(self):
        # Destroy the Tkinter root after each test
        self.root.destroy()

    def test_emoji_font_in_session(self):
        """Test if the emoji font is loaded in the session layout."""
        emoji_font_path = os.path.join(os.getcwd(), "fonts", "NotoColorEmoji.ttf")
        self.session.layout()

        # Check if the font is applied to the root title or other widgets
        if os.path.exists(emoji_font_path):
            self.assertIn("NotoColorEmoji", str(self.session.root.title()))
        else:
            self.assertNotIn("NotoColorEmoji", str(self.session.root.title()))

    def test_emoji_font_in_file_explorer(self):
        """Test if the emoji font is loaded in the file explorer labels."""
        emoji_font_path = os.path.join(os.getcwd(), "fonts", "NotoColorEmoji.ttf")
        explorer_frame = self.file_explorer.to_gui(self.root)

        # Check if the font is applied to the path label
        path_label = self.file_explorer._path_label
        if os.path.exists(emoji_font_path):
            self.assertIn("NotoColorEmoji", str(path_label["font"]))
        else:
            self.assertNotIn("NotoColorEmoji", str(path_label["font"]))


if __name__ == "__main__":
    unittest.main()

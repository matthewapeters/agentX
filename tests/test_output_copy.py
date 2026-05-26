import types

from agentx.gui.chat_panel import ChatPanel


class DummyText:
    def __init__(self):
        self.tags = set()
        self.selection = None
        self.events = []

    def tag_add(self, tag, start, end):
        self.tags.add(tag)

    def mark_set(self, mark, position):
        self.selection = position

    def tag_names(self):
        return list(self.tags)

    def event_generate(self, event):
        self.events.append(event)


class DummyWidgets:
    def __init__(self):
        self.output_text = DummyText()


class DummyConfig:
    def __init__(self):
        self.output_bg = "#ffffff"
        self.default_font = ("Courier New", 10)
        self.agent_response_fg = "#000000"
        self.markdown_render_enabled = False


class DummyGUIManager:
    def __init__(self):
        self.config = DummyConfig()
        self.widgets = DummyWidgets()
        self._text_font = ("Courier New", 10)
        self.COLOR_AGENT_RESPONSE = "#000000"


def test_copy_event_called():
    gui = DummyGUIManager()
    panel = ChatPanel(gui)
    # Ensure output_text exists
    assert panel._widgets.output_text is not None
    # Call copy
    result = panel._copy_output_text_selection()
    assert result == "break"
    assert "<<Copy>>" in panel._widgets.output_text.events


def test_select_all_and_copy():
    gui = DummyGUIManager()
    panel = ChatPanel(gui)
    panel._select_all_output_text()
    # Verify selection tag added
    assert "sel" in panel._widgets.output_text.tags
    # Now copy
    panel._copy_output_text_selection()
    assert "<<Copy>>" in panel._widgets.output_text.events

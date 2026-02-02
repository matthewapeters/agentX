# GUIManager Architecture Design

## Executive Summary

This document defines the architectural design for refactoring `AgentXSession` to separate GUI concerns from business logic by introducing a `GUIManager` class. The goal is to achieve proper encapsulation, improve testability, and enable independent evolution of presentation and business layers.

## Problem Statement

### Current Issues

The `AgentXSession` class currently violates the Single Responsibility Principle by mixing:
- **Business Logic**: Session state, message management, LLM communication, history tracking
- **GUI Management**: Widget creation, layout, styling, event binding
- **Presentation Logic**: Rendering messages, updating displays, managing widget state

This tight coupling results in:
- Inability to test business logic without tkinter runtime
- Difficulty maintaining and modifying UI independently
- Widget state scattered across `self.root` with `hasattr()` checks
- Complex methods that mix GUI updates with business operations
- No clear API boundary between layers

## Architectural Goals

### Primary Objectives

1. **Separation of Concerns**: Clear boundary between business logic and presentation
2. **Testability**: Business logic testable without GUI runtime
3. **Maintainability**: UI changes isolated from business logic changes
4. **Flexibility**: Support multiple UI implementations or themes
5. **Type Safety**: Strong typing for interfaces and data exchange
6. **Single Source of Truth**: Centralized widget management

### Design Principles

- **Dependency Inversion**: Session depends on GUI abstraction, not concrete implementation
- **Encapsulation**: Widget lifecycle and state managed entirely by GUI layer
- **Loose Coupling**: Communication through well-defined interfaces with primitive types
- **Command-Query Separation**: Clear distinction between state-modifying and query operations

## Architecture Overview

### Component Structure

```
┌─────────────────────────────────────────────────────────────┐
│                      AgentXSession                          │
│  (Business Logic / Controller)                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ - Context management                                 │   │
│  │ - History tracking                                   │   │
│  │ - Message lifecycle                                  │   │
│  │ - LLM communication                                  │   │
│  │ - File attachment logic                              │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                          │
                          │ Uses IGUIManager interface
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                     GUIManager                              │
│  (Presentation Layer)                                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ - Widget creation and lifecycle                      │   │
│  │ - Layout management                                  │   │
│  │ - Event binding and handling                         │   │
│  │ - Display updates and rendering                      │   │
│  │ - Style and theme management                         │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                          │
                          │ Manages
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                  WidgetRegistry                             │
│  (Widget Storage and Lifecycle)                             │
│  - Centralized widget references                            │
│  - Type-safe widget access                                  │
│  - Cleanup and disposal                                     │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

#### User Input Flow
```
User Action (GUI) → GUIManager → Callback → AgentXSession
                                           → Business Logic
                                           → GUIManager.display_*()
                                           → GUI Update
```

#### State Update Flow
```
Business Logic → State Change → GUIManager.update_*()
                              → Widget Update
                              → Display Refresh
```

## Core Components

### 1. IGUIManager Interface

Defines the contract between business logic and presentation layer.

```python
class IGUIManager(Protocol):
    """Interface for GUI implementations."""
    
    # Lifecycle Methods
    def create_layout(self) -> None:
        """Creates and arranges all GUI widgets."""
        
    def destroy(self) -> None:
        """Cleanup and destroy all widgets."""
    
    # Display Methods - Output
    def display_user_message(
        self,
        content: str,
        attachments: list[str],
        timestamp: datetime
    ) -> None:
        """Display a user message in the output area."""
        
    def display_agent_thinking(self, content: str) -> None:
        """Display agent thinking content (streaming)."""
        
    def display_agent_response(self, content: str) -> None:
        """Display agent response content (streaming)."""
        
    def display_error(self, message: str) -> None:
        """Display an error message to the user."""
    
    # Display Methods - Attachments
    def update_attachment_bar(
        self,
        current_attachments: list[AttachmentInfo],
        history_attachments: list[AttachmentInfo]
    ) -> None:
        """Update the attachment display above input box."""
        
    # Display Methods - Context/History
    def update_context_panel(self, context_widget: tk.Widget) -> None:
        """Replace the context panel with new rendered content."""
        
    def update_history_panel(self, history_widget: tk.Widget) -> None:
        """Replace the history panel with new rendered content."""
        
    def update_files_panel(self, files_widget: tk.Widget) -> None:
        """Replace the files panel with new rendered content."""
    
    # Input Methods
    def get_user_input(self) -> str:
        """Extract and clear user input text."""
        
    def clear_user_input(self) -> None:
        """Clear the user input field."""
    
    # State Management
    def set_streaming_state(self, is_streaming: bool) -> None:
        """Update UI for streaming (disable submit, enable break)."""
        
    def set_busy_state(self, is_busy: bool) -> None:
        """Update UI for busy operations."""
    
    # Widget Access (minimal, for integration)
    def get_root(self) -> tk.Tk:
        """Get the root window (for dialogs, etc)."""
        
    def get_context_parent(self) -> tk.Widget:
        """Get parent widget for context rendering."""
        
    def get_history_parent(self) -> tk.Widget:
        """Get parent widget for history rendering."""
        
    def get_files_parent(self) -> tk.Widget:
        """Get parent widget for file explorer rendering."""
```

### 2. GUIConfig Dataclass

Type-safe configuration for GUI layer.

```python
@dataclass
class GUIConfig:
    """Configuration for GUI appearance and behavior."""
    
    # Window Configuration
    screen_side: str = "right"  # "left" or "right"
    window_width_ratio: float = 0.5
    window_height_ratio: float = 1.0
    
    # Layout Configuration
    output_panel_ratio: float = 0.66  # Sash position (2:1 split)
    attachment_bar_height: float = 0.03
    input_panel_height: float = 0.2
    
    # Font Configuration
    default_font: tuple[str, int] = ("Terminal", 10)
    emoji_font_path: str | None = None
    
    # Style Configuration
    output_bg: str = "white"
    status_bg: str = "lightblue"
    input_bg: str = "lightgrey"
    attachment_bg: str = "white"
    history_attachment_bg: str = "#f0f0f0"
    
    # Text Style Configuration
    user_prompt_font: tuple[str, int, str] = ("Terminal", 10, "bold")
    agent_response_font: tuple[str, int, str] = ("Terminal", 10, "normal")
    agent_thinking_font: tuple[str, int, str] = ("Terminal", 10, "italic")
    gray_text_font: tuple[str, int, str] = ("Terminal", 10, "italic")
    
    @classmethod
    def from_dict(cls, config: dict) -> 'GUIConfig':
        """Create GUIConfig from application config dictionary."""
        agentx = config.get('agentx', {})
        return cls(
            screen_side=agentx.get('screen_side', 'right'),
            # Extract other values with defaults
        )
```

### 3. AttachmentInfo Dataclass

Data transfer object for attachment display information.

```python
@dataclass
class AttachmentInfo:
    """Information needed to display an attachment in GUI."""
    
    file_path: str
    display_name: str
    enabled: bool
    is_from_history: bool
    attachment_id: str  # Unique identifier for callbacks
    
    @classmethod
    def from_attachment(cls, attachment, is_from_history: bool = False) -> 'AttachmentInfo':
        """Create from Attachment object."""
        return cls(
            file_path=attachment.file_path,
            display_name=os.path.basename(attachment.file_path),
            enabled=attachment.enabled,
            is_from_history=is_from_history,
            attachment_id=str(id(attachment))
        )
```

### 4. WidgetRegistry Class

Centralized widget storage and lifecycle management.

```python
class WidgetRegistry:
    """Manages widget references and lifecycle."""
    
    def __init__(self):
        # Main structure widgets
        self.root: tk.Tk | None = None
        self.paned: tk.PanedWindow | None = None
        
        # Output panel widgets
        self.output_display: tk.Frame | None = None
        self.output_notebook: ttk.Notebook | None = None
        self.output_tab: tk.Frame | None = None
        self.output_text: tk.Text | None = None
        self.output_scrollbar: tk.Scrollbar | None = None
        
        # Status panel widgets
        self.system_status: tk.Frame | None = None
        self.system_notebook: ttk.Notebook | None = None
        self.session_tab: tk.Frame | None = None
        self.files_tab: tk.Frame | None = None
        
        # Dynamic content widgets (replaced during updates)
        self.system_status_history: tk.Widget | None = None
        self.system_status_context: tk.Widget | None = None
        self.system_status_files: tk.Widget | None = None
        
        # Input panel widgets
        self.attachments_frame: tk.Frame | None = None
        self.attachment_labels: list[tk.Widget] = []
        self.user_input: tk.Frame | None = None
        self.user_input_text: tk.Text | None = None
        self.input_scrollbar: tk.Scrollbar | None = None
        self.user_submit: tk.Button | None = None
        self.user_break: tk.Button | None = None
    
    def clear_attachments(self) -> None:
        """Destroy all attachment label widgets."""
        for widget in self.attachment_labels:
            widget.destroy()
        self.attachment_labels.clear()
    
    def destroy_all(self) -> None:
        """Destroy all managed widgets."""
        # Implementation destroys widgets in reverse creation order
```

### 5. GUIManager Class

Main implementation of the IGUIManager interface.

```python
class GUIManager:
    """Manages all GUI widgets and presentation logic."""
    
    def __init__(
        self,
        root: tk.Tk,
        config: GUIConfig,
        on_submit: Callable[[], None],
        on_interrupt: Callable[[], None],
        on_attachment_toggle: Callable[[str, bool], None]
    ):
        """
        Initialize GUI manager.
        
        Args:
            root: The tkinter root window
            config: GUI configuration
            on_submit: Callback when user submits input
            on_interrupt: Callback when user interrupts streaming
            on_attachment_toggle: Callback when attachment enabled state changes
                                  Args: (attachment_id, enabled)
        """
        self.root = root
        self.config = config
        self.widgets = WidgetRegistry()
        
        # Store callbacks
        self._on_submit = on_submit
        self._on_interrupt = on_interrupt
        self._on_attachment_toggle = on_attachment_toggle
        
        # Cache for text font
        self._text_font: tuple | None = None
    
    # IGUIManager interface implementation
    # ... (methods detailed in separate sections)
    
    # Private helper methods
    def _setup_fonts(self) -> tuple:
        """Determine and cache text font."""
        
    def _setup_window_geometry(self) -> None:
        """Configure window size and position."""
        
    def _create_output_panel(self) -> None:
        """Create output display widgets."""
        
    def _create_status_panel(self) -> None:
        """Create status panel with tabs."""
        
    def _create_input_panel(self) -> None:
        """Create user input area."""
        
    def _configure_text_styles(self) -> None:
        """Configure text widget tags for styling."""
        
    def _create_attachment_widget(
        self,
        parent: tk.Frame,
        info: AttachmentInfo
    ) -> tk.Widget:
        """Create a single attachment display widget."""
```

## Detailed API Specification

### Display Methods

#### display_user_message

```python
def display_user_message(
    self,
    content: str,
    attachments: list[str],
    timestamp: datetime
) -> None:
    """
    Display a user message in the output area.
    
    Args:
        content: The message text content
        attachments: List of attachment filenames (for display only)
        timestamp: When the message was created
        
    Behavior:
        - Appends formatted message to output_text widget
        - Uses 'user_prompt' text tag for styling
        - Lists attachments below message with 'gray' tag
        - Auto-scrolls to show new content
        - Does NOT modify business state
        
    Example:
        gui.display_user_message(
            "How do I implement this?",
            ["code.py", "docs.md"],
            datetime.now()
        )
    """
```

#### display_agent_thinking

```python
def display_agent_thinking(self, content: str) -> None:
    """
    Append agent thinking content to output (streaming).
    
    Args:
        content: Chunk of thinking text to append
        
    Behavior:
        - On first call (detected by checking previous content),
          inserts header: "(Agent is thinking...)\n\n"
        - Appends content with 'agent_thinking' tag
        - Auto-scrolls to show new content
        - Designed for streaming: called multiple times with chunks
        
    Note:
        This is a stateful operation - tracks whether header was added
        by checking last inserted content. Alternative: explicit 
        display_thinking_start() method.
    """
```

#### display_agent_response

```python
def display_agent_response(self, content: str) -> None:
    """
    Append agent response content to output (streaming).
    
    Args:
        content: Chunk of response text to append
        
    Behavior:
        - On first call, inserts header: "Agent:\n\n"
        - Appends content with 'agent_response' tag
        - Auto-scrolls to show new content
        - Designed for streaming: called multiple times
        
    Note:
        First call detection same as display_agent_thinking.
        Caller (AgentXSession) is responsible for tracking
        channel transitions (thinking → content).
    """
```

#### display_error

```python
def display_error(self, message: str) -> None:
    """
    Display an error message to the user.
    
    Args:
        message: Error message text
        
    Behavior:
        - Appends error to output_text with error styling
        - Could be enhanced with error dialog in future
        - Auto-scrolls to show error
    """
```

### Attachment Management

#### update_attachment_bar

```python
def update_attachment_bar(
    self,
    current_attachments: list[AttachmentInfo],
    history_attachments: list[AttachmentInfo]
) -> None:
    """
    Update the attachment display bar above input.
    
    Args:
        current_attachments: Attachments on current message
        history_attachments: Enabled attachments from history
        
    Behavior:
        - Destroys existing attachment widgets
        - Creates checkbox + label for each attachment
        - Current attachments: white background, 📁 icon
        - History attachments: gray background, 📜 icon, "(history)" suffix
        - Checkbox bound to on_attachment_toggle callback
        
    Design Notes:
        - Caller provides all attachment state
        - GUI doesn't maintain attachment state
        - Idempotent: can be called repeatedly with full state
        
    Callback Contract:
        on_attachment_toggle(attachment_id: str, enabled: bool)
        - Called when user checks/unchecks checkbox
        - attachment_id from AttachmentInfo
        - Session handles business logic update
        - Session calls update_attachment_bar again to refresh
    """
```

### Context and Panel Updates

#### update_context_panel

```python
def update_context_panel(self, context_widget: tk.Widget) -> None:
    """
    Replace context panel content with new widget.
    
    Args:
        context_widget: Fully rendered context widget from Context.to_gui()
        
    Behavior:
        - Destroys existing system_status_context widget if exists
        - Packs new widget into session_tab
        - Stores reference in widgets.system_status_context
        
    Integration:
        Context.to_gui() creates widget with expand=True, fill=BOTH
        This method handles the replacement lifecycle
        
    Design Rationale:
        Context rendering is complex (collapsible messages, attachments)
        Keep that logic in Context class where it belongs
        GUI just handles widget lifecycle
    """
```

#### update_history_panel

```python
def update_history_panel(self, history_widget: tk.Widget) -> None:
    """
    Replace history panel content with new widget.
    
    Args:
        history_widget: Fully rendered history widget from History.to_gui()
        
    Behavior:
        - Destroys existing system_status_history widget
        - Packs new widget into session_tab (collapsed by default)
        - Stores reference in widgets.system_status_history
        
    Integration:
        History.to_gui() creates widget with sessions from past
        Collapsed by default, user can expand
    """
```

#### update_files_panel

```python
def update_files_panel(self, files_widget: tk.Widget) -> None:
    """
    Replace files panel content with new widget.
    
    Args:
        files_widget: Fully rendered file explorer from FileExplorer.to_gui()
        
    Behavior:
        - Destroys existing system_status_files widget
        - Packs new widget into files_tab
        - Stores reference in widgets.system_status_files
    """
```

### Input Management

#### get_user_input

```python
def get_user_input(self) -> str:
    """
    Extract and clear user input text.
    
    Returns:
        Stripped input text
        
    Behavior:
        - Gets text from user_input_text widget (1.0 to END)
        - Strips whitespace
        - Clears the widget
        - Returns the text
        
    Design Note:
        Combines get and clear for atomic operation
        Prevents duplicate submissions
    """
```

#### clear_user_input

```python
def clear_user_input(self) -> None:
    """
    Clear the user input field.
    
    Behavior:
        - Deletes all text from user_input_text widget
        
    Use Case:
        Error conditions where input should be cleared
        without processing it
    """
```

### State Management

#### set_streaming_state

```python
def set_streaming_state(self, is_streaming: bool) -> None:
    """
    Update UI for streaming state.
    
    Args:
        is_streaming: True if streaming in progress, False if idle
        
    Behavior:
        If is_streaming:
            - Disable submit button
            - Enable break button (for interrupt)
        Else:
            - Enable submit button
            - Disable break button
            
    Design Note:
        GUI reflects state, doesn't own it
        Session manages streaming state, tells GUI to update
    """
```

#### set_busy_state

```python
def set_busy_state(self, is_busy: bool) -> None:
    """
    Update UI for busy operations (non-streaming).
    
    Args:
        is_busy: True if operation in progress
        
    Behavior:
        - Change cursor to wait/normal
        - Disable/enable input controls
        - Could show progress indicator in future
        
    Use Case:
        Loading history, initializing session, etc.
    """
```

### Widget Access Methods

#### get_root

```python
def get_root(self) -> tk.Tk:
    """
    Get the root window.
    
    Returns:
        The tkinter root window
        
    Use Case:
        - Creating dialogs (file pickers, etc.)
        - Global key bindings
        - Window-level operations
        
    Design Note:
        Minimize usage - most operations should go through
        GUIManager methods. Expose only when necessary.
    """
```

#### get_context_parent, get_history_parent, get_files_parent

```python
def get_context_parent(self) -> tk.Widget:
    """Get parent widget for context rendering."""
    
def get_history_parent(self) -> tk.Widget:
    """Get parent widget for history rendering."""
    
def get_files_parent(self) -> tk.Widget:
    """Get parent widget for file explorer rendering."""
```

**Purpose**: Allow Context, History, and FileExplorer to render themselves without knowing GUI structure.

**Usage Pattern**:
```python
# In AgentXSession
context_widget = self.context.to_gui(
    parent=self.gui.get_context_parent(),
    on_attachment_toggle=self._handle_attachment_toggle
)
self.gui.update_context_panel(context_widget)
```

## Callback Architecture

### Callback Registration

GUIManager receives callbacks during initialization:

```python
gui = GUIManager(
    root=root,
    config=gui_config,
    on_submit=self._handle_submit,
    on_interrupt=self._handle_interrupt,
    on_attachment_toggle=self._handle_attachment_toggle
)
```

### Callback Signatures

#### on_submit

```python
def _handle_submit(self) -> None:
    """
    Called when user submits input (click or Ctrl-Enter).
    
    Responsibilities:
        - Extract input via gui.get_user_input()
        - Validate input
        - Create Message object
        - Initiate streaming response
        - Update GUI state via gui methods
    """
```

#### on_interrupt

```python
def _handle_interrupt(self) -> None:
    """
    Called when user clicks break button (or Ctrl-Space).
    
    Responsibilities:
        - Set streaming flag to stop LLM communication
        - Update GUI state to idle
    """
```

#### on_attachment_toggle

```python
def _handle_attachment_toggle(self, attachment_id: str, enabled: bool) -> None:
    """
    Called when user toggles attachment checkbox.
    
    Args:
        attachment_id: Unique identifier of attachment
        enabled: New enabled state
        
    Responsibilities:
        - Find attachment object by ID
        - Update attachment.enabled
        - Refresh attachment display
    """
```

### Event Binding

GUIManager binds events to internal handlers that call callbacks:

```python
class GUIManager:
    def _create_input_panel(self):
        # Submit button
        self.widgets.user_submit = tk.Button(
            command=self._on_submit_clicked
        )
        
        # Keyboard binding
        self.widgets.user_input_text.bind(
            "<Control-Return>",
            lambda e: self._on_submit_clicked()
        )
    
    def _on_submit_clicked(self):
        """Internal handler - validates then calls callback."""
        if self._on_submit:
            self._on_submit()
```

## State Management Strategy

### State Ownership

**Business State** (owned by AgentXSession):
- Current message being composed
- Message history (context)
- Session history
- Enabled attachments
- Streaming status

**Display State** (owned by GUIManager):
- Widget references
- Scroll positions
- Expanded/collapsed panels
- Text buffer contents

### State Synchronization Pattern

**Push Model**: Session pushes updates to GUI

```python
# In AgentXSession
def add_message_to_context(self, message: Message):
    # Update business state
    self.context.add_message(ts=datetime.now(), message=message)
    
    # Push to GUI
    context_widget = self.context.to_gui(
        self.gui.get_context_parent(),
        on_attachment_toggle=self._handle_attachment_toggle
    )
    self.gui.update_context_panel(context_widget)
```

**Benefits**:
- Clear data flow
- Session controls when updates occur
- No polling or watching required

**Tradeoffs**:
- Session must remember to update GUI
- Risk of state divergence if update forgotten

### Refresh Pattern

For complex widgets (context, history, files), use full refresh:

```python
def refresh_context_gui(self):
    """Regenerate and display current context."""
    widget = self.context.to_gui(
        self.gui.get_context_parent(),
        on_attachment_toggle=self._handle_attachment_toggle
    )
    self.gui.update_context_panel(widget)
```

For simple state (attachments), pass full state:

```python
def refresh_attachment_bar(self):
    """Update attachment display with current state."""
    current = [AttachmentInfo.from_attachment(a) for a in self.message.attachments]
    history = [AttachmentInfo.from_attachment(a, True) for a in self.enabled_history_attachments]
    self.gui.update_attachment_bar(current, history)
```

## Integration Points

### AgentXSession Integration

```python
class AgentXSession:
    def __init__(self, root: tk.Tk, config: dict[str, Any]):
        # Create GUI config from app config
        gui_config = GUIConfig.from_dict(config)
        
        # Create GUI manager with callbacks
        self.gui = GUIManager(
            root=root,
            config=gui_config,
            on_submit=self._handle_submit,
            on_interrupt=self._handle_interrupt,
            on_attachment_toggle=self._handle_attachment_toggle
        )
        
        # Business logic initialization
        self.config = config
        self.context = Context()
        self.file_explorer = FileExplorer(start_path=os.getcwd())
        # ... rest of business logic setup
    
    def layout(self):
        """Initialize GUI layout."""
        self.gui.create_layout()
        
        # Initialize dynamic content
        self.refresh_context_gui()
        self.refresh_files_gui()
```

### Context/History Integration

```python
class Context:
    def to_gui(
        self,
        parent: tk.Widget,
        on_attachment_toggle: Callable[[Any, bool], None] = None
    ) -> tk.Widget:
        """
        Render context as a tkinter widget.
        
        Args:
            parent: Parent widget to render into
            on_attachment_toggle: Callback for attachment state changes
            
        Returns:
            Fully rendered widget ready to pack/grid
            
        Design:
            - Returns widget, doesn't pack it
            - Caller handles placement and lifecycle
            - Internal structure opaque to caller
        """
```

## Migration Considerations

### Backward Compatibility

During migration, some methods may need to maintain dual interfaces:

```python
class AgentXSession:
    @property
    def root(self) -> tk.Tk:
        """Backward compatibility - return root from gui."""
        return self.gui.get_root()
```

### Incremental Migration

The design supports incremental migration:

1. **Phase 1**: Create GUIManager with empty methods, delegate to existing code
2. **Phase 2**: Move widget creation to GUIManager, update session to use GUI methods
3. **Phase 3**: Move display logic to GUIManager
4. **Phase 4**: Remove widget references from session

### Testing Strategy

**With GUIManager**, testing becomes possible:

```python
class MockGUIManager:
    """Test double for GUIManager."""
    
    def __init__(self):
        self.displayed_messages = []
        self.user_input = ""
        self.streaming_state = False
    
    def display_user_message(self, content, attachments, timestamp):
        self.displayed_messages.append(("user", content))
    
    def get_user_input(self) -> str:
        result = self.user_input
        self.user_input = ""
        return result

# Test business logic without GUI
def test_message_handling():
    gui = MockGUIManager()
    session = AgentXSession(gui=gui, config=test_config)
    
    gui.user_input = "Hello"
    session._handle_submit()
    
    assert len(session.context.messages) == 1
    assert gui.displayed_messages[0] == ("user", "Hello")
```

## Design Rationale

### Why Not MVC/MVP?

**Considered**: Full Model-View-Controller pattern

**Decision**: Use simplified GUIManager pattern

**Rationale**:
- MVC adds circular dependencies (view ↔ controller)
- Application is relatively simple - doesn't need full framework
- One-way data flow (session → GUI) is sufficient
- Can evolve to MVC later if complexity increases

### Why Pass Widgets for Context/History?

**Alternative**: Have GUIManager call Context.to_gui() internally

**Decision**: Session generates widget, passes to GUI

**Rationale**:
- Context rendering requires business callbacks (attachment toggle)
- Session knows correct callbacks to pass
- Keeps Context/History independent of Session
- GUI just manages lifecycle, not rendering logic

### Why Callback Pattern Instead of Events?

**Alternative**: Event system (pub/sub)

**Decision**: Direct callbacks

**Rationale**:
- Simpler - fewer abstractions
- Clear call graph for debugging
- Type-safe with modern Python
- Can add event system later if needed

### Why Separate AttachmentInfo?

**Alternative**: Pass Attachment objects directly

**Decision**: Use DTO (Data Transfer Object)

**Rationale**:
- Decouples GUI from business objects
- GUI only needs display information
- Prevents GUI from modifying business state
- Clear API boundary
- Easier to test

## Future Enhancements

### Potential Extensions

1. **Multiple Themes**: Config-driven theme switching
2. **Custom Layouts**: Alternative layouts (horizontal split, single panel, etc.)
3. **Plugin System**: Third-party UI extensions
4. **Accessibility**: Screen reader support, keyboard navigation
5. **Responsive Design**: Adapt to window resizing
6. **Status Indicators**: Progress bars, spinners for long operations
7. **Dialog Management**: File pickers, confirmations, settings
8. **Undo/Redo**: For message editing and deletion

### Extension Points

```python
class GUIManager:
    def register_custom_panel(
        self,
        panel_id: str,
        factory: Callable[[tk.Widget], tk.Widget]
    ) -> None:
        """Register a custom panel for extension."""
        
    def get_theme(self) -> Theme:
        """Get current theme for consistent styling."""
```

## Conclusion

This architecture provides:

- **Clear Separation**: Business logic isolated from GUI concerns
- **Testability**: Mock GUI for unit testing business logic
- **Maintainability**: Changes to UI don't affect business logic
- **Flexibility**: Support alternative UIs or themes
- **Type Safety**: Strong typing at API boundaries
- **Simplicity**: Just enough abstraction, not over-engineered

The design is pragmatic - solves current problems while leaving room for future evolution.

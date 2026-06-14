package main

// AppletPaneSize captures terminal dimensions assigned to a widget pane.
type AppletPaneSize struct {
	Height int
	Width  int
}

// InputWidgetScreenState models computed geometry for the input applet.
type InputWidgetScreenState struct {
	Pane         AppletPaneSize
	ShowHelp     bool
	ViewportRows int
	ViewportCols int
	Layout       inputWidgetRenderLayout
	Components   InputWidgetComponents
}

// OutputWidgetScreenState models width budgeting for output applet content.
type OutputWidgetScreenState struct {
	Pane       AppletPaneSize
	Components OutputWidgetComponents
}

type InputWidgetComponents struct {
	Header     InputWidgetHeaderComponent
	ComposeBox InputWidgetComposeBoxComponent
	ControlBox InputWidgetControlBoxComponent
	Cursor     InputWidgetCursorAnchors
}

type InputWidgetHeaderComponent struct {
	TopRow    int
	TitleRows int
	HelpRows  int
}

type InputWidgetComposeBoxComponent struct {
	TopRow      int
	InnerTopRow int
	InnerRows   int
	InnerCols   int
}

type InputWidgetControlBoxComponent struct {
	TopRow      int
	InnerTopRow int
	InnerCols   int
}

type InputWidgetCursorAnchors struct {
	InputInnerTopRow   int
	ControlInnerTopRow int
}

type OutputWidgetComponents struct {
	Content OutputWidgetContentComponent
}

type OutputWidgetContentComponent struct {
	OuterPadding int
	MinBudget    int
}

func NewInputWidgetScreenStateFromPane(height int, width int, showHelp bool) InputWidgetScreenState {
	pane := AppletPaneSize{Height: height, Width: width}
	rows, cols := computeInputWidgetViewportSize(pane, showHelp)
	return NewInputWidgetScreenStateFromViewport(rows, cols, showHelp)
}

func NewInputWidgetScreenStateFromViewport(viewportRows int, viewportCols int, showHelp bool) InputWidgetScreenState {
	if viewportRows < 1 {
		viewportRows = 1
	}
	if viewportCols < 12 {
		viewportCols = 12
	}
	components, layout := buildInputWidgetComponents(viewportRows, viewportCols, showHelp)
	return InputWidgetScreenState{
		ShowHelp:     showHelp,
		ViewportRows: viewportRows,
		ViewportCols: viewportCols,
		Layout:       layout,
		Components:   components,
	}
}

func computeInputWidgetViewportSize(pane AppletPaneSize, showHelp bool) (rows int, cols int) {
	width := pane.Width
	if width < 44 {
		width = 44
	}
	header := 3
	if showHelp {
		header += 2
	}
	controlOuter := 3
	inputOuter := pane.Height - header - controlOuter
	if inputOuter < 5 {
		inputOuter = 5
	}
	inputInner := inputOuter - 2
	rows = inputInner - 1
	if rows < 1 {
		rows = 1
	}
	cols = width - 6
	if cols < 12 {
		cols = 12
	}
	return rows, cols
}

func buildInputWidgetComponents(viewportRows int, viewportCols int, showHelp bool) (InputWidgetComponents, inputWidgetRenderLayout) {
	headerLines := 1
	helpLines := 0
	if showHelp {
		helpLines = 2
		headerLines += helpLines
	}
	inputBoxTopRow := headerLines + 2
	controlBoxTopRow := inputBoxTopRow + (viewportRows + 3) + 1

	controlInnerCols := viewportCols + 1
	if controlInnerCols < 16 {
		controlInnerCols = 16
	}

	layout := inputWidgetRenderLayout{
		inputInnerTopRow:   inputBoxTopRow + 1,
		controlInnerTopRow: controlBoxTopRow + 1,
	}

	components := InputWidgetComponents{
		Header: InputWidgetHeaderComponent{
			TopRow:    1,
			TitleRows: 1,
			HelpRows:  helpLines,
		},
		ComposeBox: InputWidgetComposeBoxComponent{
			TopRow:      inputBoxTopRow,
			InnerTopRow: layout.inputInnerTopRow,
			InnerRows:   viewportRows,
			InnerCols:   viewportCols,
		},
		ControlBox: InputWidgetControlBoxComponent{
			TopRow:      controlBoxTopRow,
			InnerTopRow: layout.controlInnerTopRow,
			InnerCols:   controlInnerCols,
		},
		Cursor: InputWidgetCursorAnchors{
			InputInnerTopRow:   layout.inputInnerTopRow,
			ControlInnerTopRow: layout.controlInnerTopRow,
		},
	}
	return components, layout
}

func NewOutputWidgetScreenState(paneWidth int) OutputWidgetScreenState {
	if paneWidth < 20 {
		paneWidth = 20
	}
	return OutputWidgetScreenState{
		Pane: AppletPaneSize{Width: paneWidth},
		Components: OutputWidgetComponents{
			Content: OutputWidgetContentComponent{
				OuterPadding: 8,
				MinBudget:    12,
			},
		},
	}
}

func (s OutputWidgetScreenState) ContentBudget(linePrefix string) int {
	paneWidth := s.Pane.Width
	if paneWidth < 20 {
		paneWidth = 20
	}
	padding := s.Components.Content.OuterPadding
	if padding < 0 {
		padding = 0
	}
	minBudget := s.Components.Content.MinBudget
	if minBudget < 1 {
		minBudget = 1
	}
	usableWidth := paneWidth - padding
	budget := usableWidth - renderStringWidth(linePrefix)
	if budget < minBudget {
		budget = minBudget
	}
	return budget
}

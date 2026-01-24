package pane

// StatusBarHeight is the height reserved for the status bar at the bottom.
const StatusBarHeight = 2

// SidebarWidth is the width of the sidebar in characters.
const SidebarWidth = 24

// Layout represents the position and size of a pane in screen coordinates.
type Layout struct {
	X0, Y0, X1, Y1 int
}

// SidebarLayout holds both the sidebar and main area layouts.
type SidebarLayout struct {
	Sidebar Layout
	Main    Layout
}

// Width returns the interior width (excluding borders).
func (l Layout) Width() int {
	w := l.X1 - l.X0 - 1
	if w < 1 {
		return 1
	}
	return w
}

// Height returns the interior height (excluding borders).
func (l Layout) Height() int {
	h := l.Y1 - l.Y0 - 1
	if h < 1 {
		return 1
	}
	return h
}

// CalculateLayouts returns layouts for the given pane count and screen dimensions.
// Layout patterns:
//
//	1 pane:  [    1    ]
//	2 panes: [  1  ][  2  ]
//	3 panes: [  1  ][  2  ]
//	         [      3      ]
//	4 panes: [  1  ][  2  ]
//	         [  3  ][  4  ]
//	5 panes: [  1  ][  2  ][  3  ]
//	         [    4    ][    5    ]
//
// For 5+ panes: top row has ceil(n/2) panes, bottom row has floor(n/2).
func CalculateLayouts(count, maxX, maxY int) []Layout {
	if count == 0 {
		return nil
	}

	layouts := make([]Layout, count)

	switch count {
	case 1:
		layouts[0] = Layout{0, 0, maxX - 1, maxY - 1}
	case 2:
		halfX := maxX / 2
		layouts[0] = Layout{0, 0, halfX - 1, maxY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, maxY - 1}
	case 3:
		halfX := maxX / 2
		halfY := maxY / 2
		layouts[0] = Layout{0, 0, halfX - 1, halfY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, halfY - 1}
		layouts[2] = Layout{0, halfY, maxX - 1, maxY - 1}
	case 4:
		halfX := maxX / 2
		halfY := maxY / 2
		layouts[0] = Layout{0, 0, halfX - 1, halfY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, halfY - 1}
		layouts[2] = Layout{0, halfY, halfX - 1, maxY - 1}
		layouts[3] = Layout{halfX, halfY, maxX - 1, maxY - 1}
	default:
		// For 5+ panes: calculate rows and columns
		// Top row has ceil(n/2) panes, bottom row has floor(n/2) panes
		topCount := (count + 1) / 2
		bottomCount := count - topCount
		halfY := maxY / 2

		// Top row
		for i := range topCount {
			x0 := (maxX * i) / topCount
			x1 := (maxX * (i + 1)) / topCount
			layouts[i] = Layout{x0, 0, x1 - 1, halfY - 1}
		}

		// Bottom row
		for i := range bottomCount {
			x0 := (maxX * i) / bottomCount
			x1 := (maxX * (i + 1)) / bottomCount
			layouts[topCount+i] = Layout{x0, halfY, x1 - 1, maxY - 1}
		}
	}

	return layouts
}

// CalculateSidebarLayout returns layouts for a sidebar on the left and main area on the right.
func CalculateSidebarLayout(maxX, maxY int) SidebarLayout {
	sidebarWidth := SidebarWidth
	if sidebarWidth > maxX/3 {
		sidebarWidth = maxX / 3
	}
	if sidebarWidth < 10 {
		sidebarWidth = 10
	}

	return SidebarLayout{
		Sidebar: Layout{0, 0, sidebarWidth - 1, maxY - 1},
		Main:    Layout{sidebarWidth, 0, maxX - 1, maxY - 1},
	}
}

// UnifiedSidebarLayout holds the repos panel, sessions panel, and main view layouts.
type UnifiedSidebarLayout struct {
	Repos    Layout
	Sessions Layout
	Main     Layout
}

// CalculateUnifiedSidebarLayout returns layouts for the unified repo manager view:
// - Repos panel (top-left, ~40% of sidebar height)
// - Sessions panel (bottom-left, ~60% of sidebar height)
// - Main view (right side)
func CalculateUnifiedSidebarLayout(maxX, maxY int) UnifiedSidebarLayout {
	sidebarWidth := SidebarWidth
	if sidebarWidth > maxX/3 {
		sidebarWidth = maxX / 3
	}
	if sidebarWidth < 10 {
		sidebarWidth = 10
	}

	// Repos panel takes 40% of sidebar height
	repoHeight := maxY * 40 / 100
	if repoHeight < 5 {
		repoHeight = 5
	}

	return UnifiedSidebarLayout{
		Repos:    Layout{0, 0, sidebarWidth - 1, repoHeight - 1},
		Sessions: Layout{0, repoHeight, sidebarWidth - 1, maxY - 1},
		Main:     Layout{sidebarWidth, 0, maxX - 1, maxY - 1},
	}
}

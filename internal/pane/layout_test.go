package pane

import "testing"

func TestCalculateLayouts_SinglePane(t *testing.T) {
	layouts := CalculateLayouts(1, 100, 50)
	if len(layouts) != 1 {
		t.Errorf("expected 1 layout, got %d", len(layouts))
	}

	l := layouts[0]
	if l.X0 != 0 || l.Y0 != 0 || l.X1 != 99 || l.Y1 != 49 {
		t.Errorf("unexpected layout: %+v", l)
	}
}

func TestCalculateLayouts_TwoPanes(t *testing.T) {
	layouts := CalculateLayouts(2, 100, 50)
	if len(layouts) != 2 {
		t.Errorf("expected 2 layouts, got %d", len(layouts))
	}

	// First pane should be left half
	l0 := layouts[0]
	if l0.X0 != 0 || l0.Y0 != 0 || l0.X1 != 49 || l0.Y1 != 49 {
		t.Errorf("unexpected left layout: %+v", l0)
	}

	// Second pane should be right half
	l1 := layouts[1]
	if l1.X0 != 50 || l1.Y0 != 0 || l1.X1 != 99 || l1.Y1 != 49 {
		t.Errorf("unexpected right layout: %+v", l1)
	}
}

func TestCalculateLayouts_FourPanes(t *testing.T) {
	layouts := CalculateLayouts(4, 100, 50)
	if len(layouts) != 4 {
		t.Errorf("expected 4 layouts, got %d", len(layouts))
	}

	// Should be 2x2 grid
	// Top-left
	l0 := layouts[0]
	if l0.X0 != 0 || l0.Y0 != 0 || l0.X1 != 49 || l0.Y1 != 24 {
		t.Errorf("unexpected top-left layout: %+v", l0)
	}

	// Top-right
	l1 := layouts[1]
	if l1.X0 != 50 || l1.Y0 != 0 || l1.X1 != 99 || l1.Y1 != 24 {
		t.Errorf("unexpected top-right layout: %+v", l1)
	}

	// Bottom-left
	l2 := layouts[2]
	if l2.X0 != 0 || l2.Y0 != 25 || l2.X1 != 49 || l2.Y1 != 49 {
		t.Errorf("unexpected bottom-left layout: %+v", l2)
	}

	// Bottom-right
	l3 := layouts[3]
	if l3.X0 != 50 || l3.Y0 != 25 || l3.X1 != 99 || l3.Y1 != 49 {
		t.Errorf("unexpected bottom-right layout: %+v", l3)
	}
}

func TestLayout_Dimensions(t *testing.T) {
	l := Layout{X0: 0, Y0: 0, X1: 50, Y1: 25}
	if w := l.Width(); w != 49 {
		t.Errorf("expected width 49, got %d", w)
	}
	if h := l.Height(); h != 24 {
		t.Errorf("expected height 24, got %d", h)
	}
}

func TestCalculateLayouts_Empty(t *testing.T) {
	layouts := CalculateLayouts(0, 100, 50)
	if layouts != nil {
		t.Errorf("expected nil for 0 panes, got %v", layouts)
	}
}

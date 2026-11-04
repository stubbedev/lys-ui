package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

func countParsesOnLoop(t *testing.T) *int {
	t.Helper()
	loop := goroutineID()
	onLoop := new(int)
	parse := parseCSS
	parseCSS = func(css string) (*style.Sheet, []string) {
		if goroutineID() == loop {
			*onLoop++
		}
		return parse(css)
	}
	t.Cleanup(func() { parseCSS = parse })
	return onLoop
}

type styledScene struct {
	root  *Box
	cards []*Box
}

func newStyledScene(t *testing.T, sheet *Stylesheet) styledScene {
	t.Helper()
	sc := styledScene{root: NewBox(Column, 0, 0)}
	for i := range 6 {
		c := NewBox(Row, 0, 0)
		c.Append(NewSpacer(40, 10), false)
		if i%2 == 0 {
			c.AddClass("hot")
		}
		sc.root.Append(c, false)
		sc.cards = append(sc.cards, c)
	}
	sc.root.AttachStylesheet(sheet)
	SizeWithin(sc.root, Size{W: 40, H: 60})
	sc.root.Arrange(render.Rect{W: 40, H: 60})
	paintTree(sc.root, 40, 60)
	CollectDamage(sc.root)
	return sc
}

func (sc styledScene) restyles() int {
	n := nodeOf(sc.root).restyleCount()
	for _, c := range sc.cards {
		n += nodeOf(c).restyleCount()
	}
	return n
}

func (sc styledScene) settle() (area, restyles int) {
	before := sc.restyles()
	rects, _ := CollectDamage(sc.root)
	paintTree(sc.root, 40, 60)
	return DamageArea(rects), sc.restyles() - before
}

func TestLoadParsedParsesNothingOnTheLoopAndRestylesAsLoadDoes(t *testing.T) {
	onLoop := countParsesOnLoop(t)
	const first = `.hot { background-color: #ff0000; }`
	const next = `.hot { background-color: #0000ff; }`

	viaLoad := NewStylesheet(first, StylePriorityApplication)
	a := newStyledScene(t, viaLoad)
	*onLoop = 0
	viaLoad.Load(next)
	loadArea, loadRestyles := a.settle()
	if *onLoop != 1 {
		t.Fatalf("Load parsed %d times on the loop, want 1: the seam counts nothing", *onLoop)
	}

	viaParsed := NewStylesheet(first, StylePriorityApplication)
	b := newStyledScene(t, viaParsed)
	ready := make(chan *ParsedStylesheet)
	go func() { ready <- ParseStylesheet(next) }()
	parsed := <-ready
	*onLoop = 0
	viaParsed.LoadParsed(parsed)
	parsedArea, parsedRestyles := b.settle()

	if *onLoop != 0 {
		t.Errorf("installing a parsed sheet parsed %d times on the loop", *onLoop)
	}
	if loadArea == 0 || loadRestyles == 0 {
		t.Fatalf("Load damaged %d px with %d restyles: the change pins nothing", loadArea, loadRestyles)
	}
	if parsedArea != loadArea || parsedRestyles != loadRestyles {
		t.Errorf("LoadParsed damaged %d px with %d restyles, Load %d px with %d", parsedArea, parsedRestyles, loadArea, loadRestyles)
	}
	if got := render.ColorFromBytes(paintTree(b.root, 40, 60)[5*render.Stride(40)+5*4:]); got != render.RGB(0, 0, 255) {
		t.Errorf("hot card = %v, want the parsed blue", got)
	}
}

func TestParseStylesheetReportsItsWarningsWhenInstalled(t *testing.T) {
	var warns []string
	style.SetParseWarn(func(msg string) { warns = append(warns, msg) })
	t.Cleanup(func() { style.SetParseWarn(nil) })
	parsed := ParseStylesheet(`.a { color: ; } .b { color: #fff; }`)
	if len(warns) != 0 {
		t.Fatalf("the parse reported %v before the install", warns)
	}
	s := NewStylesheet("", StylePriorityApplication)
	s.LoadParsed(parsed)
	if len(warns) == 0 || !strings.HasPrefix(warns[0], "css: ") {
		t.Errorf("warnings = %v, want the parse's, as Load reports them", warns)
	}
}

package export

import (
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"

	"git.sr.ht/~sbinet/gg"
	"github.com/ajstarks/svgo"
)

// Beautiful color palette (Dracula-inspired dark theme)
var (
	// Background colors
	bgDark      = color.RGBA{0x1e, 0x1e, 0x2e, 0xff} // Deep dark blue-gray
	bgCard      = color.RGBA{0x2a, 0x2a, 0x3e, 0xff} // Slightly lighter card bg
	bgHeader    = color.RGBA{0x24, 0x24, 0x34, 0xff} // Header background
	bgGlow      = color.RGBA{0x50, 0xfa, 0x7b, 0x40} // Green glow (semi-transparent)
	bgGlowBlue  = color.RGBA{0x8b, 0xe9, 0xfd, 0x40} // Blue glow
	bgGlowPink  = color.RGBA{0xff, 0x79, 0xc6, 0x40} // Pink glow

	// Node status colors (vibrant Dracula palette)
	nodeOpen     = color.RGBA{0x50, 0xfa, 0x7b, 0xff} // Bright green
	nodeProgress = color.RGBA{0x8b, 0xe9, 0xfd, 0xff} // Cyan
	nodeBlocked  = color.RGBA{0xff, 0x55, 0x55, 0xff} // Red
	nodeClosed   = color.RGBA{0x62, 0x72, 0xa4, 0xff} // Muted purple-gray

	// Edge colors
	edgeNormal  = color.RGBA{0x6b, 0x80, 0xbf, 0x80} // Semi-transparent blue
	edgeBlocks  = color.RGBA{0xff, 0x55, 0x55, 0xa0} // Red for blocking deps
	edgeRelated = color.RGBA{0xbd, 0x93, 0xf9, 0x60} // Purple for related

	// Text colors
	textPrimary   = color.RGBA{0xf8, 0xf8, 0xf2, 0xff} // Off-white
	textSecondary = color.RGBA{0xa0, 0xa0, 0xb0, 0xff} // Muted
	textAccent    = color.RGBA{0xbd, 0x93, 0xf9, 0xff} // Purple accent

	// Priority colors for badges
	prioP0 = color.RGBA{0xff, 0x55, 0x55, 0xff} // Critical - Red
	prioP1 = color.RGBA{0xff, 0xb8, 0x6c, 0xff} // High - Orange
	prioP2 = color.RGBA{0xf1, 0xfa, 0x8c, 0xff} // Medium - Yellow
	prioP3 = color.RGBA{0x8b, 0xe9, 0xfd, 0xff} // Low - Cyan
	prioP4 = color.RGBA{0x62, 0x72, 0xa4, 0xff} // Backlog - Gray
)

// statusColorBeautiful returns the vibrant color for a status
func statusColorBeautiful(s model.Status) color.RGBA {
	switch s {
	case model.StatusOpen:
		return nodeOpen
	case model.StatusInProgress:
		return nodeProgress
	case model.StatusBlocked:
		return nodeBlocked
	case model.StatusClosed:
		return nodeClosed
	default:
		return nodeOpen
	}
}

// priorityColor returns color for priority badge
func priorityColor(p int) color.RGBA {
	switch p {
	case 0:
		return prioP0
	case 1:
		return prioP1
	case 2:
		return prioP2
	case 3:
		return prioP3
	default:
		return prioP4
	}
}

// RenderForceLayoutPNG renders a beautiful PNG from a force layout
func RenderForceLayoutPNG(layout ForceLayout, path string) error {
	width := int(layout.Width)
	height := int(layout.Height)

	// Ensure minimum size
	if width < 800 {
		width = 800
	}
	if height < 600 {
		height = 600
	}

	dc := gg.NewContext(width, height)

	// Fill background with gradient effect
	dc.SetColor(bgDark)
	dc.Clear()

	// Draw subtle radial gradient from center (darker edges)
	cx, cy := float64(width)/2, float64(height)/2
	maxDist := math.Sqrt(cx*cx + cy*cy)
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			dist := math.Sqrt(float64((x-width/2)*(x-width/2)+(y-height/2)*(y-height/2))) / maxDist
			alpha := uint8(20 * dist)
			dc.SetColor(color.RGBA{0, 0, 0, alpha})
			dc.DrawRectangle(float64(x), float64(y), 4, 4)
			dc.Fill()
		}
	}

	// Draw header card
	drawHeaderCard(dc, width, layout)

	// Build node position map for edges
	nodePos := make(map[string]*ForceNode)
	for i := range layout.Nodes {
		nodePos[layout.Nodes[i].ID] = &layout.Nodes[i]
	}

	// Draw edges first (below nodes) with bezier curves
	for _, e := range layout.Edges {
		from := nodePos[e.From]
		to := nodePos[e.To]
		if from == nil || to == nil {
			continue
		}
		drawBezierEdge(dc, from, to, e.Type)
	}

	// Draw nodes with glow effects (sorted by pagerank, important on top)
	for i := range layout.Nodes {
		node := &layout.Nodes[i]
		drawBeautifulNode(dc, node, node.ID == layout.TopNode)
	}

	// Draw legend
	drawBeautifulLegend(dc, width, height)

	return dc.SavePNG(path)
}

// RenderForceLayoutSVG renders a beautiful SVG from a force layout
func RenderForceLayoutSVG(layout ForceLayout, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return RenderForceLayoutSVGToWriter(layout, file)
}

// RenderForceLayoutSVGToWriter renders SVG to a writer
func RenderForceLayoutSVGToWriter(layout ForceLayout, w io.Writer) error {
	width := int(layout.Width)
	height := int(layout.Height)

	if width < 800 {
		width = 800
	}
	if height < 600 {
		height = 600
	}

	canvas := svg.New(w)
	canvas.Start(width, height)

	// Define gradients and filters
	canvas.Def()

	// Background gradient
	canvas.LinearGradient("bgGrad", 0, 0, 0, 100, []svg.Offcolor{
		{Offset: 0, Color: cssRGBA(bgDark), Opacity: 1},
		{Offset: 100, Color: "#151520", Opacity: 1},
	})

	// Glow filter for important nodes
	canvas.Filter("glow")
	canvas.FeGaussianBlur(svg.Filterspec{In: "SourceGraphic", Result: "blur"}, 8, 8)
	canvas.FeMerge([]string{"blur", "SourceGraphic"})
	canvas.Fend()

	// Drop shadow filter
	canvas.Filter("shadow")
	canvas.FeGaussianBlur(svg.Filterspec{In: "SourceAlpha", Result: "blur"}, 4, 4)
	canvas.FeOffset(svg.Filterspec{In: "blur", Result: "offsetBlur"}, 2, 2)
	canvas.FeMerge([]string{"offsetBlur", "SourceGraphic"})
	canvas.Fend()

	// Node gradients for each status
	createNodeGradient(canvas, "gradOpen", nodeOpen)
	createNodeGradient(canvas, "gradProgress", nodeProgress)
	createNodeGradient(canvas, "gradBlocked", nodeBlocked)
	createNodeGradient(canvas, "gradClosed", nodeClosed)

	canvas.DefEnd()

	// Background
	canvas.Rect(0, 0, width, height, "fill:url(#bgGrad)")

	// Header
	drawHeaderCardSVG(canvas, width, layout)

	// Build node map
	nodePos := make(map[string]*ForceNode)
	for i := range layout.Nodes {
		nodePos[layout.Nodes[i].ID] = &layout.Nodes[i]
	}

	// Draw edges with bezier curves
	for _, e := range layout.Edges {
		from := nodePos[e.From]
		to := nodePos[e.To]
		if from == nil || to == nil {
			continue
		}
		drawBezierEdgeSVG(canvas, from, to, e.Type)
	}

	// Draw nodes
	for i := range layout.Nodes {
		node := &layout.Nodes[i]
		drawBeautifulNodeSVG(canvas, node, node.ID == layout.TopNode)
	}

	// Legend
	drawBeautifulLegendSVG(canvas, width, height)

	canvas.End()
	return nil
}

func createNodeGradient(canvas *svg.SVG, id string, base color.RGBA) {
	lighter := color.RGBA{
		R: uint8(math.Min(255, float64(base.R)*1.3)),
		G: uint8(math.Min(255, float64(base.G)*1.3)),
		B: uint8(math.Min(255, float64(base.B)*1.3)),
		A: base.A,
	}
	canvas.LinearGradient(id, 0, 0, 0, 100, []svg.Offcolor{
		{Offset: 0, Color: cssRGBA(lighter), Opacity: 1},
		{Offset: 100, Color: cssRGBA(base), Opacity: 1},
	})
}

func drawHeaderCard(dc *gg.Context, width int, layout ForceLayout) {
	// Semi-transparent header background
	dc.SetColor(color.RGBA{0x24, 0x24, 0x34, 0xe0})
	dc.DrawRoundedRectangle(20, 12, float64(width)-40, 76, 12)
	dc.Fill()

	// Border glow
	dc.SetLineWidth(1)
	dc.SetColor(color.RGBA{0xbd, 0x93, 0xf9, 0x60})
	dc.DrawRoundedRectangle(20, 12, float64(width)-40, 76, 12)
	dc.Stroke()

	// Title
	dc.SetColor(textPrimary)
	title := layout.Title
	if title == "" {
		title = "Dependency Graph"
	}
	dc.DrawStringAnchored(title, 36, 36, 0, 0.5)

	// Stats
	dc.SetColor(textSecondary)
	dc.DrawStringAnchored(fmt.Sprintf("%d nodes · %d edges", len(layout.Nodes), len(layout.Edges)), 36, 56, 0, 0.5)

	// Top node
	if layout.TopNode != "" {
		dc.SetColor(textAccent)
		dc.DrawStringAnchored(fmt.Sprintf("★ %s (PR %.3f)", layout.TopNode, layout.TopNodeRank), 36, 76, 0, 0.5)
	}

	// Hash
	if layout.DataHash != "" {
		dc.SetColor(color.RGBA{0x62, 0x72, 0xa4, 0xff})
		dc.DrawStringAnchored(fmt.Sprintf("#%s", layout.DataHash[:8]), float64(width)-36, 36, 1, 0.5)
	}
}

func drawHeaderCardSVG(canvas *svg.SVG, width int, layout ForceLayout) {
	// Header background with border
	canvas.Roundrect(20, 12, width-40, 76, 12, 12,
		fmt.Sprintf("fill:%s;fill-opacity:0.88;stroke:%s;stroke-opacity:0.4", cssRGBA(bgHeader), cssRGBA(textAccent)))

	title := layout.Title
	if title == "" {
		title = "Dependency Graph"
	}

	canvas.Text(36, 40, title, fmt.Sprintf("fill:%s;font-size:18px;font-family:system-ui,sans-serif;font-weight:600", cssRGBA(textPrimary)))
	canvas.Text(36, 60, fmt.Sprintf("%d nodes · %d edges", len(layout.Nodes), len(layout.Edges)),
		fmt.Sprintf("fill:%s;font-size:13px;font-family:system-ui,sans-serif", cssRGBA(textSecondary)))

	if layout.TopNode != "" {
		canvas.Text(36, 80, fmt.Sprintf("★ %s (PR %.3f)", layout.TopNode, layout.TopNodeRank),
			fmt.Sprintf("fill:%s;font-size:12px;font-family:system-ui,sans-serif", cssRGBA(textAccent)))
	}

	if layout.DataHash != "" && len(layout.DataHash) >= 8 {
		canvas.Text(width-36, 40, "#"+layout.DataHash[:8],
			fmt.Sprintf("fill:%s;font-size:11px;font-family:monospace;text-anchor:end", cssRGBA(nodeClosed)))
	}
}

func drawBezierEdge(dc *gg.Context, from, to *ForceNode, depType model.DependencyType) {
	// Calculate control points for smooth bezier curve
	x1, y1 := from.X, from.Y
	x2, y2 := to.X, to.Y

	// Midpoint
	mx := (x1 + x2) / 2
	my := (y1 + y2) / 2

	// Perpendicular offset for curve (based on distance)
	dist := math.Sqrt((x2-x1)*(x2-x1) + (y2-y1)*(y2-y1))
	offset := dist * 0.15

	// Perpendicular direction
	dx := x2 - x1
	dy := y2 - y1
	px := -dy / dist
	py := dx / dist

	// Control point
	cx := mx + px*offset
	cy := my + py*offset

	// Set color based on dependency type
	edgeColor := edgeNormal
	if depType == model.DepBlocks {
		edgeColor = edgeBlocks
	}

	// Draw glow
	dc.SetColor(color.RGBA{edgeColor.R, edgeColor.G, edgeColor.B, 0x30})
	dc.SetLineWidth(6)
	dc.MoveTo(x1, y1)
	dc.QuadraticTo(cx, cy, x2, y2)
	dc.Stroke()

	// Draw main line
	dc.SetColor(edgeColor)
	dc.SetLineWidth(2)
	dc.MoveTo(x1, y1)
	dc.QuadraticTo(cx, cy, x2, y2)
	dc.Stroke()

	// Draw arrowhead at destination
	drawArrowHead(dc, cx, cy, x2, y2, edgeColor, to.Radius)
}

func drawArrowHead(dc *gg.Context, cx, cy, x2, y2 float64, c color.RGBA, nodeRadius float64) {
	// Direction from control point to end
	dx := x2 - cx
	dy := y2 - cy
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist == 0 {
		return
	}
	dx /= dist
	dy /= dist

	// Arrow at node edge
	ax := x2 - dx*(nodeRadius+4)
	ay := y2 - dy*(nodeRadius+4)

	// Arrow size
	arrowLen := 12.0
	arrowWidth := 6.0

	// Perpendicular
	px := -dy
	py := dx

	// Arrow points
	p1x := ax - dx*arrowLen + px*arrowWidth
	p1y := ay - dy*arrowLen + py*arrowWidth
	p2x := ax - dx*arrowLen - px*arrowWidth
	p2y := ay - dy*arrowLen - py*arrowWidth

	dc.SetColor(c)
	dc.MoveTo(ax, ay)
	dc.LineTo(p1x, p1y)
	dc.LineTo(p2x, p2y)
	dc.ClosePath()
	dc.Fill()
}

func drawBezierEdgeSVG(canvas *svg.SVG, from, to *ForceNode, depType model.DependencyType) {
	x1, y1 := from.X, from.Y
	x2, y2 := to.X, to.Y

	mx := (x1 + x2) / 2
	my := (y1 + y2) / 2

	dist := math.Sqrt((x2-x1)*(x2-x1) + (y2-y1)*(y2-y1))
	offset := dist * 0.15

	dx := x2 - x1
	dy := y2 - y1
	px := -dy / dist
	py := dx / dist

	cx := mx + px*offset
	cy := my + py*offset

	edgeColor := edgeNormal
	if depType == model.DepBlocks {
		edgeColor = edgeBlocks
	}

	// Path for bezier curve
	pathD := fmt.Sprintf("M %.1f %.1f Q %.1f %.1f %.1f %.1f", x1, y1, cx, cy, x2, y2)

	// Glow
	canvas.Path(pathD, fmt.Sprintf("fill:none;stroke:%s;stroke-width:6;stroke-opacity:0.2", cssRGBA(edgeColor)))

	// Main line
	canvas.Path(pathD, fmt.Sprintf("fill:none;stroke:%s;stroke-width:2", cssRGBA(edgeColor)))

	// Arrowhead
	ddx := x2 - cx
	ddy := y2 - cy
	d := math.Sqrt(ddx*ddx + ddy*ddy)
	if d > 0 {
		ddx /= d
		ddy /= d

		ax := x2 - ddx*(to.Radius+4)
		ay := y2 - ddy*(to.Radius+4)

		arrowLen := 12.0
		arrowWidth := 6.0

		ppx := -ddy
		ppy := ddx

		p1x := ax - ddx*arrowLen + ppx*arrowWidth
		p1y := ay - ddy*arrowLen + ppy*arrowWidth
		p2x := ax - ddx*arrowLen - ppx*arrowWidth
		p2y := ay - ddy*arrowLen - ppy*arrowWidth

		canvas.Polygon(
			[]int{int(ax), int(p1x), int(p2x)},
			[]int{int(ay), int(p1y), int(p2y)},
			fmt.Sprintf("fill:%s", cssRGBA(edgeColor)),
		)
	}
}

func drawBeautifulNode(dc *gg.Context, node *ForceNode, isTop bool) {
	x, y := node.X, node.Y
	r := node.Radius

	statusColor := statusColorBeautiful(node.Status)

	// Draw glow for important nodes
	if isTop || node.Priority <= 1 {
		glowColor := color.RGBA{statusColor.R, statusColor.G, statusColor.B, 0x40}
		for i := 3; i > 0; i-- {
			dc.SetColor(color.RGBA{glowColor.R, glowColor.G, glowColor.B, uint8(20 * i)})
			dc.DrawCircle(x, y, r+float64(i*6))
			dc.Fill()
		}
	}

	// Drop shadow
	dc.SetColor(color.RGBA{0, 0, 0, 0x40})
	dc.DrawCircle(x+3, y+3, r)
	dc.Fill()

	// Main node circle with gradient effect (lighter on top)
	// Draw multiple rings for gradient effect
	for i := 0; i < 5; i++ {
		factor := float64(i) / 4.0
		rr := r - float64(i)*2
		if rr < 0 {
			break
		}
		c := lerpColor(statusColor, color.RGBA{
			R: uint8(math.Min(255, float64(statusColor.R)+50)),
			G: uint8(math.Min(255, float64(statusColor.G)+50)),
			B: uint8(math.Min(255, float64(statusColor.B)+50)),
			A: 255,
		}, factor)
		dc.SetColor(c)
		dc.DrawCircle(x, y-float64(i)*0.5, rr)
		dc.Fill()
	}

	// Border
	dc.SetLineWidth(2)
	dc.SetColor(color.RGBA{statusColor.R, statusColor.G, statusColor.B, 0xa0})
	dc.DrawCircle(x, y, r)
	dc.Stroke()

	// Inner highlight (top-left)
	dc.SetColor(color.RGBA{255, 255, 255, 0x30})
	dc.DrawArc(x, y, r*0.7, math.Pi*1.25, math.Pi*1.75)
	dc.SetLineWidth(3)
	dc.Stroke()

	// ID text
	dc.SetColor(textPrimary)
	dc.DrawStringAnchored(node.ID, x, y-6, 0.5, 0.5)

	// PageRank score
	dc.SetColor(textSecondary)
	dc.DrawStringAnchored(fmt.Sprintf("%.3f", node.PageRank), x, y+10, 0.5, 0.5)

	// Priority badge
	if node.Priority <= 2 {
		badgeX := x + r*0.7
		badgeY := y - r*0.7
		badgeColor := priorityColor(node.Priority)

		dc.SetColor(badgeColor)
		dc.DrawCircle(badgeX, badgeY, 10)
		dc.Fill()

		dc.SetColor(color.RGBA{0, 0, 0, 0xff})
		dc.DrawStringAnchored(fmt.Sprintf("P%d", node.Priority), badgeX, badgeY, 0.5, 0.5)
	}
}

func drawBeautifulNodeSVG(canvas *svg.SVG, node *ForceNode, isTop bool) {
	x, y := int(node.X), int(node.Y)
	r := int(node.Radius)

	statusColor := statusColorBeautiful(node.Status)
	gradID := gradientID(node.Status)

	// Glow for important nodes
	filter := ""
	if isTop || node.Priority <= 1 {
		filter = "filter:url(#glow)"
		canvas.Circle(x, y, r+10, fmt.Sprintf("fill:%s;fill-opacity:0.3;%s", cssRGBA(statusColor), filter))
	}

	// Drop shadow
	canvas.Circle(x+3, y+3, r, "fill:rgba(0,0,0,0.25)")

	// Main circle with gradient
	canvas.Circle(x, y, r, fmt.Sprintf("fill:url(#%s);stroke:%s;stroke-width:2;stroke-opacity:0.6;filter:url(#shadow)", gradID, cssRGBA(statusColor)))

	// ID text
	canvas.Text(x, y-4, node.ID, fmt.Sprintf("fill:%s;font-size:11px;font-family:system-ui,sans-serif;font-weight:600;text-anchor:middle;dominant-baseline:middle", cssRGBA(textPrimary)))

	// PageRank
	canvas.Text(x, y+12, fmt.Sprintf("%.3f", node.PageRank), fmt.Sprintf("fill:%s;font-size:9px;font-family:system-ui,sans-serif;text-anchor:middle", cssRGBA(textSecondary)))

	// Priority badge
	if node.Priority <= 2 {
		bx := x + int(float64(r)*0.7)
		by := y - int(float64(r)*0.7)
		badgeColor := priorityColor(node.Priority)

		canvas.Circle(bx, by, 10, fmt.Sprintf("fill:%s", cssRGBA(badgeColor)))
		canvas.Text(bx, by+1, fmt.Sprintf("P%d", node.Priority), "fill:#000;font-size:9px;font-family:system-ui,sans-serif;font-weight:bold;text-anchor:middle;dominant-baseline:middle")
	}
}

func gradientID(status model.Status) string {
	switch status {
	case model.StatusOpen:
		return "gradOpen"
	case model.StatusInProgress:
		return "gradProgress"
	case model.StatusBlocked:
		return "gradBlocked"
	case model.StatusClosed:
		return "gradClosed"
	default:
		return "gradOpen"
	}
}

func drawBeautifulLegend(dc *gg.Context, width, height int) {
	boxW := 160.0
	boxH := 130.0
	x := float64(width) - boxW - 20
	y := float64(height) - boxH - 20

	// Background
	dc.SetColor(color.RGBA{0x24, 0x24, 0x34, 0xe0})
	dc.DrawRoundedRectangle(x, y, boxW, boxH, 10)
	dc.Fill()

	// Border
	dc.SetLineWidth(1)
	dc.SetColor(color.RGBA{0x62, 0x72, 0xa4, 0x60})
	dc.DrawRoundedRectangle(x, y, boxW, boxH, 10)
	dc.Stroke()

	// Title
	dc.SetColor(textPrimary)
	dc.DrawStringAnchored("Status", x+12, y+18, 0, 0.5)

	// Legend items
	items := []struct {
		color color.RGBA
		label string
	}{
		{nodeOpen, "Open"},
		{nodeProgress, "In Progress"},
		{nodeBlocked, "Blocked"},
		{nodeClosed, "Closed"},
	}

	for i, item := range items {
		iy := y + 40 + float64(i)*22

		// Color circle
		dc.SetColor(item.color)
		dc.DrawCircle(x+20, iy, 8)
		dc.Fill()

		// Label
		dc.SetColor(textSecondary)
		dc.DrawStringAnchored(item.label, x+36, iy, 0, 0.5)
	}
}

func drawBeautifulLegendSVG(canvas *svg.SVG, width, height int) {
	boxW := 160
	boxH := 130
	x := width - boxW - 20
	y := height - boxH - 20

	canvas.Roundrect(x, y, boxW, boxH, 10, 10,
		fmt.Sprintf("fill:%s;fill-opacity:0.88;stroke:%s;stroke-opacity:0.4", cssRGBA(bgHeader), cssRGBA(nodeClosed)))

	canvas.Text(x+12, y+22, "Status", fmt.Sprintf("fill:%s;font-size:13px;font-family:system-ui,sans-serif;font-weight:600", cssRGBA(textPrimary)))

	items := []struct {
		color color.RGBA
		label string
	}{
		{nodeOpen, "Open"},
		{nodeProgress, "In Progress"},
		{nodeBlocked, "Blocked"},
		{nodeClosed, "Closed"},
	}

	for i, item := range items {
		iy := y + 44 + i*22
		canvas.Circle(x+20, iy, 8, fmt.Sprintf("fill:%s", cssRGBA(item.color)))
		canvas.Text(x+36, iy+4, item.label, fmt.Sprintf("fill:%s;font-size:11px;font-family:system-ui,sans-serif", cssRGBA(textSecondary)))
	}
}

// lerpColor linearly interpolates between two colors
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R) + t*(float64(b.R)-float64(a.R))),
		G: uint8(float64(a.G) + t*(float64(b.G)-float64(a.G))),
		B: uint8(float64(a.B) + t*(float64(b.B)-float64(a.B))),
		A: uint8(float64(a.A) + t*(float64(b.A)-float64(a.A))),
	}
}

func cssRGBA(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// SaveBeautifulGraphSnapshot is the main entry point for beautiful graph export
func SaveBeautifulGraphSnapshot(opts GraphSnapshotOptions) error {
	if len(opts.Issues) == 0 {
		return fmt.Errorf("no issues to export")
	}
	if opts.Stats == nil {
		return fmt.Errorf("graph stats are required")
	}

	// Compute force-directed layout
	layoutOpts := ForceLayoutOptions{
		Issues:   opts.Issues,
		Stats:    opts.Stats,
		Title:    opts.Title,
		DataHash: opts.DataHash,
	}

	// Adjust parameters based on preset
	if strings.EqualFold(opts.Preset, "roomy") {
		layoutOpts.MinNodeSize = 30
		layoutOpts.MaxNodeSize = 70
		layoutOpts.RepelForce = 12000
	}

	layout := ComputeForceLayout(layoutOpts)

	// Determine format
	format := strings.ToLower(opts.Format)
	if format == "" {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(opts.Path), "."))
		if ext == "png" || ext == "svg" {
			format = ext
		} else {
			format = "svg"
		}
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	switch format {
	case "png":
		return RenderForceLayoutPNG(layout, opts.Path)
	case "svg":
		return RenderForceLayoutSVG(layout, opts.Path)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

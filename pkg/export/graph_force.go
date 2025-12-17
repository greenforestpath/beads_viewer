package export

import (
	"math"
	"math/rand"
	"sort"

	"github.com/Dicklesworthstone/beads_viewer/pkg/analysis"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// ForceNode represents a node in the force-directed layout
type ForceNode struct {
	ID       string
	Title    string
	Status   model.Status
	Priority int
	PageRank float64
	X, Y     float64 // Position
	Vx, Vy   float64 // Velocity
	Radius   float64 // Node size based on importance
}

// ForceEdge represents an edge in the force layout
type ForceEdge struct {
	From, To string
	Type     model.DependencyType
}

// ForceLayout contains the computed force-directed layout
type ForceLayout struct {
	Nodes       []ForceNode
	Edges       []ForceEdge
	Width       float64
	Height      float64
	CenterX     float64
	CenterY     float64
	MinX, MaxX  float64
	MinY, MaxY  float64
	Title       string
	DataHash    string
	TopNode     string // Highest pagerank
	TopNodeRank float64
}

// ForceLayoutOptions configures the force simulation
type ForceLayoutOptions struct {
	Issues       []model.Issue
	Stats        *analysis.GraphStats
	Title        string
	DataHash     string
	Iterations   int     // Number of simulation iterations (default 300)
	RepelForce   float64 // Node repulsion strength (default 5000)
	AttractForce float64 // Edge attraction strength (default 0.02)
	Damping      float64 // Velocity damping (default 0.85)
	MinNodeSize  float64 // Minimum node radius (default 20)
	MaxNodeSize  float64 // Maximum node radius (default 50)
}

// ComputeForceLayout runs the Fruchterman-Reingold force-directed layout algorithm
func ComputeForceLayout(opts ForceLayoutOptions) ForceLayout {
	// Set defaults
	if opts.Iterations == 0 {
		opts.Iterations = 300
	}
	if opts.RepelForce == 0 {
		opts.RepelForce = 8000
	}
	if opts.AttractForce == 0 {
		opts.AttractForce = 0.015
	}
	if opts.Damping == 0 {
		opts.Damping = 0.85
	}
	if opts.MinNodeSize == 0 {
		opts.MinNodeSize = 24
	}
	if opts.MaxNodeSize == 0 {
		opts.MaxNodeSize = 60
	}

	// Build nodes
	pageRank := opts.Stats.PageRank()
	betweenness := opts.Stats.Betweenness()

	// Find max pagerank for normalization
	maxPR := 0.0
	for _, pr := range pageRank {
		if pr > maxPR {
			maxPR = pr
		}
	}
	if maxPR == 0 {
		maxPR = 1
	}

	// Calculate canvas size based on node count
	nodeCount := len(opts.Issues)
	canvasSize := math.Max(800, math.Sqrt(float64(nodeCount))*200)

	// Create nodes with random initial positions
	rng := rand.New(rand.NewSource(42)) // Deterministic for reproducibility
	nodes := make([]ForceNode, 0, nodeCount)
	nodeMap := make(map[string]*ForceNode)

	var topNode string
	var topNodeRank float64

	for _, iss := range opts.Issues {
		pr := pageRank[iss.ID]
		if pr > topNodeRank {
			topNodeRank = pr
			topNode = iss.ID
		}

		// Size based on pagerank + betweenness
		importance := pr/maxPR*0.7 + betweenness[iss.ID]*0.3
		radius := opts.MinNodeSize + importance*(opts.MaxNodeSize-opts.MinNodeSize)

		node := ForceNode{
			ID:       iss.ID,
			Title:    iss.Title,
			Status:   iss.Status,
			Priority: iss.Priority,
			PageRank: pr,
			X:        canvasSize/2 + (rng.Float64()-0.5)*canvasSize*0.8,
			Y:        canvasSize/2 + (rng.Float64()-0.5)*canvasSize*0.8,
			Radius:   radius,
		}
		nodes = append(nodes, node)
		nodeMap[iss.ID] = &nodes[len(nodes)-1]
	}

	// Build edges
	var edges []ForceEdge
	issueIDs := make(map[string]bool)
	for _, iss := range opts.Issues {
		issueIDs[iss.ID] = true
	}
	for _, iss := range opts.Issues {
		for _, dep := range iss.Dependencies {
			if dep == nil || !issueIDs[dep.DependsOnID] {
				continue
			}
			edges = append(edges, ForceEdge{
				From: iss.ID,
				To:   dep.DependsOnID,
				Type: dep.Type,
			})
		}
	}

	// Build adjacency for attraction
	adjacency := make(map[string][]string)
	for _, e := range edges {
		adjacency[e.From] = append(adjacency[e.From], e.To)
		adjacency[e.To] = append(adjacency[e.To], e.From)
	}

	// Run force simulation
	temperature := canvasSize / 2 // Initial temperature for simulated annealing
	for iter := 0; iter < opts.Iterations; iter++ {
		// Calculate repulsive forces (all pairs)
		for i := range nodes {
			nodes[i].Vx = 0
			nodes[i].Vy = 0
		}

		for i := range nodes {
			for j := range nodes {
				if i == j {
					continue
				}
				dx := nodes[i].X - nodes[j].X
				dy := nodes[i].Y - nodes[j].Y
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < 1 {
					dist = 1
				}

				// Repulsive force (Coulomb's law)
				force := opts.RepelForce / (dist * dist)
				nodes[i].Vx += (dx / dist) * force
				nodes[i].Vy += (dy / dist) * force
			}
		}

		// Calculate attractive forces (edges only)
		for _, e := range edges {
			n1 := nodeMap[e.From]
			n2 := nodeMap[e.To]
			if n1 == nil || n2 == nil {
				continue
			}

			dx := n2.X - n1.X
			dy := n2.Y - n1.Y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < 1 {
				dist = 1
			}

			// Attractive force (Hooke's law)
			force := dist * opts.AttractForce

			// Apply to both nodes (opposite directions)
			n1.Vx += (dx / dist) * force
			n1.Vy += (dy / dist) * force
			n2.Vx -= (dx / dist) * force
			n2.Vy -= (dy / dist) * force
		}

		// Apply forces with temperature limiting
		for i := range nodes {
			// Limit displacement by temperature
			disp := math.Sqrt(nodes[i].Vx*nodes[i].Vx + nodes[i].Vy*nodes[i].Vy)
			if disp > temperature {
				nodes[i].Vx = (nodes[i].Vx / disp) * temperature
				nodes[i].Vy = (nodes[i].Vy / disp) * temperature
			}

			// Apply damping
			nodes[i].Vx *= opts.Damping
			nodes[i].Vy *= opts.Damping

			// Update position
			nodes[i].X += nodes[i].Vx
			nodes[i].Y += nodes[i].Vy
		}

		// Cool down (simulated annealing)
		temperature *= 0.97
	}

	// Calculate bounds
	minX, maxX := math.MaxFloat64, -math.MaxFloat64
	minY, maxY := math.MaxFloat64, -math.MaxFloat64
	for _, n := range nodes {
		if n.X-n.Radius < minX {
			minX = n.X - n.Radius
		}
		if n.X+n.Radius > maxX {
			maxX = n.X + n.Radius
		}
		if n.Y-n.Radius < minY {
			minY = n.Y - n.Radius
		}
		if n.Y+n.Radius > maxY {
			maxY = n.Y + n.Radius
		}
	}

	// Add padding
	padding := 100.0
	minX -= padding
	minY -= padding
	maxX += padding
	maxY += padding

	// Normalize positions to canvas with header space
	headerHeight := 100.0
	width := maxX - minX
	height := maxY - minY + headerHeight

	for i := range nodes {
		nodes[i].X = nodes[i].X - minX
		nodes[i].Y = nodes[i].Y - minY + headerHeight
	}

	// Sort nodes by pagerank for consistent rendering order (low first, high on top)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].PageRank < nodes[j].PageRank
	})

	return ForceLayout{
		Nodes:       nodes,
		Edges:       edges,
		Width:       width,
		Height:      height,
		CenterX:     width / 2,
		CenterY:     height / 2,
		MinX:        0,
		MaxX:        width,
		MinY:        0,
		MaxY:        height,
		Title:       opts.Title,
		DataHash:    opts.DataHash,
		TopNode:     topNode,
		TopNodeRank: topNodeRank,
	}
}

// GetNodeByID finds a node by ID
func (fl *ForceLayout) GetNodeByID(id string) *ForceNode {
	for i := range fl.Nodes {
		if fl.Nodes[i].ID == id {
			return &fl.Nodes[i]
		}
	}
	return nil
}

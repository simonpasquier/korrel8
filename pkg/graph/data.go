package graph

import (
	"fmt"

	"github.com/korrel8r/korrel8r/pkg/korrel8r"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
)

// Data contains the class nodes and rule lines for rule/class graphs.
// All graphs based on the same Data have stable, consistent node and line IDs.
// Note new rules can be added to a Data instance but never removed.
type Data struct {
	Nodes  []*Node // Nodes slice index == Node.ID()
	Lines  []*Line // Lines slice index == Line.ID()
	nodeID map[korrel8r.Class]int64
}

func NewData(rules ...korrel8r.Rule) *Data {
	d := Data{nodeID: make(map[korrel8r.Class]int64)}
	for _, r := range rules {
		d.AddRule(r)
	}
	return &d
}

func (d *Data) AddRule(r korrel8r.Rule) {
	id := int64(len(d.Lines))
	l := &Line{
		Line:        multi.Line{F: d.NodeFor(r.Start()), T: d.NodeFor(r.Goal()), UID: id},
		Rule:        r,
		Attrs:       Attrs{},
		QueryCounts: QueryCounts{},
	}
	d.Lines = append(d.Lines, l)
}

// NodeFor returns the Node for class c, creating it if necessary.
func (d *Data) NodeFor(c korrel8r.Class) *Node {
	if id, ok := d.nodeID[c]; ok {
		return d.Nodes[id]
	}
	id := int64(len(d.Nodes))
	n := &Node{
		Node:        multi.Node(id),
		Class:       c,
		Attrs:       Attrs{},
		Result:      korrel8r.NewResult(c),
		QueryCounts: QueryCounts{},
	}
	d.Nodes = append(d.Nodes, n)
	d.nodeID[c] = id
	return n
}

// EmptyGraph returns a new emptpy graph based on this Data.
func (d *Data) EmptyGraph() *Graph { return New(d) }

// NewGraph returns a new graph of all the Data.
func (d *Data) NewGraph() *Graph {
	g := New(d)
	for _, l := range d.Lines {
		g.SetLine(l)
	}
	for _, n := range d.Nodes {
		if nn := g.Node(n.ID()); nn == nil {
			g.AddNode(n)
		} else if nn != n {
			panic(fmt.Errorf("invalid node %v, already have %v", n, nn))
		}
	}
	return g
}

func (d *Data) Rules() []korrel8r.Rule {
	var rules []korrel8r.Rule
	for _, l := range d.Lines {
		rules = append(rules, l.Rule)
	}
	return rules
}

func (d *Data) Classes() []korrel8r.Class {
	var classs []korrel8r.Class
	for _, n := range d.Nodes {
		classs = append(classs, n.Class)
	}
	return classs
}

// Node is a graph Node, corresponds to a Class.
type Node struct {
	multi.Node
	Attrs       // GraphViz Attributer
	Class       korrel8r.Class
	Result      korrel8r.Result // Accumulate query results.
	QueryCounts QueryCounts     // All queries leading to this node.
}

func ClassFor(n graph.Node) korrel8r.Class { return n.(*Node).Class }

func (n *Node) String() string { return korrel8r.ClassName(n.Class) }
func (n *Node) DOTID() string  { return n.String() }

// Line is one line in a multi-graph edge, corresponds to a rule.
type Line struct {
	multi.Line
	Attrs       // GraphViz Attributer
	Rule        korrel8r.Rule
	QueryCounts QueryCounts // Queries generated by Rule
}

func (l *Line) DOTID() string            { return l.Rule.String() }
func RuleFor(l graph.Line) korrel8r.Rule { return l.(*Line).Rule }

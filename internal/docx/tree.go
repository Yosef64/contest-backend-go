// Package docx turns a Word (.docx) course chapter into lessons the app
// shows: text with its formatting and colours, topics (h2) and sub-headings
// (h3) recognised from how the author formatted them, lists, tables, Word
// equations as LaTeX (data-tex), diagrams drawn with Word shapes as SVG, and
// pictures (stored through a callback). A "CHAPTER ..." line starts a new
// lesson, so one file can hold several chapters.
//
// scripts/docx_to_lesson.py is the same converter as a command-line tool.
package docx

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// Namespaces used in Word documents.
const (
	nsW   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsM   = "http://schemas.openxmlformats.org/officeDocument/2006/math"
	nsWP  = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	nsA   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsWPS = "http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
	nsMC  = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsPic = "http://schemas.openxmlformats.org/drawingml/2006/picture"
	nsR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

var prefixes = map[string]string{
	"w": nsW, "m": nsM, "wp": nsWP, "a": nsA, "wps": nsWPS, "mc": nsMC, "pic": nsPic, "r": nsR,
}

// node is one XML element with its children and text.
type node struct {
	space, local string
	attrs        map[string]string // "space local" -> value
	kids         []*node
	text         string
}

func parse(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	root := &node{}
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{space: t.Name.Space, local: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[a.Name.Space+" "+a.Name.Local] = a.Value
			}
			top := stack[len(stack)-1]
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			top := stack[len(stack)-1]
			top.text += string(t)
		}
	}
	return root, nil
}

// is reports whether the node is prefix:local, e.g. n.is("w:p").
func (n *node) is(name string) bool {
	if n == nil {
		return false
	}
	p, l, _ := strings.Cut(name, ":")
	return n.space == prefixes[p] && n.local == l
}

// attr reads an attribute by prefixed name ("w:val"); unprefixed names
// ("cx") have no namespace.
func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	if p, l, ok := strings.Cut(name, ":"); ok {
		return n.attrs[prefixes[p]+" "+l]
	}
	return n.attrs[" "+name]
}

func (n *node) has(name string) bool {
	if n == nil {
		return false
	}
	if p, l, ok := strings.Cut(name, ":"); ok {
		_, found := n.attrs[prefixes[p]+" "+l]
		return found
	}
	_, found := n.attrs[" "+name]
	return found
}

// child is the first direct child with this name; a path "w:pPr/w:spacing"
// walks children.
func (n *node) child(path string) *node {
	cur := n
	for _, part := range strings.Split(path, "/") {
		if cur == nil {
			return nil
		}
		var next *node
		for _, k := range cur.kids {
			if k.is(part) {
				next = k
				break
			}
		}
		cur = next
	}
	return cur
}

func (n *node) children(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for _, k := range n.kids {
		if k.is(name) {
			out = append(out, k)
		}
	}
	return out
}

// walk visits the node and all descendants, depth first. Returning false
// from fn skips a node's children.
func (n *node) walk(fn func(*node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, k := range n.kids {
		k.walk(fn)
	}
}

// find is the first descendant with this name (not the node itself).
func (n *node) find(name string) *node {
	var found *node
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		k.walk(func(x *node) bool {
			if found != nil {
				return false
			}
			if x.is(name) {
				found = x
				return false
			}
			return true
		})
		if found != nil {
			break
		}
	}
	return found
}

// all lists descendants with this name (not the node itself).
func (n *node) all(name string) []*node {
	var out []*node
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		k.walk(func(x *node) bool {
			if x.is(name) {
				out = append(out, x)
			}
			return true
		})
	}
	return out
}

// texts joins the text of w:t descendants (or m:t for equations).
func (n *node) texts(name string) string {
	var b strings.Builder
	for _, t := range n.all(name) {
		b.WriteString(t.text)
	}
	if n.is(name) {
		return n.text + b.String()
	}
	return b.String()
}

// val is the w:val (or m:val) attribute of a child, "" when absent.
func (n *node) val(path string) string {
	c := n.child(path)
	if c == nil {
		return ""
	}
	if v := c.attr("w:val"); v != "" {
		return v
	}
	return c.attr("m:val")
}

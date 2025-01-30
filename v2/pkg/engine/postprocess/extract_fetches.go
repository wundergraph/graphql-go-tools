package postprocess

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type extractor struct {
	fetches             []*resolve.FetchItem
	currentFetchPath    []resolve.FetchItemPathElement
	currentResponsePath []string
	info                *resolve.GraphQLResponseInfo
}

func (e *extractor) extractFetches(res *resolve.GraphQLResponse) []*resolve.FetchItem {
	e.traverseNode(res.Data)
	return e.fetches
}

func (e *extractor) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		hasPath := len(n.Path) > 0
		if hasPath {
			e.currentFetchPath = append(e.currentFetchPath, resolve.FetchItemPathElement{
				Kind: resolve.FetchItemPathElementKindObject,
				Path: n.Path,
			})
		}
		e.collectFetches(n.Fetches)
		n.Fetches = nil
		for i := range n.Fields {
			e.currentResponsePath = append(e.currentResponsePath, string(n.Fields[i].Name))
			e.traverseNode(n.Fields[i].Value)
			e.currentResponsePath = e.currentResponsePath[:len(e.currentResponsePath)-1]
		}
		if hasPath {
			e.currentFetchPath = e.currentFetchPath[:len(e.currentFetchPath)-1]
		}
	case *resolve.Array:
		e.currentFetchPath = append(e.currentFetchPath, resolve.FetchItemPathElement{
			Kind: resolve.FetchItemPathElementKindArray,
			Path: n.Path,
		})
		e.currentResponsePath = append(e.currentResponsePath, "@")
		e.traverseNode(n.Item)
		e.currentResponsePath = e.currentResponsePath[:len(e.currentResponsePath)-1]
		e.currentFetchPath = e.currentFetchPath[:len(e.currentFetchPath)-1]
	}
}

func (e *extractor) fetchPath() []resolve.FetchItemPathElement {
	hasPathElements := false
	for i := range e.currentFetchPath {
		if len(e.currentFetchPath[i].Path) > 0 {
			hasPathElements = true
			break
		}
	}
	if !hasPathElements {
		return nil
	}
	path := make([]resolve.FetchItemPathElement, len(e.currentFetchPath))
	copy(path, e.currentFetchPath)
	return path
}

func (e *extractor) responsePath() string {
	sb := &strings.Builder{}
	if len(e.currentResponsePath) > 0 {
		for i := range e.currentResponsePath {
			if i == len(e.currentResponsePath)-1 && e.currentResponsePath[i] == "@" {
				continue
			}
			if i > 0 {
				sb.WriteRune('.')
			}
			sb.WriteString(e.currentResponsePath[i])
		}
	}
	return sb.String()
}

func (e *extractor) collectFetches(fetches []resolve.Fetch) {
	path := e.fetchPath()
	var responsePathElements []string
	if len(e.currentResponsePath) > 0 {
		// remove the trailing @
		if e.currentResponsePath[len(e.currentResponsePath)-1] == "@" {
			if len(e.currentResponsePath) > 1 {
				responsePathElements = make([]string, len(e.currentResponsePath)-1)
				copy(responsePathElements, e.currentResponsePath[:len(e.currentResponsePath)-1])
			}
		} else {
			responsePathElements = make([]string, len(e.currentResponsePath))
			copy(responsePathElements, e.currentResponsePath)
		}
	}
	for i := range fetches {
		switch f := fetches[i].(type) {
		case *resolve.SingleFetch:
			item := &resolve.FetchItem{
				Fetch:                f,
				FetchPath:            path,
				ResponsePath:         e.responsePath(),
				ResponsePathElements: responsePathElements,
			}
			e.fetches = append(e.fetches, item)
		}
	}
}

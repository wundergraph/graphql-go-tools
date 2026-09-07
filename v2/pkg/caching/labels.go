package caching

import (
	"sort"
	"strings"
)

// Labels are what a client response carries so a CDN can purge by them. Stored
// with the entry, unlike Tags which only name an index.
const (
	subgraphLabelPrefix = "subgraph-"
	typeLabelPrefix     = "type-"
)

func SubgraphLabel(subgraph string) string {
	return subgraphLabelPrefix + subgraph
}

func TypeLabel(subgraph, typeName string) string {
	return typeLabelPrefix + subgraph + "-" + typeName
}

// labelTier: coarsest first, so a size limit cuts the finest purge surface.
func labelTier(label string) int {
	switch {
	case strings.HasPrefix(label, subgraphLabelPrefix):
		return 0
	case strings.HasPrefix(label, typeLabelPrefix):
		return 1
	default:
		return 2
	}
}

// SortLabels orders by tier then lexically, so the same set always packs the same.
func SortLabels(labels []string) {
	sort.SliceStable(labels, func(i, j int) bool {
		ti, tj := labelTier(labels[i]), labelTier(labels[j])
		if ti != tj {
			return ti < tj
		}
		return labels[i] < labels[j]
	})
}

// MergeLabels appends every label of src not already in dst, first seen order.
func MergeLabels(dst []string, src ...[]string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, label := range dst {
		seen[label] = struct{}{}
	}
	for _, labels := range src {
		for _, label := range labels {
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			dst = append(dst, label)
		}
	}
	return dst
}

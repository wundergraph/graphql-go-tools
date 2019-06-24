//go:generate genny -in=$GOFILE -out=gen-$GOFILE gen "Node=Directive,FieldDefinition,RootOperationTypeDefinition,Argument,Type,InputValueDefinition,EnumValueDefinition,Value"
package ast

import (
	"github.com/cheekybits/genny/generic"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

type Node generic.Type

type NodeList struct {
	Open          position.Position
	Close         position.Position
	current       Node
	currentRef    int
	nextRef       int
	isInitialized bool
}

type NodeGetter interface {
	GetNode(ref int) (node Node, nextRef int)
}

func NewNodeList(first int) NodeList {
	nodeList := NodeList{}
	nodeList.SetFirst(first)
	return nodeList
}

func (n *NodeList) SetFirst(first int) {
	n.nextRef = first
	n.isInitialized = first != -1
}

func (n *NodeList) HasNext() bool {
	return n.isInitialized && n.nextRef != -1
}

func (n *NodeList) Next(getter NodeGetter) bool {
	if !n.isInitialized || n.nextRef == -1 {
		return false
	}
	n.currentRef = n.nextRef
	n.current, n.nextRef = getter.GetNode(n.nextRef)
	return true
}

func (n *NodeList) Value() (Node, int) {
	return n.current, n.currentRef
}

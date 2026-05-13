package planbytecode

import (
	"fmt"
	"strconv"
)

type Opcode uint32

const (
	OpNop Opcode = iota
	OpEnterSequence
	OpLeaveSequence
	OpEnterParallel
	OpLeaveParallel
	OpFetchSubgraph
	OpPasteAtPointer
	OpEnterObject
	OpLeaveObject
	OpEnterArray
	OpLeaveArray
	OpProjectField
	OpEmitLiteral
	OpEmitResponse
)

func (o Opcode) String() string {
	switch o {
	case OpNop:
		return "nop"
	case OpEnterSequence:
		return "enter_sequence"
	case OpLeaveSequence:
		return "leave_sequence"
	case OpEnterParallel:
		return "enter_parallel"
	case OpLeaveParallel:
		return "leave_parallel"
	case OpFetchSubgraph:
		return "fetch_subgraph"
	case OpPasteAtPointer:
		return "paste_at_pointer"
	case OpEnterObject:
		return "enter_object"
	case OpLeaveObject:
		return "leave_object"
	case OpEnterArray:
		return "enter_array"
	case OpLeaveArray:
		return "leave_array"
	case OpProjectField:
		return "project_field"
	case OpEmitLiteral:
		return "emit_literal"
	case OpEmitResponse:
		return "emit_response"
	default:
		return fmt.Sprintf("opcode_%d", o)
	}
}

// Op is intentionally fixed-width. The operands index into side tables on Program.
type Op struct {
	Code Opcode
	A    uint32
	B    uint32
	C    uint32
}

type Program struct {
	Ops           []Op
	Strings       []string
	QuotedStrings []string
	Paths         [][]string
	Fetches       []Fetch

	DirectResponse *DirectResponse

	Stats       Stats
	Unsupported []UnsupportedFeature
}

func (p *Program) FastPathReady() bool {
	return p != nil && len(p.Unsupported) == 0
}

func (p *Program) DirectResponseReady() bool {
	return p != nil && p.DirectResponse != nil && len(
		p.DirectResponse.Fields,
	) != 0 && len(p.Unsupported) == 0
}

func (p *Program) QuotedString(ref uint32) (string, bool) {
	if p == nil || int(ref) >= len(p.Strings) {
		return "", false
	}

	if int(ref) < len(p.QuotedStrings) && p.QuotedStrings[ref] != "" {
		return p.QuotedStrings[ref], true
	}

	return strconv.Quote(p.Strings[ref]), true
}

type Stats struct {
	Fetches        int
	DCEFetches     int
	Objects        int
	Arrays         int
	Fields         int
	Literals       int
	ResponseNodes  int
	UnsupportedOps int
}

type UnsupportedFeature struct {
	Feature string
	Reason  string
}

type Fetch struct {
	Kind                        uint32
	DataSourceIDRef             uint32
	DataSourceNameRef           uint32
	ResponsePathRef             uint32
	SelectResponseDataPathRef   uint32
	SelectResponseErrorsPathRef uint32
	MergePathRef                uint32
	DependsOnFetchIDs           []int

	// Item is intentionally opaque to keep this IR package independent from
	// resolve. The engine compiler stores *resolve.FetchItem here, and the
	// resolve-owned interpreter type-asserts it back inside the resolve package.
	Item any `json:"-"`
}

type DirectResponse struct {
	Fields []DirectField
}

type DirectField struct {
	NameRef    uint32
	PathRef    uint32
	LiteralRef uint32
	Flags      uint32
	ItemFlags  uint32
	Children   []DirectField
}

const (
	DirectFieldFlagLiteral uint32 = 1 << 31
	DirectFieldKindMask    uint32 = 0x0000ffff
	DirectFieldNullable    uint32 = 1 << 16
)

func EncodeDirectFieldFlags(kind uint32, nullable bool, literal bool) uint32 {
	flags := kind & DirectFieldKindMask
	if nullable {
		flags |= DirectFieldNullable
	}
	if literal {
		flags |= DirectFieldFlagLiteral
	}
	return flags
}

func DirectFieldKind(flags uint32) uint32 {
	return flags & DirectFieldKindMask
}

func DirectFieldIsNullable(flags uint32) bool {
	return flags&DirectFieldNullable != 0
}

func DirectFieldIsLiteral(flags uint32) bool {
	return flags&DirectFieldFlagLiteral != 0
}

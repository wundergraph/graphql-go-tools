// Package pgvector implements the searchindex.Index and searchindex.IndexFactory
// interfaces for PostgreSQL with pgvector.
//
// It uses only database/sql from the standard library -- no external pgvector
// SDK or driver is imported. The caller is responsible for registering a
// PostgreSQL driver (e.g. "github.com/lib/pq") and providing an open *sql.DB.
//
// Supports: vector search (pgvector <=> operator), full-text (tsvector/tsquery),
// CTE-based hybrid search with Reciprocal Rank Fusion (RRF).
// Filter translation: searchindex.Filter -> SQL WHERE clauses with parameterized queries.
package pgvector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// Compile-time interface conformance checks.
var (
	_ searchindex.Index        = (*Index)(nil)
	_ searchindex.IndexFactory = (*Factory)(nil)
)

// defaultTablePrefix is used when Config.TablePrefix is empty.
const defaultTablePrefix = "searchindex_"

// defaultRRFConstant is the k parameter in Reciprocal Rank Fusion: 1/(k+rank).
const defaultRRFConstant = 60

// Config holds pgvector-specific configuration.
type Config struct {
	TablePrefix string `json:"table_prefix,omitempty"`
}

// Factory implements searchindex.IndexFactory for pgvector.
// It holds a reference to a *sql.DB that must already be connected with a
// PostgreSQL driver registered by the caller.
type Factory struct {
	DB *sql.DB
}

// NewFactory returns a new pgvector IndexFactory backed by the given database
// connection. The caller is responsible for importing a PostgreSQL driver
// (e.g. _ "github.com/lib/pq") and opening the *sql.DB.
func NewFactory(db *sql.DB) *Factory {
	return &Factory{DB: db}
}

// CreateIndex creates a new index backed by a PostgreSQL table. It executes
// DDL statements to create the pgvector extension (if not present), the table,
// and appropriate indexes (GIN for tsvector, HNSW for vector columns, B-tree
// for filterable scalar columns).
func (f *Factory) CreateIndex(ctx context.Context, name string, schema searchindex.IndexConfig, configJSON []byte) (searchindex.Index, error) {
	var cfg Config
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("pgvector: invalid config: %w", err)
		}
	}

	prefix := cfg.TablePrefix
	if prefix == "" {
		prefix = defaultTablePrefix
	}

	tableName := prefix + sanitizeIdentifier(name)

	idx := &Index{
		db:        f.DB,
		name:      name,
		tableName: tableName,
		schema:    schema,
		prefix:    prefix,
	}

	// Classify fields by type for later use.
	idx.classifyFields()

	if err := idx.createTable(ctx); err != nil {
		return nil, err
	}

	return idx, nil
}

// Index implements searchindex.Index backed by a PostgreSQL table with pgvector.
type Index struct {
	db        *sql.DB
	name      string
	tableName string
	schema    searchindex.IndexConfig
	prefix    string

	// Cached field classifications.
	textFields    []string            // field names of type TEXT
	vectorFields  map[string]int      // field name -> dimensions
	allFieldNames []string            // all user-defined field names in schema order
	fieldTypes    map[string]fieldDef // field name -> definition
}

// fieldDef stores the type information for a single field.
type fieldDef struct {
	config searchindex.FieldConfig
	colSQL string // SQL column type
}

// classifyFields pre-computes field classifications from the schema.
func (idx *Index) classifyFields() {
	idx.vectorFields = make(map[string]int)
	idx.fieldTypes = make(map[string]fieldDef, len(idx.schema.Fields))

	for _, fc := range idx.schema.Fields {
		colSQL := fieldColumnType(fc)
		idx.fieldTypes[fc.Name] = fieldDef{config: fc, colSQL: colSQL}
		idx.allFieldNames = append(idx.allFieldNames, fc.Name)

		switch fc.Type {
		case searchindex.FieldTypeText:
			idx.textFields = append(idx.textFields, fc.Name)
		case searchindex.FieldTypeVector:
			idx.vectorFields[fc.Name] = fc.Dimensions
		}
	}
}

// fieldColumnType returns the SQL column type string for a FieldConfig.
func fieldColumnType(fc searchindex.FieldConfig) string {
	switch fc.Type {
	case searchindex.FieldTypeText:
		return "TEXT"
	case searchindex.FieldTypeKeyword:
		return "TEXT"
	case searchindex.FieldTypeNumeric:
		return "DOUBLE PRECISION"
	case searchindex.FieldTypeBool:
		return "BOOLEAN"
	case searchindex.FieldTypeVector:
		return fmt.Sprintf("vector(%d)", fc.Dimensions)
	case searchindex.FieldTypeGeo:
		// Store geo as JSONB for now; PostGIS support can be added later.
		return "JSONB"
	case searchindex.FieldTypeDate:
		return "DATE"
	case searchindex.FieldTypeDateTime:
		return "TIMESTAMPTZ"
	default:
		return "TEXT"
	}
}

// createTable executes DDL to create the extension, table, and indexes.
func (idx *Index) createTable(ctx context.Context) error {
	// Enable pgvector extension.
	if _, err := idx.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return fmt.Errorf("pgvector: create extension: %w", err)
	}

	// Build CREATE TABLE statement.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", quoteIdent(idx.tableName)))
	sb.WriteString("  doc_id TEXT PRIMARY KEY,\n")
	sb.WriteString("  type_name TEXT NOT NULL,\n")
	sb.WriteString("  key_fields_json TEXT NOT NULL")

	for _, fc := range idx.schema.Fields {
		def := idx.fieldTypes[fc.Name]
		sb.WriteString(fmt.Sprintf(",\n  %s %s", quoteIdent(fc.Name), def.colSQL))
	}

	// Add tsvector column for full-text search if there are text fields.
	if len(idx.textFields) > 0 {
		sb.WriteString(",\n  tsv tsvector")
	}

	sb.WriteString("\n)")

	if _, err := idx.db.ExecContext(ctx, sb.String()); err != nil {
		return fmt.Errorf("pgvector: create table: %w", err)
	}

	// Create indexes.
	if err := idx.createIndexes(ctx); err != nil {
		return err
	}

	return nil
}

// createIndexes creates supporting indexes on the table.
func (idx *Index) createIndexes(ctx context.Context) error {
	// GIN index on tsvector column for full-text search.
	if len(idx.textFields) > 0 {
		ginSQL := fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (tsv)",
			quoteIdent(idx.tableName+"_tsv_idx"),
			quoteIdent(idx.tableName),
		)
		if _, err := idx.db.ExecContext(ctx, ginSQL); err != nil {
			return fmt.Errorf("pgvector: create GIN index: %w", err)
		}
	}

	// HNSW indexes on vector columns.
	for fieldName := range idx.vectorFields {
		hnswSQL := fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s vector_cosine_ops)",
			quoteIdent(idx.tableName+"_"+fieldName+"_hnsw_idx"),
			quoteIdent(idx.tableName),
			quoteIdent(fieldName),
		)
		if _, err := idx.db.ExecContext(ctx, hnswSQL); err != nil {
			return fmt.Errorf("pgvector: create HNSW index on %s: %w", fieldName, err)
		}
	}

	// B-tree indexes on filterable/sortable scalar columns.
	for _, fc := range idx.schema.Fields {
		if fc.Type == searchindex.FieldTypeVector {
			continue
		}
		if fc.Filterable || fc.Sortable {
			btreeSQL := fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
				quoteIdent(idx.tableName+"_"+fc.Name+"_idx"),
				quoteIdent(idx.tableName),
				quoteIdent(fc.Name),
			)
			if _, err := idx.db.ExecContext(ctx, btreeSQL); err != nil {
				return fmt.Errorf("pgvector: create B-tree index on %s: %w", fc.Name, err)
			}
		}
	}

	// B-tree index on type_name for type filtering.
	typeIdx := fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s (type_name)",
		quoteIdent(idx.tableName+"_type_name_idx"),
		quoteIdent(idx.tableName),
	)
	if _, err := idx.db.ExecContext(ctx, typeIdx); err != nil {
		return fmt.Errorf("pgvector: create type_name index: %w", err)
	}

	return nil
}

// documentID computes a deterministic string ID from a DocumentIdentity.
// Format: TypeName:key1=val1,key2=val2,... (keys sorted alphabetically).
// This matches the convention used by the Bleve backend.
func documentID(id searchindex.DocumentIdentity) string {
	if len(id.KeyFields) == 0 {
		return id.TypeName
	}
	keys := make([]string, 0, len(id.KeyFields))
	for k := range id.KeyFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(id.TypeName)
	b.WriteByte(':')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		fmt.Fprintf(&b, "%v", id.KeyFields[k])
	}
	return b.String()
}

// IndexDocument indexes a single document.
func (idx *Index) IndexDocument(ctx context.Context, doc searchindex.EntityDocument) error {
	return idx.IndexDocuments(ctx, []searchindex.EntityDocument{doc})
}

// IndexDocuments indexes a batch of documents using upserts.
func (idx *Index) IndexDocuments(ctx context.Context, docs []searchindex.EntityDocument) error {
	if len(docs) == 0 {
		return nil
	}

	tx, err := idx.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pgvector: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, doc := range docs {
		if err := idx.upsertDocument(ctx, tx, doc); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("pgvector: commit: %w", err)
	}
	return nil
}

// upsertDocument performs INSERT ... ON CONFLICT DO UPDATE for a single document.
func (idx *Index) upsertDocument(ctx context.Context, tx *sql.Tx, doc searchindex.EntityDocument) error {
	docID := documentID(doc.Identity)

	keyFieldsJSON, err := json.Marshal(doc.Identity.KeyFields)
	if err != nil {
		return fmt.Errorf("pgvector: marshal key fields: %w", err)
	}

	// Build column list and value placeholders.
	columns := []string{"doc_id", "type_name", "key_fields_json"}
	args := []any{docID, doc.Identity.TypeName, string(keyFieldsJSON)}
	paramIdx := 4 // next parameter index

	for _, fieldName := range idx.allFieldNames {
		columns = append(columns, quoteIdent(fieldName))

		def := idx.fieldTypes[fieldName]
		if def.config.Type == searchindex.FieldTypeVector {
			// Vector field: format as pgvector string.
			if vec, ok := doc.Vectors[fieldName]; ok {
				args = append(args, formatVector(vec))
			} else {
				args = append(args, nil)
			}
		} else {
			// Scalar field.
			if val, ok := doc.Fields[fieldName]; ok {
				args = append(args, val)
			} else {
				args = append(args, nil)
			}
		}
		paramIdx++
	}

	// Add tsvector column if there are text fields.
	hasTSV := len(idx.textFields) > 0
	if hasTSV {
		columns = append(columns, "tsv")
		// Build tsvector from text fields.
		tsvParts := make([]string, 0, len(idx.textFields))
		for _, tf := range idx.textFields {
			if val, ok := doc.Fields[tf]; ok {
				tsvParts = append(tsvParts, fmt.Sprintf("%v", val))
			}
		}
		tsvText := strings.Join(tsvParts, " ")
		args = append(args, tsvText)
	}

	// Build placeholders.
	placeholders := make([]string, len(args))
	for i := range args {
		if i < 3 {
			// First 3 are plain columns.
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		} else if hasTSV && i == len(args)-1 {
			// Last arg is the tsvector text; wrap in to_tsvector.
			placeholders[i] = fmt.Sprintf("to_tsvector('english', $%d)", i+1)
		} else {
			// Check if this is a vector field.
			fieldIdx := i - 3 // index into allFieldNames
			if fieldIdx >= 0 && fieldIdx < len(idx.allFieldNames) {
				def := idx.fieldTypes[idx.allFieldNames[fieldIdx]]
				if def.config.Type == searchindex.FieldTypeVector {
					placeholders[i] = fmt.Sprintf("$%d::vector", i+1)
				} else {
					placeholders[i] = fmt.Sprintf("$%d", i+1)
				}
			} else {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
			}
		}
	}

	// Build UPDATE SET clause (exclude doc_id).
	setClauses := make([]string, 0, len(columns)-1)
	for i := 1; i < len(columns); i++ {
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", columns[i], columns[i]))
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (doc_id) DO UPDATE SET %s",
		quoteIdent(idx.tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(setClauses, ", "),
	)

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("pgvector: upsert document %q: %w", docID, err)
	}
	return nil
}

// formatVector formats a float32 slice as a pgvector literal string, e.g. "[0.1,0.2,0.3]".
func formatVector(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", v)
	}
	sb.WriteByte(']')
	return sb.String()
}

// DeleteDocument deletes a single document by identity.
func (idx *Index) DeleteDocument(ctx context.Context, id searchindex.DocumentIdentity) error {
	return idx.DeleteDocuments(ctx, []searchindex.DocumentIdentity{id})
}

// DeleteDocuments deletes a batch of documents by identity.
func (idx *Index) DeleteDocuments(ctx context.Context, ids []searchindex.DocumentIdentity) error {
	if len(ids) == 0 {
		return nil
	}

	if len(ids) == 1 {
		docID := documentID(ids[0])
		query := fmt.Sprintf("DELETE FROM %s WHERE doc_id = $1", quoteIdent(idx.tableName))
		if _, err := idx.db.ExecContext(ctx, query, docID); err != nil {
			return fmt.Errorf("pgvector: delete document %q: %w", docID, err)
		}
		return nil
	}

	// Batch delete using ANY.
	docIDs := make([]string, len(ids))
	for i, id := range ids {
		docIDs[i] = documentID(id)
	}

	// Build an array literal for use with ANY.
	query := fmt.Sprintf("DELETE FROM %s WHERE doc_id = ANY($1::text[])", quoteIdent(idx.tableName))
	arrayLiteral := formatTextArray(docIDs)
	if _, err := idx.db.ExecContext(ctx, query, arrayLiteral); err != nil {
		return fmt.Errorf("pgvector: batch delete: %w", err)
	}
	return nil
}

// formatTextArray formats a string slice as a PostgreSQL text array literal.
func formatTextArray(vals []string) string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		// Escape backslashes and double quotes.
		for _, c := range v {
			if c == '\\' || c == '"' {
				sb.WriteByte('\\')
			}
			sb.WriteRune(c)
		}
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	return sb.String()
}

// Search performs a search query and returns results.
func (idx *Index) Search(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	hasText := req.TextQuery != ""
	hasVector := len(req.Vector) > 0

	switch {
	case hasText && hasVector:
		return idx.hybridSearch(ctx, req)
	case hasVector:
		return idx.vectorSearch(ctx, req)
	default:
		// Text-only or filter-only search.
		return idx.textSearch(ctx, req)
	}
}

// textSearch handles text-only and filter-only queries.
func (idx *Index) textSearch(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	var (
		qb      queryBuilder
		selCols []string
		orderBy []string
		hasText = req.TextQuery != ""
	)

	// SELECT clause.
	selCols = append(selCols, "doc_id", "type_name", "key_fields_json")
	for _, fn := range idx.allFieldNames {
		def := idx.fieldTypes[fn]
		if def.config.Type == searchindex.FieldTypeVector {
			continue // Don't select vector columns in text results.
		}
		selCols = append(selCols, quoteIdent(fn))
	}

	if hasText {
		tsvQuery := idx.buildTSVQuery(req)
		p := qb.addParam(req.TextQuery)
		selCols = append(selCols, fmt.Sprintf("ts_rank(tsv, %s) AS score", tsvQuery))
		qb.addWhere(fmt.Sprintf("tsv @@ %s", tsvQuery))
		_ = p // param already added
		if len(req.Sort) == 0 {
			orderBy = append(orderBy, "score DESC")
		}
	} else {
		selCols = append(selCols, "0::float AS score")
	}

	// Type filter.
	if req.TypeName != "" {
		p := qb.addParam(req.TypeName)
		qb.addWhere(fmt.Sprintf("type_name = %s", p))
	}

	// Structured filter.
	if req.Filter != nil {
		filterSQL, err := idx.translateFilter(req.Filter, &qb)
		if err != nil {
			return nil, err
		}
		if filterSQL != "" {
			qb.addWhere(filterSQL)
		}
	}

	// Sorting.
	isCursorMode := len(req.SearchAfter) > 0 || len(req.SearchBefore) > 0
	isBackward := len(req.SearchBefore) > 0

	for _, sf := range req.Sort {
		dir := "ASC"
		if !sf.Ascending {
			dir = "DESC"
		}
		if isBackward {
			if dir == "ASC" {
				dir = "DESC"
			} else {
				dir = "ASC"
			}
		}
		orderBy = append(orderBy, fmt.Sprintf("%s %s", quoteIdent(sf.Field), dir))
	}

	// Cursor-based keyset WHERE clause.
	if isCursorMode && len(req.Sort) > 0 {
		cursorVals := req.SearchAfter
		if isBackward {
			cursorVals = req.SearchBefore
		}
		if len(cursorVals) > 0 {
			op := ">"
			if isBackward {
				op = "<"
			}
			p := qb.addParam(cursorVals[0])
			qb.addWhere(fmt.Sprintf("%s %s %s", quoteIdent(req.Sort[0].Field), op, p))
		}
	}

	// Build the main query.
	mainQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selCols, ", "), quoteIdent(idx.tableName))
	if len(qb.wheres) > 0 {
		mainQuery += " WHERE " + strings.Join(qb.wheres, " AND ")
	}
	if len(orderBy) > 0 {
		mainQuery += " ORDER BY " + strings.Join(orderBy, ", ")
	}

	// Count query.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(idx.tableName))
	if len(qb.wheres) > 0 {
		countQuery += " WHERE " + strings.Join(qb.wheres, " AND ")
	}

	// Pagination.
	limit := effectiveLimit(req.Limit)
	mainQuery += fmt.Sprintf(" LIMIT %d", limit)
	if !isCursorMode && req.Offset > 0 {
		mainQuery += fmt.Sprintf(" OFFSET %d", req.Offset)
	}

	// Execute count query.
	var totalCount int
	if err := idx.db.QueryRowContext(ctx, countQuery, qb.params...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("pgvector: count query: %w", err)
	}

	// Execute main query.
	rows, err := idx.db.QueryContext(ctx, mainQuery, qb.params...)
	if err != nil {
		return nil, fmt.Errorf("pgvector: search query: %w", err)
	}
	defer rows.Close()

	hits, err := idx.scanHits(rows)
	if err != nil {
		return nil, err
	}

	// Populate SortValues from sort fields for cursor-based pagination.
	if len(req.Sort) > 0 {
		for i := range hits {
			sortVals := make([]string, 0, len(req.Sort))
			for _, sf := range req.Sort {
				if v, ok := hits[i].Representation[sf.Field]; ok {
					sortVals = append(sortVals, fmt.Sprintf("%v", v))
				}
			}
			hits[i].SortValues = sortVals
		}
	}

	result := &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: totalCount,
	}

	// Facets.
	if len(req.Facets) > 0 {
		facets, err := idx.executeFacets(ctx, req, &qb)
		if err != nil {
			return nil, err
		}
		result.Facets = facets
	}

	return result, nil
}

// buildTSVQuery builds the tsquery expression based on text fields configuration.
func (idx *Index) buildTSVQuery(req searchindex.SearchRequest) string {
	return "plainto_tsquery('english', $1)"
}

// vectorSearch handles vector-only queries using the <=> distance operator.
func (idx *Index) vectorSearch(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	vectorField := req.VectorField
	if vectorField == "" {
		// Use first vector field if not specified.
		for fn := range idx.vectorFields {
			vectorField = fn
			break
		}
	}
	if vectorField == "" {
		return nil, fmt.Errorf("pgvector: no vector field available for vector search")
	}

	var (
		qb      queryBuilder
		selCols []string
		orderBy []string
	)

	// Format vector as a SQL literal rather than a parameter placeholder.
	// pgvector values must be inlined because lib/pq cannot determine the
	// type of a $N placeholder used with the <=> operator, and count queries
	// that don't reference the vector column would receive extra parameters.
	vecLiteral := fmt.Sprintf("'%s'::vector", formatVector(req.Vector))

	selCols = append(selCols, "doc_id", "type_name", "key_fields_json")
	for _, fn := range idx.allFieldNames {
		def := idx.fieldTypes[fn]
		if def.config.Type == searchindex.FieldTypeVector {
			continue
		}
		selCols = append(selCols, quoteIdent(fn))
	}
	selCols = append(selCols, fmt.Sprintf("%s <=> %s AS distance", quoteIdent(vectorField), vecLiteral))

	// Type filter.
	if req.TypeName != "" {
		p := qb.addParam(req.TypeName)
		qb.addWhere(fmt.Sprintf("type_name = %s", p))
	}

	// Structured filter.
	if req.Filter != nil {
		filterSQL, err := idx.translateFilter(req.Filter, &qb)
		if err != nil {
			return nil, err
		}
		if filterSQL != "" {
			qb.addWhere(filterSQL)
		}
	}

	// For vector search, default order is by distance ASC.
	if len(req.Sort) == 0 {
		orderBy = append(orderBy, "distance ASC")
	} else {
		for _, sf := range req.Sort {
			dir := "ASC"
			if !sf.Ascending {
				dir = "DESC"
			}
			orderBy = append(orderBy, fmt.Sprintf("%s %s", quoteIdent(sf.Field), dir))
		}
	}

	mainQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selCols, ", "), quoteIdent(idx.tableName))
	if len(qb.wheres) > 0 {
		mainQuery += " WHERE " + strings.Join(qb.wheres, " AND ")
	}
	mainQuery += " ORDER BY " + strings.Join(orderBy, ", ")

	// Count query.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(idx.tableName))
	if len(qb.wheres) > 0 {
		countQuery += " WHERE " + strings.Join(qb.wheres, " AND ")
	}

	limit := effectiveLimit(req.Limit)
	mainQuery += fmt.Sprintf(" LIMIT %d", limit)
	if req.Offset > 0 {
		mainQuery += fmt.Sprintf(" OFFSET %d", req.Offset)
	}

	var totalCount int
	if err := idx.db.QueryRowContext(ctx, countQuery, qb.params...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("pgvector: vector count query: %w", err)
	}

	rows, err := idx.db.QueryContext(ctx, mainQuery, qb.params...)
	if err != nil {
		return nil, fmt.Errorf("pgvector: vector search query: %w", err)
	}
	defer rows.Close()

	hits, err := idx.scanVectorHits(rows)
	if err != nil {
		return nil, err
	}

	result := &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: totalCount,
	}

	if len(req.Facets) > 0 {
		facets, err := idx.executeFacets(ctx, req, &qb)
		if err != nil {
			return nil, err
		}
		result.Facets = facets
	}

	return result, nil
}

// hybridSearch performs CTE-based Reciprocal Rank Fusion combining text and vector results.
func (idx *Index) hybridSearch(ctx context.Context, req searchindex.SearchRequest) (*searchindex.SearchResult, error) {
	vectorField := req.VectorField
	if vectorField == "" {
		for fn := range idx.vectorFields {
			vectorField = fn
			break
		}
	}
	if vectorField == "" {
		return nil, fmt.Errorf("pgvector: no vector field available for hybrid search")
	}

	var qb queryBuilder
	limit := effectiveLimit(req.Limit)
	k := defaultRRFConstant

	// Parameter for text query.
	textParam := qb.addParam(req.TextQuery)
	// Format vector as a SQL literal rather than a parameter placeholder.
	// pgvector values must be inlined because lib/pq cannot determine the
	// type of a $N placeholder used with the <=> operator, and count queries
	// that don't reference the vector column would receive extra parameters.
	vecLiteral := fmt.Sprintf("'%s'::vector", formatVector(req.Vector))

	// Build filter WHERE clause (shared by both CTEs).
	var filterWhere string
	if req.TypeName != "" {
		p := qb.addParam(req.TypeName)
		filterWhere = fmt.Sprintf("type_name = %s", p)
	}
	if req.Filter != nil {
		filterSQL, err := idx.translateFilter(req.Filter, &qb)
		if err != nil {
			return nil, err
		}
		if filterSQL != "" {
			if filterWhere != "" {
				filterWhere += " AND " + filterSQL
			} else {
				filterWhere = filterSQL
			}
		}
	}

	textWhere := fmt.Sprintf("tsv @@ plainto_tsquery('english', %s)", textParam)
	if filterWhere != "" {
		textWhere += " AND " + filterWhere
	}

	vecWhere := filterWhere

	// Build the CTE-based RRF query.
	// text_results: ranked by ts_rank descending.
	// vec_results: ranked by distance ascending (closest first).
	// Combined using RRF: score = 1/(k+rank).
	//
	// We use ROW_NUMBER() for ranking within each CTE.
	rrfLimit := limit * 3 // Fetch more candidates from each CTE for better fusion.
	if rrfLimit < 100 {
		rrfLimit = 100
	}

	var sb strings.Builder
	sb.WriteString("WITH text_results AS (\n")
	sb.WriteString(fmt.Sprintf("  SELECT doc_id, ROW_NUMBER() OVER (ORDER BY ts_rank(tsv, plainto_tsquery('english', %s)) DESC) AS rank\n", textParam))
	sb.WriteString(fmt.Sprintf("  FROM %s\n", quoteIdent(idx.tableName)))
	sb.WriteString(fmt.Sprintf("  WHERE %s\n", textWhere))
	sb.WriteString(fmt.Sprintf("  LIMIT %d\n", rrfLimit))
	sb.WriteString("),\nvec_results AS (\n")
	sb.WriteString(fmt.Sprintf("  SELECT doc_id, ROW_NUMBER() OVER (ORDER BY %s <=> %s ASC) AS rank\n", quoteIdent(vectorField), vecLiteral))
	sb.WriteString(fmt.Sprintf("  FROM %s\n", quoteIdent(idx.tableName)))
	if vecWhere != "" {
		sb.WriteString(fmt.Sprintf("  WHERE %s\n", vecWhere))
	}
	sb.WriteString(fmt.Sprintf("  LIMIT %d\n", rrfLimit))
	sb.WriteString("),\ncombined AS (\n")
	sb.WriteString("  SELECT COALESCE(t.doc_id, v.doc_id) AS doc_id,\n")
	sb.WriteString(fmt.Sprintf("    COALESCE(1.0/(%d + t.rank), 0) + COALESCE(1.0/(%d + v.rank), 0) AS rrf_score\n", k, k))
	sb.WriteString("  FROM text_results t\n")
	sb.WriteString("  FULL OUTER JOIN vec_results v ON t.doc_id = v.doc_id\n")
	sb.WriteString(")\n")

	// Select combined results joined back to the main table.
	selCols := []string{"m.doc_id", "m.type_name", "m.key_fields_json"}
	for _, fn := range idx.allFieldNames {
		def := idx.fieldTypes[fn]
		if def.config.Type == searchindex.FieldTypeVector {
			continue
		}
		selCols = append(selCols, "m."+quoteIdent(fn))
	}
	selCols = append(selCols, "c.rrf_score AS score")

	sb.WriteString(fmt.Sprintf("SELECT %s\n", strings.Join(selCols, ", ")))
	sb.WriteString(fmt.Sprintf("FROM combined c JOIN %s m ON c.doc_id = m.doc_id\n", quoteIdent(idx.tableName)))

	// Sorting.
	if len(req.Sort) > 0 {
		sortClauses := make([]string, 0, len(req.Sort))
		for _, sf := range req.Sort {
			dir := "ASC"
			if !sf.Ascending {
				dir = "DESC"
			}
			sortClauses = append(sortClauses, fmt.Sprintf("m.%s %s", quoteIdent(sf.Field), dir))
		}
		sb.WriteString("ORDER BY " + strings.Join(sortClauses, ", ") + "\n")
	} else {
		sb.WriteString("ORDER BY c.rrf_score DESC\n")
	}

	sb.WriteString(fmt.Sprintf("LIMIT %d", limit))
	if req.Offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", req.Offset))
	}

	mainQuery := sb.String()

	// Count query for hybrid: count the combined CTE.
	var countSB strings.Builder
	countSB.WriteString("WITH text_results AS (\n")
	countSB.WriteString(fmt.Sprintf("  SELECT doc_id FROM %s WHERE %s LIMIT %d\n", quoteIdent(idx.tableName), textWhere, rrfLimit))
	countSB.WriteString("),\nvec_results AS (\n")
	countSB.WriteString(fmt.Sprintf("  SELECT doc_id FROM %s", quoteIdent(idx.tableName)))
	if vecWhere != "" {
		countSB.WriteString(fmt.Sprintf(" WHERE %s", vecWhere))
	}
	countSB.WriteString(fmt.Sprintf(" LIMIT %d\n", rrfLimit))
	countSB.WriteString(")\n")
	countSB.WriteString("SELECT COUNT(DISTINCT doc_id) FROM (\n")
	countSB.WriteString("  SELECT doc_id FROM text_results\n")
	countSB.WriteString("  UNION\n")
	countSB.WriteString("  SELECT doc_id FROM vec_results\n")
	countSB.WriteString(") AS all_docs")

	var totalCount int
	if err := idx.db.QueryRowContext(ctx, countSB.String(), qb.params...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("pgvector: hybrid count query: %w", err)
	}

	rows, err := idx.db.QueryContext(ctx, mainQuery, qb.params...)
	if err != nil {
		return nil, fmt.Errorf("pgvector: hybrid search query: %w", err)
	}
	defer rows.Close()

	hits, err := idx.scanHits(rows)
	if err != nil {
		return nil, err
	}

	result := &searchindex.SearchResult{
		Hits:       hits,
		TotalCount: totalCount,
	}

	if len(req.Facets) > 0 {
		facets, err := idx.executeFacets(ctx, req, &qb)
		if err != nil {
			return nil, err
		}
		result.Facets = facets
	}

	return result, nil
}

// scanHits scans rows from a text or hybrid search query into SearchHit slices.
// Expected columns: doc_id, type_name, key_fields_json, [scalar fields...], score.
func (idx *Index) scanHits(rows *sql.Rows) ([]searchindex.SearchHit, error) {
	var hits []searchindex.SearchHit

	scalarFields := idx.scalarFieldNames()

	for rows.Next() {
		var (
			docID        string
			typeName     string
			keyFieldsRaw string
			score        float64
		)

		scanDest := make([]any, 0, 3+len(scalarFields)+1)
		scanDest = append(scanDest, &docID, &typeName, &keyFieldsRaw)

		fieldPtrs := make([]*sql.NullString, len(scalarFields))
		for i := range scalarFields {
			fieldPtrs[i] = &sql.NullString{}
			scanDest = append(scanDest, fieldPtrs[i])
		}
		scanDest = append(scanDest, &score)

		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("pgvector: scan hit: %w", err)
		}

		identity, err := parseIdentity(typeName, keyFieldsRaw)
		if err != nil {
			return nil, err
		}

		representation := make(map[string]any, len(scalarFields)+2)
		representation["__typename"] = typeName
		for k, v := range identity.KeyFields {
			representation[k] = v
		}

		for i, fn := range scalarFields {
			if fieldPtrs[i].Valid {
				representation[fn] = idx.parseFieldValue(fn, fieldPtrs[i].String)
			}
		}

		hits = append(hits, searchindex.SearchHit{
			Identity:       identity,
			Score:          score,
			Representation: representation,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: rows iteration: %w", err)
	}

	if hits == nil {
		hits = []searchindex.SearchHit{}
	}

	return hits, nil
}

// scanVectorHits scans rows from a vector search query into SearchHit slices.
// Expected columns: doc_id, type_name, key_fields_json, [scalar fields...], distance.
func (idx *Index) scanVectorHits(rows *sql.Rows) ([]searchindex.SearchHit, error) {
	var hits []searchindex.SearchHit

	scalarFields := idx.scalarFieldNames()

	for rows.Next() {
		var (
			docID        string
			typeName     string
			keyFieldsRaw string
			distance     float64
		)

		scanDest := make([]any, 0, 3+len(scalarFields)+1)
		scanDest = append(scanDest, &docID, &typeName, &keyFieldsRaw)

		fieldPtrs := make([]*sql.NullString, len(scalarFields))
		for i := range scalarFields {
			fieldPtrs[i] = &sql.NullString{}
			scanDest = append(scanDest, fieldPtrs[i])
		}
		scanDest = append(scanDest, &distance)

		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("pgvector: scan vector hit: %w", err)
		}

		identity, err := parseIdentity(typeName, keyFieldsRaw)
		if err != nil {
			return nil, err
		}

		representation := make(map[string]any, len(scalarFields)+2)
		representation["__typename"] = typeName
		for k, v := range identity.KeyFields {
			representation[k] = v
		}

		for i, fn := range scalarFields {
			if fieldPtrs[i].Valid {
				representation[fn] = idx.parseFieldValue(fn, fieldPtrs[i].String)
			}
		}

		hits = append(hits, searchindex.SearchHit{
			Identity:       identity,
			Score:          1.0 / (1.0 + distance), // Convert distance to similarity score.
			Distance:       distance,
			Representation: representation,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: vector rows iteration: %w", err)
	}

	if hits == nil {
		hits = []searchindex.SearchHit{}
	}

	return hits, nil
}

// scalarFieldNames returns the names of all non-vector fields in schema order.
func (idx *Index) scalarFieldNames() []string {
	var names []string
	for _, fn := range idx.allFieldNames {
		if idx.fieldTypes[fn].config.Type != searchindex.FieldTypeVector {
			names = append(names, fn)
		}
	}
	return names
}

// parseFieldValue converts a string value from the database to the appropriate Go type
// based on the field's type definition.
func (idx *Index) parseFieldValue(fieldName string, raw string) any {
	def, ok := idx.fieldTypes[fieldName]
	if !ok {
		return raw
	}

	switch def.config.Type {
	case searchindex.FieldTypeNumeric:
		var f float64
		if _, err := fmt.Sscanf(raw, "%f", &f); err == nil {
			return f
		}
		return raw
	case searchindex.FieldTypeBool:
		switch raw {
		case "true", "t", "1":
			return true
		case "false", "f", "0":
			return false
		}
		return raw
	default:
		return raw
	}
}

// parseIdentity reconstructs a DocumentIdentity from stored fields.
func parseIdentity(typeName, keyFieldsRaw string) (searchindex.DocumentIdentity, error) {
	var keyFields map[string]any
	if keyFieldsRaw != "" {
		if err := json.Unmarshal([]byte(keyFieldsRaw), &keyFields); err != nil {
			return searchindex.DocumentIdentity{}, fmt.Errorf("pgvector: unmarshal key fields: %w", err)
		}
	}
	if keyFields == nil {
		keyFields = make(map[string]any)
	}

	return searchindex.DocumentIdentity{
		TypeName:  typeName,
		KeyFields: keyFields,
	}, nil
}

// executeFacets runs facet aggregation queries and returns the results.
func (idx *Index) executeFacets(ctx context.Context, req searchindex.SearchRequest, baseQB *queryBuilder) (map[string]searchindex.FacetResult, error) {
	facets := make(map[string]searchindex.FacetResult, len(req.Facets))

	for _, fr := range req.Facets {
		size := fr.Size
		if size <= 0 {
			size = 10
		}

		facetQuery := fmt.Sprintf(
			"SELECT %s AS val, COUNT(*) AS cnt FROM %s",
			quoteIdent(fr.Field),
			quoteIdent(idx.tableName),
		)
		if len(baseQB.wheres) > 0 {
			facetQuery += " WHERE " + strings.Join(baseQB.wheres, " AND ")
		}
		facetQuery += fmt.Sprintf(
			" GROUP BY %s ORDER BY cnt DESC LIMIT %d",
			quoteIdent(fr.Field),
			size,
		)

		rows, err := idx.db.QueryContext(ctx, facetQuery, baseQB.params...)
		if err != nil {
			return nil, fmt.Errorf("pgvector: facet query for %s: %w", fr.Field, err)
		}

		var values []searchindex.FacetValue
		for rows.Next() {
			var (
				val sql.NullString
				cnt int
			)
			if err := rows.Scan(&val, &cnt); err != nil {
				rows.Close()
				return nil, fmt.Errorf("pgvector: scan facet: %w", err)
			}
			if val.Valid {
				values = append(values, searchindex.FacetValue{
					Value: val.String,
					Count: cnt,
				})
			}
		}
		rows.Close()

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("pgvector: facet rows: %w", err)
		}

		facets[fr.Field] = searchindex.FacetResult{Values: values}
	}

	return facets, nil
}

// translateFilter recursively converts a searchindex.Filter tree to SQL WHERE
// clause fragments using parameterized queries.
func (idx *Index) translateFilter(f *searchindex.Filter, qb *queryBuilder) (string, error) {
	if f == nil {
		return "", nil
	}

	// AND
	if len(f.And) > 0 {
		parts := make([]string, 0, len(f.And))
		for _, child := range f.And {
			s, err := idx.translateFilter(child, qb)
			if err != nil {
				return "", err
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " AND ") + ")", nil
	}

	// OR
	if len(f.Or) > 0 {
		parts := make([]string, 0, len(f.Or))
		for _, child := range f.Or {
			s, err := idx.translateFilter(child, qb)
			if err != nil {
				return "", err
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " OR ") + ")", nil
	}

	// NOT
	if f.Not != nil {
		s, err := idx.translateFilter(f.Not, qb)
		if err != nil {
			return "", err
		}
		if s == "" {
			return "", nil
		}
		return fmt.Sprintf("NOT (%s)", s), nil
	}

	// Term
	if f.Term != nil {
		p := qb.addParam(f.Term.Value)
		return fmt.Sprintf("%s = %s", quoteIdent(f.Term.Field), p), nil
	}

	// Terms (IN)
	if f.Terms != nil {
		if len(f.Terms.Values) == 0 {
			return "", nil
		}
		placeholders := make([]string, len(f.Terms.Values))
		for i, v := range f.Terms.Values {
			placeholders[i] = qb.addParam(v)
		}
		return fmt.Sprintf("%s IN (%s)", quoteIdent(f.Terms.Field), strings.Join(placeholders, ", ")), nil
	}

	// Range
	if f.Range != nil {
		return idx.translateRangeFilter(f.Range, qb)
	}

	// Prefix: escape LIKE wildcards in the user value before appending %.
	if f.Prefix != nil {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(f.Prefix.Value)
		p := qb.addParam(escaped + "%")
		return fmt.Sprintf("%s LIKE %s", quoteIdent(f.Prefix.Field), p), nil
	}

	// Exists
	if f.Exists != nil {
		return fmt.Sprintf("%s IS NOT NULL", quoteIdent(f.Exists.Field)), nil
	}

	return "", nil
}

// translateRangeFilter converts a RangeFilter to SQL predicates.
func (idx *Index) translateRangeFilter(rf *searchindex.RangeFilter, qb *queryBuilder) (string, error) {
	var parts []string

	if rf.GTE != nil {
		p := qb.addParam(rf.GTE)
		parts = append(parts, fmt.Sprintf("%s >= %s", quoteIdent(rf.Field), p))
	} else if rf.HasGT && rf.GT != nil {
		p := qb.addParam(rf.GT)
		parts = append(parts, fmt.Sprintf("%s > %s", quoteIdent(rf.Field), p))
	}

	if rf.LTE != nil {
		p := qb.addParam(rf.LTE)
		parts = append(parts, fmt.Sprintf("%s <= %s", quoteIdent(rf.Field), p))
	} else if rf.HasLT && rf.LT != nil {
		p := qb.addParam(rf.LT)
		parts = append(parts, fmt.Sprintf("%s < %s", quoteIdent(rf.Field), p))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

// Autocomplete returns terms from the index matching the given prefix.
// It splits text field values into words and returns distinct words matching the prefix.
func (idx *Index) Autocomplete(ctx context.Context, req searchindex.AutocompleteRequest) (*searchindex.AutocompleteResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	prefix := strings.ToLower(req.Prefix)

	query := fmt.Sprintf(
		`SELECT word, COUNT(*) AS cnt FROM (
			SELECT DISTINCT unnest(regexp_split_to_array(LOWER(%s), '\s+')) AS word
			FROM %s
		) sub WHERE word LIKE $1 GROUP BY word ORDER BY cnt DESC, word ASC LIMIT $2`,
		quoteIdent(req.Field), quoteIdent(idx.tableName))

	rows, err := idx.db.QueryContext(ctx, query, prefix+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("pgvector: autocomplete query: %w", err)
	}
	defer rows.Close()

	var terms []searchindex.AutocompleteTerm
	for rows.Next() {
		var t searchindex.AutocompleteTerm
		if err := rows.Scan(&t.Term, &t.Count); err != nil {
			return nil, fmt.Errorf("pgvector: scan autocomplete row: %w", err)
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: autocomplete rows: %w", err)
	}

	return &searchindex.AutocompleteResult{Terms: terms}, nil
}

// Close releases resources held by the index. The underlying *sql.DB is NOT
// closed since it is owned by the caller.
func (idx *Index) Close() error {
	return nil
}

// ---------- Helpers ----------

// queryBuilder tracks parameterized query state.
type queryBuilder struct {
	params []any
	wheres []string
}

// addParam adds a parameter and returns its placeholder string ($N).
func (qb *queryBuilder) addParam(val any) string {
	qb.params = append(qb.params, val)
	return fmt.Sprintf("$%d", len(qb.params))
}

// addWhere adds a WHERE clause fragment.
func (qb *queryBuilder) addWhere(clause string) {
	qb.wheres = append(qb.wheres, clause)
}

// effectiveLimit returns a sensible default if limit is zero or negative.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

// quoteIdent wraps a SQL identifier in double quotes to prevent injection
// and handle reserved words. Internal double quotes are doubled per SQL standard.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// sanitizeIdentifier removes characters not suitable for use in a PostgreSQL
// identifier. Only letters, digits, and underscores are kept.
func sanitizeIdentifier(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

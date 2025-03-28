package grpcdatasource

import (
	"bytes"
	"context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

var _ resolve.DataSource = (*DataSource)(nil)

type DataSource struct {
}

// Load implements resolve.DataSource.
func (d *DataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	panic("unimplemented")
}

// LoadWithFiles implements resolve.DataSource.
func (d *DataSource) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
	panic("unimplemented")
}

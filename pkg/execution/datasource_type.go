package execution

type TypeDataSourcePlanner struct {
}

func (t *TypeDataSourcePlanner) DirectiveName() []byte {
	return []byte("resolveType")
}

func (t *TypeDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) EnterField(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveField(ref int) {

}

func (t *TypeDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &TypeDataSource{}, nil
}

type TypeDataSource struct {
}

func (t *TypeDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {
	return nil
}

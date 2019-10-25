package execution

type StaticDataSourcePlanner struct {
}

func (s *StaticDataSourcePlanner) DirectiveName() []byte {
	return []byte("StaticDataSource")
}

func (s *StaticDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) EnterField(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveField(ref int) {

}

func (s *StaticDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &StaticDataSource{}, nil
}

type StaticDataSource struct {
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {
	return args[0].Value
}

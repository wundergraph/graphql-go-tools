package httpclient

type FileUpload struct {
	path         string
	name         string
	variablePath string
}

func NewFileUpload(path string, name string, variablePath string) *FileUpload {
	return &FileUpload{
		path:         path,
		name:         name,
		variablePath: variablePath,
	}
}

func (f *FileUpload) Path() string {
	return f.path
}

func (f *FileUpload) Name() string {
	return f.name
}

func (f *FileUpload) VariablePath() string {
	return f.variablePath
}

func (f *FileUpload) SetVariablePath(variablePath string) {
	f.variablePath = variablePath
}

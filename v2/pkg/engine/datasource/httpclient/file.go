package httpclient

type File interface {
	Path() string
	Name() string
}

type internalFile struct {
	path string
	name string
}

func NewFile(path string, name string) File {
	return &internalFile{
		path: path,
		name: name,
	}
}

func (f *internalFile) Path() string {
	return f.path
}

func (f *internalFile) Name() string {
	return f.name
}

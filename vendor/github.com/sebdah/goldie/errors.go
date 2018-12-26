package goldie

import "fmt"

// errFixtureNotFound is thrown when the fixture file could not be found.
type errFixtureNotFound struct {
	message string
}

// newErrFixtureNotFound returns a new instance of the error.
func newErrFixtureNotFound() errFixtureNotFound {
	return errFixtureNotFound{
		message: "Golden fixture not found. Try running with -update flag.",
	}
}

// Error returns the error message.
func (e errFixtureNotFound) Error() string {
	return e.message
}

// errFixtureMismatch is thrown when the actual and expected data is not
// matching.
type errFixtureMismatch struct {
	message string
}

// newErrFixtureMismatch returns a new instance of the error.
func newErrFixtureMismatch(message string) errFixtureMismatch {
	return errFixtureMismatch{
		message: message,
	}
}

func (e errFixtureMismatch) Error() string {
	return e.message
}

// errFixtureDirecetoryIsFile is thrown when the fixture directory is a file
type errFixtureDirectoryIsFile struct {
	file string
}

// newFixtureDirectoryIsFile returns a new instance of the error.
func newErrFixtureDirectoryIsFile(file string) errFixtureDirectoryIsFile {
	return errFixtureDirectoryIsFile{
		file: file,
	}
}

func (e errFixtureDirectoryIsFile) Error() string {
	return fmt.Sprintf("fixture folder is a file: %s", e.file)
}

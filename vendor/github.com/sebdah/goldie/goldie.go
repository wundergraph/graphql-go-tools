// Package goldie provides test assertions based on golden files. It's typically
// used for testing responses with larger data bodies.
//
// The concept is straight forward. Valid response data is stored in a "golden
// file". The actual response data will be byte compared with the golden file
// and the test will fail if there is a difference.
//
// Updating the golden file can be done by running `go test -update ./...`.
package goldie

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"text/template"
)

var (
	// FixtureDir is the folder name for where the fixtures are stored. It's
	// relative to the "go test" path.
	FixtureDir = "fixtures"

	// FileNameSuffix is the suffix appended to the fixtures. Set to empty
	// string to disable file name suffixes.
	FileNameSuffix = ".golden"

	// FlagName is the name of the command line flag for go test.
	FlagName = "update"

	// FilePerms is used to set the permissions on the golden fixture files.
	FilePerms os.FileMode = 0644

	// DirPerms is used to set the permissions on the golden fixture folder.
	DirPerms os.FileMode = 0755

	// update determines if the actual received data should be written to the
	// golden files or not. This should be true when you need to update the
	// data, but false when actually running the tests.
	update = flag.Bool(FlagName, false, "Update golden test file fixture")
)

// Assert compares the actual data received with the expected data in the
// golden files. If the update flag is set, it will also update the golden
// file.
//
// `name` refers to the name of the test and it should typically be unique
// within the package. Also it should be a valid file name (so keeping to
// `a-z0-9\-\_` is a good idea).
func Assert(t *testing.T, name string, actualData []byte) {
	AssertWithTemplate(t, name, nil, actualData)
}

// Assert compares the actual data received with the expected data in the
// golden files after executing it as a template with data parameter.
// If the update flag is set, it will also update the golden file.
// `name` refers to the name of the test and it should typically be unique
// within the package. Also it should be a valid file name (so keeping to
// `a-z0-9\-\_` is a good idea).
func AssertWithTemplate(t *testing.T, name string, data interface{}, actualData []byte) {
	if *update {
		err := Update(name, actualData)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}

	err := compare(name, data, actualData)
	if err != nil {
		switch err.(type) {
		case errFixtureNotFound:
			t.Error(err)
			t.FailNow()
		case errFixtureMismatch:
			t.Error(err)
		default:
			t.Error(err)
		}
	}
}

// Update will update the golden fixtures with the received actual data.
//
// This method does not need to be called from code, but it's exposed so that it
// can be explicitly called if needed. The more common approach would be to
// update using `go test -update ./...`.
func Update(name string, actualData []byte) error {
	if err := ensureDir(filepath.Dir(goldenFileName(name))); err != nil {
		return err
	}

	return ioutil.WriteFile(goldenFileName(name), actualData, FilePerms)
}

// compare is reading the golden fixture file and compare the stored data with
// the actual data.
func compare(name string, data interface{}, actualData []byte) error {
	expectedDataTmpl, err := ioutil.ReadFile(goldenFileName(name))

	if err != nil {
		if os.IsNotExist(err) {
			return newErrFixtureNotFound()
		}

		return fmt.Errorf("Expected %s to be nil", err.Error())
	}

	tmpl, err := template.New("test").Parse(string(expectedDataTmpl))
	if err != nil {
		return fmt.Errorf("Expected %s to be nil", err.Error())
	}

	var expectedData bytes.Buffer
	err = tmpl.Execute(&expectedData, data)
	if err != nil {
		return fmt.Errorf("Expected %s to be nil", err.Error())
	}

	if !bytes.Equal(actualData, expectedData.Bytes()) {
		return newErrFixtureMismatch(
			fmt.Sprintf("Result did not match the golden fixture.\n"+
				"Expected: %s\n"+
				"Got: %s",
				string(expectedData.Bytes()),
				string(actualData)))
	}

	return nil
}

// ensureDir will create the fixture folder if it does not already exist.
func ensureDir(loc string) error {
	s, err := os.Stat(loc)
	switch {
	case err != nil && os.IsNotExist(err):
		// the location does not exist, so make directories to there
		err = os.MkdirAll(loc, DirPerms)
		if err != nil {
			return err
		}
		return err
	case err == nil && !s.IsDir():
		return newErrFixtureDirectoryIsFile(loc)
	default:
		return err
	}
}

// goldenFileName simply returns the file name of the golden file fixture.
func goldenFileName(name string) string {
	return filepath.Join(FixtureDir, fmt.Sprintf("%s%s", name, FileNameSuffix))
}

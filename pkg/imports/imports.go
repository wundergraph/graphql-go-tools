// Package imports helps combining multiple GraphQL documents into one document using import comments.
package imports

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	importStatementRegex, _ = regexp.Compile(`(#import "[^";]+")`)
	pathStatementRegex, _   = regexp.Compile(`"(.*?)"`)
)

type Scanner struct {
	knownFiles map[string]struct{}
}

func (s *Scanner) ScanFile(inputFilePath string) (*GraphQLFile, error) {
	s.knownFiles = map[string]struct{}{}
	return s.scanFile(inputFilePath)
}

func (s *Scanner) ScanRegex(filesRegex string) (*GraphQLFile, error) {
	s.knownFiles = map[string]struct{}{}
	file := &GraphQLFile{}
	var err error
	file.Imports, err = s.fileImportsForPattern(filesRegex)
	return file, err
}

func (s *Scanner) scanFile(inputFilePath string) (*GraphQLFile, error) {

	basePath, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	absoluteFilePath, err := filepath.Abs(inputFilePath)
	if err != nil {
		return nil, err
	}

	relativeFilePath, err := filepath.Rel(basePath, absoluteFilePath)
	if err != nil {
		return nil, err
	}

	if _, exists := s.knownFiles[relativeFilePath]; exists {
		return nil, fmt.Errorf("file forms import cycle: %s", relativeFilePath)
	}

	s.knownFiles[relativeFilePath] = struct{}{}

	fileDir := filepath.Dir(relativeFilePath)

	content, err := ioutil.ReadFile(inputFilePath)
	if err != nil {
		return nil, err
	}

	file := &GraphQLFile{
		RelativePath: relativeFilePath,
	}

	importStatements := importStatementRegex.FindAll(content, -1)
	for i := 0; i < len(importStatements); i++ {
		importFilePath := s.importFilePath(string(importStatements[i]))
		filePathPattern := path.Join(fileDir, importFilePath)
		if importFilePath != "" {
			imports, err := s.fileImportsForPattern(filePathPattern)
			if err != nil {
				return nil, err
			}
			file.Imports = append(file.Imports, imports...)
		}
	}

	return file, nil
}

func (s *Scanner) importFilePath(importStatement string) string {
	out := pathStatementRegex.FindString(importStatement)
	out = strings.TrimLeft(out, "\"")
	out = strings.TrimRight(out, "\"")
	return out
}

func (s *Scanner) fileImportsForPattern(pattern string) ([]GraphQLFile, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	out := make([]GraphQLFile, 0, len(matches))
	for _, match := range matches {
		importFile, err := s.scanFile(match)
		if err != nil {
			return nil, err
		}
		if importFile == nil {
			continue
		}
		out = append(out, *importFile)
	}
	return out, nil
}

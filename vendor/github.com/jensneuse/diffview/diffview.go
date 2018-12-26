package diffview

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
)

func NewGoland() DiffViewer {
	return DiffViewer{
		opener: golandDiffViewer{},
	}
}

type opener interface {
	open(a, b string) error
}

type DiffViewer struct {
	aFile      *os.File
	bFile      *os.File
	_aFileName string
	_bFileName string
	opener     opener
}

func (d *DiffViewer) aFileName(name string) string {

	if d._aFileName == "" {
		d._aFileName = path.Join(os.TempDir(), name+"_a.txt")
	}

	return d._aFileName
}

func (d DiffViewer) bFileName(name string) string {
	if d._bFileName == "" {
		d._bFileName = path.Join(os.TempDir(), name+"_b.txt")
	}

	return d._bFileName
}

func (d *DiffViewer) files(name string) (a, b *os.File) {
	var err error
	d.aFile, err = os.OpenFile(d.aFileName(name), os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	d.bFile, err = os.OpenFile(d.bFileName(name), os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	return d.aFile, d.bFile
}

func (d *DiffViewer) cleanup(name string) {
	err := os.Remove(d.aFileName(name))
	if err != nil {
		log.Fatal(err)
	}
	err = os.Remove(d.bFileName(name))
	if err != nil {
		log.Fatal(err)
	}
}

func (d DiffViewer) DiffViewBytes(name string, a, b []byte) {

	var err error
	aFile, bFile := d.files(name)
	if err != nil {
		log.Fatal(err)
	}

	_, err = bytes.NewBuffer(a).WriteTo(aFile)
	if err != nil {
		log.Fatal(err)
	}

	_, err = bytes.NewBuffer(b).WriteTo(bFile)
	if err != nil {
		log.Fatal(err)
	}

	err = aFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	err = bFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	err = d.opener.open(d.aFileName(name), d.bFileName(name))
	if err != nil {
		log.Fatal(err)
	}

	d.cleanup(name)
}

func (d DiffViewer) DiffViewReader(name string, a, b io.Reader) {
	aBytes, err := ioutil.ReadAll(a)
	if err != nil {
		log.Fatal(err)
	}

	bBytes, err := ioutil.ReadAll(b)
	if err != nil {
		log.Fatal(err)
	}

	d.DiffViewBytes(name, aBytes, bBytes)
}

func (d DiffViewer) DiffViewAny(name string, a, b interface{}) {

	var aBuff bytes.Buffer
	spew.Fdump(&aBuff, a)

	var bBuff bytes.Buffer
	spew.Fdump(&bBuff, b)

	d.DiffViewBytes(name, aBuff.Bytes(), bBuff.Bytes())
}

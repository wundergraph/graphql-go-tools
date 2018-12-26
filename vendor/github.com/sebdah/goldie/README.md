# goldie - Golden test utility for Go

[![GoDoc](https://godoc.org/github.com/sebdah/goldie?status.svg)](https://godoc.org/github.com/sebdah/goldie)
[![CircleCI](https://circleci.com/gh/sebdah/goldie.svg?style=svg)](https://circleci.com/gh/sebdah/goldie)
[![Go Report Card](https://goreportcard.com/badge/github.com/sebdah/goldie)](https://goreportcard.com/report/github.com/sebdah/goldie)

`goldie` is a golden file test utility for Go projects. It's typically used for
testing responses with larger data bodies.

The concept is straight forward. Valid response data is stored in a "golden
file". The actual response data will be byte compared with the golden file and
the test will fail if there is a difference.

Updating the golden file can be done by running `go test -update ./...`.

See the [GoDoc](https://godoc.org/github.com/sebdah/goldie) for API reference
and configuration options.

## Example usage

The below example fetches data from a REST API. The last line in the test is the
actual usage of `goldie`. It takes the HTTP response body and asserts that it's
what is present in the golden test file.

```
func TestExample(t *testing.T) {
    recorder := httptest.NewRecorder()

    req, err := http.NewRequest("GET", "/example", nil)
    assert.Nil(t, err)

    handler := http.HandlerFunc(ExampleHandler)
    handler.ServeHTTP()

    goldie.Assert(t, "example", recorder.Body.Bytes())
}
```

## Using template golden file

If some values in the golden file can change depending on the test, you can use golang
template in the golden file and pass the data to `goldie.AssertWithTemplate`.

### example.golden
```
This is a {{ .Type }} file.
```

### Test
```
func TestTemplateExample(t *testing.T) {
    recorder := httptest.NewRecorder()

    req, err := http.NewRequest("POST", "/example/Golden", nil)
    assert.Nil(t, err)

    handler := http.HandlerFunc(ExampleHandler)
    handler.ServeHTTP()

    data := struct {
        Type	string
    }{
        Type:	"Golden",
    }

    goldie.AssertWithTemplate(t, "example", data, recorder.Body.Bytes())
}
```

Then run your test with the `-update` flag the first time to store the result.

`go test -update ./...`

For any consecutive runs where you actually want to compare the data, simply
drop the `-update` flag.

`go test ./...`

## FAQ

### Do you need any help in the project?

Yes, please! Pull requests are most welcome. On the wish list:

- Unit tests.
- Better output for failed tests. A diff of some sort would be great.

### Why the name `goldie`?

The name comes from the fact that it's for Go and handles golden file testing.
But yes, it may not be the best name in the world.

### How did you come up with the idea?

This is based on the great [Advanced Go
testing](https://www.youtube.com/watch?v=yszygk1cpEc) talk by
[@mitchellh](https://twitter.com/mitchellh).

## License

MIT

Copyright 2016 Sebastian Dahlgren <sebastian.dahlgren@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

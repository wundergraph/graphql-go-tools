# diffview

This library opens your favourite diff viewer directly from your tests.

## why

I did a lot of testing recently where I had to compare complex structs and or strings.

I really like the GoLand diff viewer.

I absolutely hated to copy stuff from the console to two files in order to use the GoLand diff view. So here is diffview.

## current caveats

It should not run from within CI. I guess a testing flag should do.
I'll fix this soon/contributions welcome.

## usage

```go

package diffview

import "testing"

var (
	a = `foo
bar
baz
`
	b = `foo
baz
bar`
)

func TestGolandDiffView(t *testing.T) {
	NewGoland().DiffViewBytes("test", []byte(a), []byte(b))
}
```

## contributing

Feel free to add Openers for different operating systems/diff view tools.

package argument_templates

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestArgumentTemplateRegex(t *testing.T) {
	t.Run("an argument templates with variable whitespace is captured", func(t *testing.T) {
		input := `{{
			args.input.id          }}
		`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 1, len(matches))
		assert.Equal(t, 2, len(matches[0]))
		assert.Equal(t, `input.id`, matches[0][1])
	})

	t.Run("an argument template with a trailing period is not captured", func(t *testing.T) {
		input := `{{args.input.id.}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run("an argument template with multiple subsequent periods is not captured #1", func(t *testing.T) {
		input := `{{args..input..id}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run("an argument template with multiple subsequent periods is not captured #2", func(t *testing.T) {
		input := `{{args..input.id}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run("an argument template with multiple subsequent periods is not captured #2", func(t *testing.T) {
		input := `{{args.input..id}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does not contain "args" is not captured`, func(t *testing.T) {
		input := `{{input.id}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does not contain double curly braces is not captured #1`, func(t *testing.T) {
		input := `args.input.id`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does contain double curly braces is not captured #2`, func(t *testing.T) {
		input := `{args.input.id}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does contain ending double curly braces is not captured`, func(t *testing.T) {
		input := `{{args.input.id`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does contain any arguments is not captured #1`, func(t *testing.T) {
		input := `{{args}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run(`an argument template that does contain any arguments is not captured #2`, func(t *testing.T) {
		input := `{{args.}}`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 0, len(matches))
	})

	t.Run("multiple templates are captured", func(t *testing.T) {
		input := `{{
			args.input.id          }}{{      args.hello.world.zusammen.yeoreobun}}{{

			args.test
		}}
		`
		matches := ArgumentTemplateRegex.FindAllStringSubmatch(input, -1)
		assert.Equal(t, 3, len(matches))
		assert.Equal(t, 2, len(matches[0]))
		assert.Equal(t, `input.id`, matches[0][1])
		assert.Equal(t, 2, len(matches[1]))
		assert.Equal(t, `hello.world.zusammen.yeoreobun`, matches[1][1])
		assert.Equal(t, 2, len(matches[2]))
		assert.Equal(t, `test`, matches[2][1])
	})
}

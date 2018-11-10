package transform

import (
	. "github.com/franela/goblin"
	. "github.com/onsi/gomega"
	"testing"
)

func TestBytesTransform(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("StringSliceToFloat32", func() {

		g.It("should transform byte[\"13.37\"] to float32(13.37)", func() {

			input := []byte("13.37")
			out, err := StringSliceToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(13.37)))
		})

		g.It("should transform byte[\"-13.37\"] to float32(-13.37)", func() {

			input := []byte("-13.37")
			out, err := StringSliceToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(-13.37)))
		})

		g.It("should transform byte[\"1337\"] to float32(1337)", func() {

			input := []byte("1337")
			out, err := StringSliceToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(1337)))
		})

		g.It("should transform byte[\"invalidInput\"] to err(invalidInput)", func() {

			input := []byte("invalidInput")
			out, err := StringSliceToFloat32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(float32(0)))
		})
	})

	g.Describe("StringSliceToInt32", func() {

		g.It("should transform byte[\"1337\"] into int32(1337)", func() {
			input := []byte("1337")
			out, err := StringSliceToInt32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(int32(1337)))
		})

		g.It("should transform byte[\"-1337\"] into int32(1337)", func() {
			input := []byte("-1337")
			out, err := StringSliceToInt32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(int32(-1337)))
		})

		g.It("should transform byte[\"13.37\"] into err(invalid syntax)", func() {
			input := []byte("13.37")
			out, err := StringSliceToInt32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(int32(0)))
		})

		g.It("should transform byte[\"invalidInput\"] into err(invalid syntax)", func() {
			input := []byte("invalidInput")
			out, err := StringSliceToInt32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(int32(0)))
		})
	})
}

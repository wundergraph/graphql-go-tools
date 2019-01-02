package transform

import (
	. "github.com/franela/goblin"
	. "github.com/onsi/gomega"
	"testing"
)

func TestBytesTransform(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("StringToFloat32", func() {

		g.It("should transform byte[\"13.37\"] to float32(13.37)", func() {

			input := "13.37"
			out, err := StringToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(13.37)))
		})

		g.It("should transform byte[\"-13.37\"] to float32(-13.37)", func() {

			input := "-13.37"
			out, err := StringToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(-13.37)))
		})

		g.It("should transform byte[\"1337\"] to float32(1337)", func() {

			input := "1337"
			out, err := StringToFloat32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(float32(1337)))
		})

		g.It("should transform byte[\"invalidInput\"] to err(invalidInput)", func() {

			input := "invalidInput"
			out, err := StringToFloat32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(float32(0)))
		})
	})

	g.Describe("StringToInt32", func() {

		g.It("should transform byte[\"1337\"] into int32(1337)", func() {
			input := "1337"
			out, err := StringToInt32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(int32(1337)))
		})

		g.It("should transform byte[\"-1337\"] into int32(1337)", func() {
			input := "-1337"
			out, err := StringToInt32(input)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(int32(-1337)))
		})

		g.It("should transform byte[\"13.37\"] into err(invalid syntax)", func() {
			input := "13.37"
			out, err := StringToInt32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(int32(0)))
		})

		g.It("should transform byte[\"invalidInput\"] into err(invalid syntax)", func() {
			input := "invalidInput"
			out, err := StringToInt32(input)
			Expect(err).NotTo(BeNil())
			Expect(out).To(Equal(int32(0)))
		})
	})
}

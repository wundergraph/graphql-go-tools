//go:generate mockgen -source=$GOFILE -destination=./prisma_mock.go -package=stage2 Prisma
package stage2

type Prisma interface {
	Query(request string) (result string)
}

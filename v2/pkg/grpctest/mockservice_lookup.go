package grpctest

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// LookupWarehouseById implements productv1.ProductServiceServer.
func (s *MockService) LookupWarehouseById(ctx context.Context, in *productv1.LookupWarehouseByIdRequest) (*productv1.LookupWarehouseByIdResponse, error) {
	var results []*productv1.Warehouse

	// Special requirement: return one less item than requested to test error handling
	// This deliberately breaks the normal pattern of returning the same number of items as keys
	keys := in.GetKeys()
	if len(keys) == 0 {
		return &productv1.LookupWarehouseByIdResponse{
			Result: results,
		}, nil
	}

	// Return all items except the last one to test error scenarios
	for i, input := range keys {
		// Skip the last item to create an intentional mismatch
		if i == len(keys)-1 {
			break
		}

		warehouseId := input.GetId()
		results = append(results, &productv1.Warehouse{
			Id:       warehouseId,
			Name:     fmt.Sprintf("Warehouse %s", warehouseId),
			Location: fmt.Sprintf("Location %d", rand.Intn(100)),
		})
	}

	return &productv1.LookupWarehouseByIdResponse{
		Result: results,
	}, nil
}

func (s *MockService) LookupProductById(ctx context.Context, in *productv1.LookupProductByIdRequest) (*productv1.LookupProductByIdResponse, error) {
	var results []*productv1.Product

	for _, input := range in.GetKeys() {
		productId := input.GetId()
		results = append(results, &productv1.Product{
			Id:    productId,
			Name:  fmt.Sprintf("Product %s", productId),
			Price: 99.99,
		})
	}

	return &productv1.LookupProductByIdResponse{
		Result: results,
	}, nil
}

func (s *MockService) LookupStorageById(ctx context.Context, in *productv1.LookupStorageByIdRequest) (*productv1.LookupStorageByIdResponse, error) {
	var results []*productv1.Storage

	for _, input := range in.GetKeys() {
		storageId := input.GetId()
		results = append(results, &productv1.Storage{
			Id:       storageId,
			Name:     fmt.Sprintf("Storage %s", storageId),
			Location: fmt.Sprintf("Location %d", rand.Intn(100)),
		})
	}

	return &productv1.LookupStorageByIdResponse{
		Result: results,
	}, nil
}

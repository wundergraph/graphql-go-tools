package grpctest

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// RequireStorageStockHealthScoreById implements [productv1.ProductServiceServer].
func (s *MockService) RequireStorageStockHealthScoreById(_ context.Context, req *productv1.RequireStorageStockHealthScoreByIdRequest) (*productv1.RequireStorageStockHealthScoreByIdResponse, error) {
	results := make([]*productv1.RequireStorageStockHealthScoreByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		// Score = itemCount * 0.1, +10 if restockData provided
		score := float64(fields.GetItemCount()) * 0.1
		if fields.GetRestockData().GetLastRestockDate() != "" {
			score += 10.0
		}

		results = append(results, &productv1.RequireStorageStockHealthScoreByIdResult{
			StockHealthScore: score,
		})
	}

	return &productv1.RequireStorageStockHealthScoreByIdResponse{Result: results}, nil
}

// RequireWarehouseStockHealthScoreById implements [productv1.ProductServiceServer].
func (s *MockService) RequireWarehouseStockHealthScoreById(_ context.Context, req *productv1.RequireWarehouseStockHealthScoreByIdRequest) (*productv1.RequireWarehouseStockHealthScoreByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseStockHealthScoreByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		// Score = inventoryCount * 0.1, +10 if restockData provided
		score := float64(fields.GetInventoryCount()) * 0.1
		if fields.GetRestockData().GetLastRestockDate() != "" {
			score += 10.0
		}

		results = append(results, &productv1.RequireWarehouseStockHealthScoreByIdResult{
			StockHealthScore: score,
		})
	}

	return &productv1.RequireWarehouseStockHealthScoreByIdResponse{Result: results}, nil
}

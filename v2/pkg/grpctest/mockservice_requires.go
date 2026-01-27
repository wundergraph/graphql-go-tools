package grpctest

import (
	"context"
	"strings"

	"google.golang.org/protobuf/types/known/wrapperspb"

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

// RequireStorageTagSummaryById implements [productv1.ProductServiceServer].
func (s *MockService) RequireStorageTagSummaryById(_ context.Context, req *productv1.RequireStorageTagSummaryByIdRequest) (*productv1.RequireStorageTagSummaryByIdResponse, error) {
	results := make([]*productv1.RequireStorageTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		// Concatenate all tags into a summary string
		tags := fields.GetTags()
		tagSummary := strings.Join(tags, ", ")

		results = append(results, &productv1.RequireStorageTagSummaryByIdResult{
			TagSummary: tagSummary,
		})
	}

	return &productv1.RequireStorageTagSummaryByIdResponse{Result: results}, nil
}

// RequireStorageOptionalTagSummaryById implements [productv1.ProductServiceServer].
func (s *MockService) RequireStorageOptionalTagSummaryById(_ context.Context, req *productv1.RequireStorageOptionalTagSummaryByIdRequest) (*productv1.RequireStorageOptionalTagSummaryByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		optionalTags := fields.GetOptionalTags()

		var optionalTagSummary *wrapperspb.StringValue
		// If optionalTags is provided and has items, create summary
		if optionalTags != nil && optionalTags.GetList() != nil && len(optionalTags.GetList().GetItems()) > 0 {
			summary := strings.Join(optionalTags.GetList().GetItems(), ", ")
			optionalTagSummary = &wrapperspb.StringValue{Value: summary}
		}
		// Otherwise, optionalTagSummary remains nil

		results = append(results, &productv1.RequireStorageOptionalTagSummaryByIdResult{
			OptionalTagSummary: optionalTagSummary,
		})
	}

	return &productv1.RequireStorageOptionalTagSummaryByIdResponse{Result: results}, nil
}

// RequireStorageMetadataScoreById implements [productv1.ProductServiceServer].
func (s *MockService) RequireStorageMetadataScoreById(_ context.Context, req *productv1.RequireStorageMetadataScoreByIdRequest) (*productv1.RequireStorageMetadataScoreByIdResponse, error) {
	results := make([]*productv1.RequireStorageMetadataScoreByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadata := fields.GetMetadata()

		// Calculate score based on metadata: capacity * zone_weight
		// Zone weights: "A" = 1.0, "B" = 0.8, "C" = 0.6, default = 0.5
		capacity := float64(metadata.GetCapacity())
		zone := metadata.GetZone()

		var zoneWeight float64
		switch zone {
		case "A":
			zoneWeight = 1.0
		case "B":
			zoneWeight = 0.8
		case "C":
			zoneWeight = 0.6
		default:
			zoneWeight = 0.5
		}

		score := capacity * zoneWeight

		results = append(results, &productv1.RequireStorageMetadataScoreByIdResult{
			MetadataScore: score,
		})
	}

	return &productv1.RequireStorageMetadataScoreByIdResponse{Result: results}, nil
}

// RequireStorageProcessedMetadataById implements [productv1.ProductServiceServer].
// Returns a complex type (StorageMetadata) with processed values.
func (s *MockService) RequireStorageProcessedMetadataById(_ context.Context, req *productv1.RequireStorageProcessedMetadataByIdRequest) (*productv1.RequireStorageProcessedMetadataByIdResponse, error) {
	results := make([]*productv1.RequireStorageProcessedMetadataByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadata := fields.GetMetadata()

		// Process metadata: double capacity, uppercase zone, adjust priority
		processedMetadata := &productv1.StorageMetadata{
			Capacity: metadata.GetCapacity() * 2,
			Zone:     strings.ToUpper(metadata.GetZone()),
			Priority: metadata.GetPriority() + 10,
		}

		results = append(results, &productv1.RequireStorageProcessedMetadataByIdResult{
			ProcessedMetadata: processedMetadata,
		})
	}

	return &productv1.RequireStorageProcessedMetadataByIdResponse{Result: results}, nil
}

// RequireStorageOptionalProcessedMetadataById implements [productv1.ProductServiceServer].
// Returns a nullable complex type (StorageMetadata).
func (s *MockService) RequireStorageOptionalProcessedMetadataById(_ context.Context, req *productv1.RequireStorageOptionalProcessedMetadataByIdRequest) (*productv1.RequireStorageOptionalProcessedMetadataByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalProcessedMetadataByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadata := fields.GetMetadata()

		var processedMetadata *productv1.StorageMetadata
		// Return nil for every other item to test nullable behavior
		if i%2 == 0 && metadata != nil {
			processedMetadata = &productv1.StorageMetadata{
				Capacity: metadata.GetCapacity() * 3,
				Zone:     strings.ToLower(metadata.GetZone()),
				Priority: 1, // Default priority for optional
			}
		}
		// For odd indices, processedMetadata remains nil

		results = append(results, &productv1.RequireStorageOptionalProcessedMetadataByIdResult{
			OptionalProcessedMetadata: processedMetadata,
		})
	}

	return &productv1.RequireStorageOptionalProcessedMetadataByIdResponse{Result: results}, nil
}

// RequireStorageProcessedTagsById implements [productv1.ProductServiceServer].
// Returns a list of strings with processed tags.
func (s *MockService) RequireStorageProcessedTagsById(_ context.Context, req *productv1.RequireStorageProcessedTagsByIdRequest) (*productv1.RequireStorageProcessedTagsByIdResponse, error) {
	results := make([]*productv1.RequireStorageProcessedTagsByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		tags := fields.GetTags()

		// Process tags: uppercase and add prefix
		processedTags := make([]string, 0, len(tags))
		for _, tag := range tags {
			processedTags = append(processedTags, "PROCESSED_"+strings.ToUpper(tag))
		}

		results = append(results, &productv1.RequireStorageProcessedTagsByIdResult{
			ProcessedTags: processedTags,
		})
	}

	return &productv1.RequireStorageProcessedTagsByIdResponse{Result: results}, nil
}

// RequireStorageOptionalProcessedTagsById implements [productv1.ProductServiceServer].
// Returns a nullable list of strings.
func (s *MockService) RequireStorageOptionalProcessedTagsById(_ context.Context, req *productv1.RequireStorageOptionalProcessedTagsByIdRequest) (*productv1.RequireStorageOptionalProcessedTagsByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalProcessedTagsByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		optionalTags := fields.GetOptionalTags()

		var processedTags *productv1.ListOfString
		// Return nil for every other item to test nullable behavior
		// Also return nil if optionalTags is empty (matching RequireStorageOptionalTagSummaryById behavior)
		if i%2 == 0 && optionalTags != nil && optionalTags.GetList() != nil && len(optionalTags.GetList().GetItems()) > 0 {
			items := optionalTags.GetList().GetItems()
			processed := make([]string, 0, len(items))
			for _, tag := range items {
				processed = append(processed, "OPT_"+strings.ToLower(tag))
			}
			processedTags = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: processed,
				},
			}
		}
		// For odd indices, processedTags remains nil

		results = append(results, &productv1.RequireStorageOptionalProcessedTagsByIdResult{
			OptionalProcessedTags: processedTags,
		})
	}

	return &productv1.RequireStorageOptionalProcessedTagsByIdResponse{Result: results}, nil
}

// RequireStorageProcessedMetadataHistoryById implements [productv1.ProductServiceServer].
// Returns a list of complex types (StorageMetadata).
func (s *MockService) RequireStorageProcessedMetadataHistoryById(_ context.Context, req *productv1.RequireStorageProcessedMetadataHistoryByIdRequest) (*productv1.RequireStorageProcessedMetadataHistoryByIdResponse, error) {
	results := make([]*productv1.RequireStorageProcessedMetadataHistoryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadataHistory := fields.GetMetadataHistory()

		// Process each metadata in history: multiply capacity by index+1, prefix zone
		processedHistory := make([]*productv1.StorageMetadata, 0, len(metadataHistory))
		for j, metadata := range metadataHistory {
			processedHistory = append(processedHistory, &productv1.StorageMetadata{
				Capacity: metadata.GetCapacity() * int32(j+1),
				Zone:     "HIST_" + metadata.GetZone(),
				Priority: int32(j + 1),
			})
		}

		results = append(results, &productv1.RequireStorageProcessedMetadataHistoryByIdResult{
			ProcessedMetadataHistory: processedHistory,
		})
	}

	return &productv1.RequireStorageProcessedMetadataHistoryByIdResponse{Result: results}, nil
}

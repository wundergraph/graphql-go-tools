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

// RequireWarehouseTagSummaryById implements [productv1.ProductServiceServer].
func (s *MockService) RequireWarehouseTagSummaryById(_ context.Context, req *productv1.RequireWarehouseTagSummaryByIdRequest) (*productv1.RequireWarehouseTagSummaryByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		// Concatenate all tags into a summary string
		tags := fields.GetTags()
		tagSummary := strings.Join(tags, ", ")

		results = append(results, &productv1.RequireWarehouseTagSummaryByIdResult{
			TagSummary: tagSummary,
		})
	}

	return &productv1.RequireWarehouseTagSummaryByIdResponse{Result: results}, nil
}

// RequireWarehouseOptionalTagSummaryById implements [productv1.ProductServiceServer].
func (s *MockService) RequireWarehouseOptionalTagSummaryById(_ context.Context, req *productv1.RequireWarehouseOptionalTagSummaryByIdRequest) (*productv1.RequireWarehouseOptionalTagSummaryByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseOptionalTagSummaryByIdResult, 0, len(req.GetContext()))

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

		results = append(results, &productv1.RequireWarehouseOptionalTagSummaryByIdResult{
			OptionalTagSummary: optionalTagSummary,
		})
	}

	return &productv1.RequireWarehouseOptionalTagSummaryByIdResponse{Result: results}, nil
}

// RequireWarehouseMetadataScoreById implements [productv1.ProductServiceServer].
func (s *MockService) RequireWarehouseMetadataScoreById(_ context.Context, req *productv1.RequireWarehouseMetadataScoreByIdRequest) (*productv1.RequireWarehouseMetadataScoreByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseMetadataScoreByIdResult, 0, len(req.GetContext()))

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

		results = append(results, &productv1.RequireWarehouseMetadataScoreByIdResult{
			MetadataScore: score,
		})
	}

	return &productv1.RequireWarehouseMetadataScoreByIdResponse{Result: results}, nil
}

// RequireWarehouseProcessedMetadataById implements [productv1.ProductServiceServer].
// Returns a complex type (WarehouseMetadata) with processed values.
func (s *MockService) RequireWarehouseProcessedMetadataById(_ context.Context, req *productv1.RequireWarehouseProcessedMetadataByIdRequest) (*productv1.RequireWarehouseProcessedMetadataByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseProcessedMetadataByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadata := fields.GetMetadata()

		// Process metadata: double capacity, uppercase zone, adjust priority
		processedMetadata := &productv1.WarehouseMetadata{
			Capacity: metadata.GetCapacity() * 2,
			Zone:     strings.ToUpper(metadata.GetZone()),
			Priority: metadata.GetPriority() + 10,
		}

		results = append(results, &productv1.RequireWarehouseProcessedMetadataByIdResult{
			ProcessedMetadata: processedMetadata,
		})
	}

	return &productv1.RequireWarehouseProcessedMetadataByIdResponse{Result: results}, nil
}

// RequireWarehouseOptionalProcessedMetadataById implements [productv1.ProductServiceServer].
// Returns a nullable complex type (WarehouseMetadata).
func (s *MockService) RequireWarehouseOptionalProcessedMetadataById(_ context.Context, req *productv1.RequireWarehouseOptionalProcessedMetadataByIdRequest) (*productv1.RequireWarehouseOptionalProcessedMetadataByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseOptionalProcessedMetadataByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadata := fields.GetMetadata()

		var processedMetadata *productv1.WarehouseMetadata
		// Return nil for every other item to test nullable behavior
		if i%2 == 0 && metadata != nil {
			processedMetadata = &productv1.WarehouseMetadata{
				Capacity: metadata.GetCapacity() * 3,
				Zone:     strings.ToLower(metadata.GetZone()),
				Priority: 1, // Default priority for optional
			}
		}
		// For odd indices, processedMetadata remains nil

		results = append(results, &productv1.RequireWarehouseOptionalProcessedMetadataByIdResult{
			OptionalProcessedMetadata: processedMetadata,
		})
	}

	return &productv1.RequireWarehouseOptionalProcessedMetadataByIdResponse{Result: results}, nil
}

// RequireWarehouseProcessedTagsById implements [productv1.ProductServiceServer].
// Returns a list of strings with processed tags.
func (s *MockService) RequireWarehouseProcessedTagsById(_ context.Context, req *productv1.RequireWarehouseProcessedTagsByIdRequest) (*productv1.RequireWarehouseProcessedTagsByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseProcessedTagsByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		tags := fields.GetTags()

		// Process tags: uppercase and add prefix
		processedTags := make([]string, 0, len(tags))
		for _, tag := range tags {
			processedTags = append(processedTags, "PROCESSED_"+strings.ToUpper(tag))
		}

		results = append(results, &productv1.RequireWarehouseProcessedTagsByIdResult{
			ProcessedTags: processedTags,
		})
	}

	return &productv1.RequireWarehouseProcessedTagsByIdResponse{Result: results}, nil
}

// RequireWarehouseOptionalProcessedTagsById implements [productv1.ProductServiceServer].
// Returns a nullable list of strings.
func (s *MockService) RequireWarehouseOptionalProcessedTagsById(_ context.Context, req *productv1.RequireWarehouseOptionalProcessedTagsByIdRequest) (*productv1.RequireWarehouseOptionalProcessedTagsByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseOptionalProcessedTagsByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		optionalTags := fields.GetOptionalTags()

		var processedTags *productv1.ListOfString
		// Return nil for every other item to test nullable behavior
		// Also return nil if optionalTags is empty (matching RequireWarehouseOptionalTagSummaryById behavior)
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

		results = append(results, &productv1.RequireWarehouseOptionalProcessedTagsByIdResult{
			OptionalProcessedTags: processedTags,
		})
	}

	return &productv1.RequireWarehouseOptionalProcessedTagsByIdResponse{Result: results}, nil
}

// RequireWarehouseProcessedMetadataHistoryById implements [productv1.ProductServiceServer].
// Returns a list of complex types (WarehouseMetadata).
func (s *MockService) RequireWarehouseProcessedMetadataHistoryById(_ context.Context, req *productv1.RequireWarehouseProcessedMetadataHistoryByIdRequest) (*productv1.RequireWarehouseProcessedMetadataHistoryByIdResponse, error) {
	results := make([]*productv1.RequireWarehouseProcessedMetadataHistoryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		metadataHistory := fields.GetMetadataHistory()

		// Process each metadata in history: multiply capacity by index+1, prefix zone
		processedHistory := make([]*productv1.WarehouseMetadata, 0, len(metadataHistory))
		for j, metadata := range metadataHistory {
			processedHistory = append(processedHistory, &productv1.WarehouseMetadata{
				Capacity: metadata.GetCapacity() * int32(j+1),
				Zone:     "HIST_" + metadata.GetZone(),
				Priority: int32(j + 1),
			})
		}

		results = append(results, &productv1.RequireWarehouseProcessedMetadataHistoryByIdResult{
			ProcessedMetadataHistory: processedHistory,
		})
	}

	return &productv1.RequireWarehouseProcessedMetadataHistoryByIdResponse{Result: results}, nil
}

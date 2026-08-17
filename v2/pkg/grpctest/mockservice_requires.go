package grpctest

import (
	"context"
	"fmt"
	"slices"
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

// RequireStorageKindSummaryById implements [productv1.ProductServiceServer].
// Returns the enum value as a string summary.
func (s *MockService) RequireStorageKindSummaryById(_ context.Context, req *productv1.RequireStorageKindSummaryByIdRequest) (*productv1.RequireStorageKindSummaryByIdResponse, error) {
	results := make([]*productv1.RequireStorageKindSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		kindSummary := fmt.Sprintf("Kind: %s", fields.GetStorageKind().String())

		results = append(results, &productv1.RequireStorageKindSummaryByIdResult{
			KindSummary: kindSummary,
		})
	}

	return &productv1.RequireStorageKindSummaryByIdResponse{Result: results}, nil
}

// RequireStorageCategoryInfoSummaryById implements [productv1.ProductServiceServer].
// Returns a summary string from nested category info containing an enum field.
func (s *MockService) RequireStorageCategoryInfoSummaryById(_ context.Context, req *productv1.RequireStorageCategoryInfoSummaryByIdRequest) (*productv1.RequireStorageCategoryInfoSummaryByIdResponse, error) {
	results := make([]*productv1.RequireStorageCategoryInfoSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		catInfo := fields.GetCategoryInfo()
		summary := fmt.Sprintf("%s (%s)", catInfo.GetName(), catInfo.GetKind().String())

		results = append(results, &productv1.RequireStorageCategoryInfoSummaryByIdResult{
			CategoryInfoSummary: summary,
		})
	}

	return &productv1.RequireStorageCategoryInfoSummaryByIdResponse{Result: results}, nil
}

// RequireStorageItemInfoById implements [productv1.ProductServiceServer].
// Extracts primaryItem interface from fields and formats a summary string.
func (s *MockService) RequireStorageItemInfoById(_ context.Context, req *productv1.RequireStorageItemInfoByIdRequest) (*productv1.RequireStorageItemInfoByIdResponse, error) {
	results := make([]*productv1.RequireStorageItemInfoByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		item := fields.GetPrimaryItem()

		var summary string
		switch v := item.GetInstance().(type) {
		case *productv1.RequireStorageItemInfoByIdFields_StorageItem_PalletItem:
			summary = fmt.Sprintf("Pallet: %s (count: %d)", v.PalletItem.GetName(), v.PalletItem.GetPalletCount())
		case *productv1.RequireStorageItemInfoByIdFields_StorageItem_ContainerItem:
			summary = fmt.Sprintf("Container: %s (size: %s)", v.ContainerItem.GetName(), v.ContainerItem.GetContainerSize())
		default:
			summary = "Unknown item"
		}

		results = append(results, &productv1.RequireStorageItemInfoByIdResult{
			ItemInfo: summary,
		})
	}

	return &productv1.RequireStorageItemInfoByIdResponse{Result: results}, nil
}

// RequireStorageOperationReportById implements [productv1.ProductServiceServer].
// Extracts lastStorageOperation union from fields and formats a report string.
func (s *MockService) RequireStorageOperationReportById(_ context.Context, req *productv1.RequireStorageOperationReportByIdRequest) (*productv1.RequireStorageOperationReportByIdResponse, error) {
	results := make([]*productv1.RequireStorageOperationReportByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		op := fields.GetLastStorageOperation()

		var report string
		switch v := op.GetValue().(type) {
		case *productv1.RequireStorageOperationReportByIdFields_StorageOperationResult_StorageSuccess:
			report = fmt.Sprintf("Success: %s at %s", v.StorageSuccess.GetMessage(), v.StorageSuccess.GetCompletedAt())
		case *productv1.RequireStorageOperationReportByIdFields_StorageOperationResult_StorageFailure:
			report = fmt.Sprintf("Failure: %s (code: %s)", v.StorageFailure.GetMessage(), v.StorageFailure.GetErrorCode())
		default:
			report = "Unknown operation"
		}

		results = append(results, &productv1.RequireStorageOperationReportByIdResult{
			OperationReport: report,
		})
	}

	return &productv1.RequireStorageOperationReportByIdResponse{Result: results}, nil
}

// RequireStorageSecuritySummaryById implements [productv1.ProductServiceServer].
// Extracts securitySetup (concrete wrapping abstract) and formats a summary.
func (s *MockService) RequireStorageSecuritySummaryById(_ context.Context, req *productv1.RequireStorageSecuritySummaryByIdRequest) (*productv1.RequireStorageSecuritySummaryByIdResponse, error) {
	results := make([]*productv1.RequireStorageSecuritySummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		setup := fields.GetSecuritySetup()

		itemSummary := "Unknown item"
		if item := setup.GetPrimaryItem(); item != nil {
			switch v := item.GetInstance().(type) {
			case *productv1.RequireStorageSecuritySummaryByIdFields_SecuritySetup_StorageItem_PalletItem:
				itemSummary = fmt.Sprintf("Pallet: %s (count: %d)", v.PalletItem.GetName(), v.PalletItem.GetPalletCount())
			case *productv1.RequireStorageSecuritySummaryByIdFields_SecuritySetup_StorageItem_ContainerItem:
				itemSummary = fmt.Sprintf("Container: %s (size: %s)", v.ContainerItem.GetName(), v.ContainerItem.GetContainerSize())
			}
		}

		summary := fmt.Sprintf("[%s] %s", setup.GetSecurityLevel(), itemSummary)
		results = append(results, &productv1.RequireStorageSecuritySummaryByIdResult{
			SecuritySummary: summary,
		})
	}

	return &productv1.RequireStorageSecuritySummaryByIdResponse{Result: results}, nil
}

// RequireStorageItemHandlerInfoById implements [productv1.ProductServiceServer].
// Extracts handler name from within interface fragments.
func (s *MockService) RequireStorageItemHandlerInfoById(_ context.Context, req *productv1.RequireStorageItemHandlerInfoByIdRequest) (*productv1.RequireStorageItemHandlerInfoByIdResponse, error) {
	results := make([]*productv1.RequireStorageItemHandlerInfoByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		item := fields.GetPrimaryItem()

		var info string
		switch v := item.GetInstance().(type) {
		case *productv1.RequireStorageItemHandlerInfoByIdFields_StorageItem_PalletItem:
			info = fmt.Sprintf("PalletHandler: %s", v.PalletItem.GetHandler().GetName())
		case *productv1.RequireStorageItemHandlerInfoByIdFields_StorageItem_ContainerItem:
			info = fmt.Sprintf("ContainerHandler: %s", v.ContainerItem.GetHandler().GetName())
		default:
			info = "Unknown handler"
		}

		results = append(results, &productv1.RequireStorageItemHandlerInfoByIdResult{
			ItemHandlerInfo: info,
		})
	}

	return &productv1.RequireStorageItemHandlerInfoByIdResponse{Result: results}, nil
}

// RequireStorageItemSpecsInfoById implements [productv1.ProductServiceServer].
// Extracts specs and dimensions from deep concrete nesting inside interface fragments.
func (s *MockService) RequireStorageItemSpecsInfoById(_ context.Context, req *productv1.RequireStorageItemSpecsInfoByIdRequest) (*productv1.RequireStorageItemSpecsInfoByIdResponse, error) {
	results := make([]*productv1.RequireStorageItemSpecsInfoByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		item := fields.GetPrimaryItem()

		var info string
		switch v := item.GetInstance().(type) {
		case *productv1.RequireStorageItemSpecsInfoByIdFields_StorageItem_PalletItem:
			specs := v.PalletItem.GetSpecs()
			dims := specs.GetDimensions()
			info = fmt.Sprintf("PalletSpecs: %s (%.1fx%.1f)", specs.GetName(), dims.GetLength(), dims.GetWidth())
		case *productv1.RequireStorageItemSpecsInfoByIdFields_StorageItem_ContainerItem:
			specs := v.ContainerItem.GetSpecs()
			dims := specs.GetDimensions()
			info = fmt.Sprintf("ContainerSpecs: %s (%.1fx%.1f)", specs.GetName(), dims.GetLength(), dims.GetWidth())
		default:
			info = "Unknown specs"
		}

		results = append(results, &productv1.RequireStorageItemSpecsInfoByIdResult{
			ItemSpecsInfo: info,
		})
	}

	return &productv1.RequireStorageItemSpecsInfoByIdResponse{Result: results}, nil
}

// RequireStorageDeepItemInfoById implements [productv1.ProductServiceServer].
// Extracts nested abstract type through concrete intermediary (handler → assignedItem).
func (s *MockService) RequireStorageDeepItemInfoById(_ context.Context, req *productv1.RequireStorageDeepItemInfoByIdRequest) (*productv1.RequireStorageDeepItemInfoByIdResponse, error) {
	results := make([]*productv1.RequireStorageDeepItemInfoByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		fields := ctx.GetFields()
		item := fields.GetPrimaryItem()

		var info string
		switch v := item.GetInstance().(type) {
		case *productv1.RequireStorageDeepItemInfoByIdFields_StorageItem_PalletItem:
			handler := v.PalletItem.GetHandler()
			assignedItem := handler.GetAssignedItem()
			switch av := assignedItem.GetInstance().(type) {
			case *productv1.RequireStorageDeepItemInfoByIdFields_PalletItem_ItemHandler_StorageItem_ContainerItem:
				info = fmt.Sprintf("PalletHandler->Container: %s (size: %s)", av.ContainerItem.GetName(), av.ContainerItem.GetContainerSize())
			case *productv1.RequireStorageDeepItemInfoByIdFields_PalletItem_ItemHandler_StorageItem_PalletItem:
				info = fmt.Sprintf("PalletHandler->Pallet: %s (count: %d)", av.PalletItem.GetName(), av.PalletItem.GetPalletCount())
			default:
				info = "PalletHandler->Unknown"
			}
		case *productv1.RequireStorageDeepItemInfoByIdFields_StorageItem_ContainerItem:
			info = fmt.Sprintf("ContainerHandler: %s", v.ContainerItem.GetHandler().GetName())
		default:
			info = "Unknown deep item"
		}

		results = append(results, &productv1.RequireStorageDeepItemInfoByIdResult{
			DeepItemInfo: info,
		})
	}

	return &productv1.RequireStorageDeepItemInfoByIdResponse{Result: results}, nil
}

// newPalletStorageItem builds a fully populated PalletItem wrapped in the StorageItem interface message.
// assignedItem is attached to the item's handler and may be nil to terminate the recursion,
// as StorageItem is reachable from itself via handler.assignedItem.
func newPalletStorageItem(id, name string, palletCount int32, assignedItem *productv1.StorageItem) *productv1.StorageItem {
	return &productv1.StorageItem{
		Instance: &productv1.StorageItem_PalletItem{
			PalletItem: &productv1.PalletItem{
				Id:          id,
				Name:        name,
				Weight:      float64(palletCount) * 12.5,
				PalletCount: palletCount,
				Handler: &productv1.ItemHandler{
					Id:           id + "-handler",
					Name:         "Handler for " + name,
					AssignedItem: assignedItem,
				},
				Specs: &productv1.PalletSpecs{
					Name:      name + " specs",
					MaxWeight: float64(palletCount) * 100,
					Dimensions: &productv1.Dimensions{
						Length: 120.0,
						Width:  80.0,
						Height: 15.0,
					},
				},
			},
		},
	}
}

// newContainerStorageItem builds a fully populated ContainerItem wrapped in the StorageItem interface message.
// assignedItem is attached to the item's handler and may be nil to terminate the recursion,
// as StorageItem is reachable from itself via handler.assignedItem.
func newContainerStorageItem(id, name, containerSize string, assignedItem *productv1.StorageItem) *productv1.StorageItem {
	return &productv1.StorageItem{
		Instance: &productv1.StorageItem_ContainerItem{
			ContainerItem: &productv1.ContainerItem{
				Id:            id,
				Name:          name,
				Weight:        float64(len(containerSize)) * 7.5,
				ContainerSize: containerSize,
				Handler: &productv1.ItemHandler{
					Id:           id + "-handler",
					Name:         "Handler for " + name,
					AssignedItem: assignedItem,
				},
				Specs: &productv1.ContainerSpecs{
					Name:   name + " specs",
					Volume: float64(len(containerSize)) * 33.3,
					Dimensions: &productv1.Dimensions{
						Length: 240.0,
						Width:  120.0,
						Height: 260.0,
					},
				},
			},
		},
	}
}

// newHandledPalletStorageItem builds a PalletItem whose handler is assigned a ContainerItem,
// so that a nested abstract type is reachable within an abstract result.
func newHandledPalletStorageItem(id, name string, palletCount int32) *productv1.StorageItem {
	assigned := newContainerStorageItem(id+"-assigned", name+" assigned container", "20ft", nil)
	return newPalletStorageItem(id, name, palletCount, assigned)
}

// newHandledContainerStorageItem builds a ContainerItem whose handler is assigned a PalletItem,
// so that a nested abstract type is reachable within an abstract result.
func newHandledContainerStorageItem(id, name, containerSize string) *productv1.StorageItem {
	assigned := newPalletStorageItem(id+"-assigned", name+" assigned pallet", 7, nil)
	return newContainerStorageItem(id, name, containerSize, assigned)
}

// RequireStorageRecommendedItemById implements [productv1.ProductServiceServer].
// Returns an interface (StorageItem) derived from the required metadata fields.
func (s *MockService) RequireStorageRecommendedItemById(_ context.Context, req *productv1.RequireStorageRecommendedItemByIdRequest) (*productv1.RequireStorageRecommendedItemByIdResponse, error) {
	results := make([]*productv1.RequireStorageRecommendedItemByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		metadata := ctx.GetFields().GetMetadata()
		capacity := metadata.GetCapacity()
		zone := metadata.GetZone()

		// High capacity storages get a pallet recommendation, everything else a container.
		var item *productv1.StorageItem
		if capacity > 100 {
			item = newHandledPalletStorageItem(
				fmt.Sprintf("pallet-%s-%d", zone, capacity),
				fmt.Sprintf("Pallet for zone %s", zone),
				capacity/10,
			)
		} else {
			item = newHandledContainerStorageItem(
				fmt.Sprintf("container-%s-%d", zone, capacity),
				fmt.Sprintf("Container for zone %s", zone),
				fmt.Sprintf("%dL", capacity),
			)
		}

		results = append(results, &productv1.RequireStorageRecommendedItemByIdResult{
			RecommendedItem: item,
		})
	}

	return &productv1.RequireStorageRecommendedItemByIdResponse{Result: results}, nil
}

// RequireStorageRecommendedItemsById implements [productv1.ProductServiceServer].
// Returns a list of interfaces (StorageItem), one entry per required tag.
func (s *MockService) RequireStorageRecommendedItemsById(_ context.Context, req *productv1.RequireStorageRecommendedItemsByIdRequest) (*productv1.RequireStorageRecommendedItemsByIdResponse, error) {
	results := make([]*productv1.RequireStorageRecommendedItemsByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		items := make([]*productv1.StorageItem, 0, len(tags))
		for i, tag := range tags {
			// Alternate between both concrete types so every list contains a mix.
			if i%2 == 0 {
				items = append(items, newHandledPalletStorageItem(
					fmt.Sprintf("pallet-%s", tag),
					fmt.Sprintf("Pallet %s", tag),
					int32(i+1),
				))
			} else {
				items = append(items, newHandledContainerStorageItem(
					fmt.Sprintf("container-%s", tag),
					fmt.Sprintf("Container %s", tag),
					strings.ToUpper(tag),
				))
			}
		}

		results = append(results, &productv1.RequireStorageRecommendedItemsByIdResult{
			RecommendedItems: items,
		})
	}

	return &productv1.RequireStorageRecommendedItemsByIdResponse{Result: results}, nil
}

// RequireStorageLatestOperationById implements [productv1.ProductServiceServer].
// Returns a union (StorageOperationResult) derived from the required storageKind enum.
func (s *MockService) RequireStorageLatestOperationById(_ context.Context, req *productv1.RequireStorageLatestOperationByIdRequest) (*productv1.RequireStorageLatestOperationByIdResponse, error) {
	results := make([]*productv1.RequireStorageLatestOperationByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		kind := ctx.GetFields().GetStorageKind()

		// Known kinds report a success, everything else a failure.
		operation := &productv1.StorageOperationResult{}
		switch kind {
		case productv1.CategoryKind_CATEGORY_KIND_BOOK,
			productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
			productv1.CategoryKind_CATEGORY_KIND_FURNITURE:
			operation.Value = &productv1.StorageOperationResult_StorageSuccess{
				StorageSuccess: &productv1.StorageSuccess{
					Message:     fmt.Sprintf("Operation completed for %s", kind.String()),
					CompletedAt: "2024-01-01T00:00:00Z",
				},
			}
		default:
			operation.Value = &productv1.StorageOperationResult_StorageFailure{
				StorageFailure: &productv1.StorageFailure{
					Message:   fmt.Sprintf("Operation failed for %s", kind.String()),
					ErrorCode: "UNSUPPORTED_KIND",
				},
			}
		}

		results = append(results, &productv1.RequireStorageLatestOperationByIdResult{
			LatestOperation: operation,
		})
	}

	return &productv1.RequireStorageLatestOperationByIdResponse{Result: results}, nil
}

// RequireStorageOptionalLatestOperationById implements [productv1.ProductServiceServer].
// Returns a nullable union (StorageOperationResult) derived from the required optionalTags.
func (s *MockService) RequireStorageOptionalLatestOperationById(_ context.Context, req *productv1.RequireStorageOptionalLatestOperationByIdRequest) (*productv1.RequireStorageOptionalLatestOperationByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalLatestOperationByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		optionalTags := ctx.GetFields().GetOptionalTags()

		var operation *productv1.StorageOperationResult
		items := optionalTags.GetList().GetItems()
		switch {
		case len(items) == 0:
			// No tags provided, the operation is unknown.
			operation = nil
		case len(items)%2 == 0:
			operation = &productv1.StorageOperationResult{
				Value: &productv1.StorageOperationResult_StorageFailure{
					StorageFailure: &productv1.StorageFailure{
						Message:   fmt.Sprintf("Operation failed for tags: %s", strings.Join(items, ", ")),
						ErrorCode: "EVEN_TAG_COUNT",
					},
				},
			}
		default:
			operation = &productv1.StorageOperationResult{
				Value: &productv1.StorageOperationResult_StorageSuccess{
					StorageSuccess: &productv1.StorageSuccess{
						Message:     fmt.Sprintf("Operation completed for tags: %s", strings.Join(items, ", ")),
						CompletedAt: "2024-01-02T00:00:00Z",
					},
				},
			}
		}

		results = append(results, &productv1.RequireStorageOptionalLatestOperationByIdResult{
			OptionalLatestOperation: operation,
		})
	}

	return &productv1.RequireStorageOptionalLatestOperationByIdResponse{Result: results}, nil
}

// RequireStorageMultiFilteredTagSummaryById implements [productv1.ProductServiceServer].
// Returns a comma separated list of tags matching any of the given prefixes, capped at maxResults.
func (s *MockService) RequireStorageMultiFilteredTagSummaryById(_ context.Context, req *productv1.RequireStorageMultiFilteredTagSummaryByIdRequest) (*productv1.RequireStorageMultiFilteredTagSummaryByIdResponse, error) {
	prefixes := req.GetFieldArgs().GetPrefixes()
	maxResults := int(req.GetFieldArgs().GetMaxResults())
	results := make([]*productv1.RequireStorageMultiFilteredTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		filteredTags := make([]string, 0, len(tags))
		for _, tag := range tags {
			for _, p := range prefixes {
				if strings.HasPrefix(tag, p) {
					filteredTags = append(filteredTags, tag)
					break
				}
			}
			if len(filteredTags) >= maxResults {
				break
			}
		}

		var summary *wrapperspb.StringValue
		if len(filteredTags) > 0 {
			summary = &wrapperspb.StringValue{Value: strings.Join(filteredTags, ", ")}
		}

		results = append(results, &productv1.RequireStorageMultiFilteredTagSummaryByIdResult{
			MultiFilteredTagSummary: summary,
		})
	}

	return &productv1.RequireStorageMultiFilteredTagSummaryByIdResponse{Result: results}, nil
}

// RequireStorageNullableFilteredTagSummaryById implements [productv1.ProductServiceServer].
// Returns a comma separated list of tags matching an optional prefix. If prefix is nil, all tags are returned.
func (s *MockService) RequireStorageNullableFilteredTagSummaryById(_ context.Context, req *productv1.RequireStorageNullableFilteredTagSummaryByIdRequest) (*productv1.RequireStorageNullableFilteredTagSummaryByIdResponse, error) {
	prefixArg := req.GetFieldArgs().GetPrefix()
	results := make([]*productv1.RequireStorageNullableFilteredTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		var filteredTags []string
		if prefixArg == nil {
			filteredTags = tags
		} else {
			for _, tag := range tags {
				if strings.HasPrefix(tag, prefixArg.GetValue()) {
					filteredTags = append(filteredTags, tag)
				}
			}
		}

		var summary *wrapperspb.StringValue
		if len(filteredTags) > 0 {
			summary = &wrapperspb.StringValue{Value: strings.Join(filteredTags, ", ")}
		}

		results = append(results, &productv1.RequireStorageNullableFilteredTagSummaryByIdResult{
			NullableFilteredTagSummary: summary,
		})
	}

	return &productv1.RequireStorageNullableFilteredTagSummaryByIdResponse{Result: results}, nil
}

// RequireStorageFilteredTagSummaryById implements [productv1.ProductServiceServer].
// Returns a comma separated list of tags having a specific prefix as given by field argument "prefix".
func (s *MockService) RequireStorageFilteredTagSummaryById(_ context.Context, req *productv1.RequireStorageFilteredTagSummaryByIdRequest) (*productv1.RequireStorageFilteredTagSummaryByIdResponse, error) {
	prefix := req.GetFieldArgs().GetPrefix()
	results := make([]*productv1.RequireStorageFilteredTagSummaryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		filteredTags := make([]string, 0, len(tags))
		for _, tag := range tags {
			if strings.HasPrefix(tag, prefix) {
				filteredTags = append(filteredTags, tag)
			}
		}

		var filteredTagSummary *wrapperspb.StringValue
		if len(filteredTags) > 0 {
			filteredTagSummary = &wrapperspb.StringValue{Value: strings.Join(filteredTags, ", ")}
		}

		results = append(results, &productv1.RequireStorageFilteredTagSummaryByIdResult{
			FilteredTagSummary: filteredTagSummary,
		})
	}

	return &productv1.RequireStorageFilteredTagSummaryByIdResponse{Result: results}, nil
}

// RequireStorageOptionalProcessedMetadataHistoryById implements [productv1.ProductServiceServer].
// Returns a nullable list of complex types (StorageMetadata) wrapped in a ListOfStorageMetadata
// message, so a null list can be distinguished from an empty one:
//   - no metadata history in the context -> null list
//   - odd context index                  -> empty (but non-null) list
//   - otherwise                          -> processed history
func (s *MockService) RequireStorageOptionalProcessedMetadataHistoryById(_ context.Context, req *productv1.RequireStorageOptionalProcessedMetadataHistoryByIdRequest) (*productv1.RequireStorageOptionalProcessedMetadataHistoryByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalProcessedMetadataHistoryByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		metadataHistory := ctx.GetFields().GetMetadataHistory()

		var processedHistory *productv1.ListOfStorageMetadata
		switch {
		case len(metadataHistory) == 0:
			// null list
		case i%2 == 1:
			processedHistory = &productv1.ListOfStorageMetadata{
				List: &productv1.ListOfStorageMetadata_List{},
			}
		default:
			items := make([]*productv1.StorageMetadata, 0, len(metadataHistory))
			for j, metadata := range metadataHistory {
				items = append(items, &productv1.StorageMetadata{
					Capacity: metadata.GetCapacity() * int32(j+1),
					Zone:     "OPT_HIST_" + metadata.GetZone(),
					Priority: int32(j + 1),
				})
			}
			processedHistory = &productv1.ListOfStorageMetadata{
				List: &productv1.ListOfStorageMetadata_List{Items: items},
			}
		}

		results = append(results, &productv1.RequireStorageOptionalProcessedMetadataHistoryByIdResult{
			OptionalProcessedMetadataHistory: processedHistory,
		})
	}

	return &productv1.RequireStorageOptionalProcessedMetadataHistoryByIdResponse{Result: results}, nil
}

// RequireStorageOptionalRecommendedItemsById implements [productv1.ProductServiceServer].
// Returns a nullable list of interfaces (StorageItem) wrapped in a ListOfStorageItem message:
//   - no tags in the context -> null list
//   - odd context index      -> empty (but non-null) list
//   - otherwise              -> one item per tag, alternating between both concrete types
func (s *MockService) RequireStorageOptionalRecommendedItemsById(_ context.Context, req *productv1.RequireStorageOptionalRecommendedItemsByIdRequest) (*productv1.RequireStorageOptionalRecommendedItemsByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalRecommendedItemsByIdResult, 0, len(req.GetContext()))

	for i, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		var recommendedItems *productv1.ListOfStorageItem
		switch {
		case len(tags) == 0:
			// null list
		case i%2 == 1:
			recommendedItems = &productv1.ListOfStorageItem{
				List: &productv1.ListOfStorageItem_List{},
			}
		default:
			items := make([]*productv1.StorageItem, 0, len(tags))
			for j, tag := range tags {
				if j%2 == 0 {
					items = append(items, newHandledPalletStorageItem(
						fmt.Sprintf("opt-pallet-%s", tag),
						fmt.Sprintf("Optional pallet %s", tag),
						int32(j+1),
					))
				} else {
					items = append(items, newHandledContainerStorageItem(
						fmt.Sprintf("opt-container-%s", tag),
						fmt.Sprintf("Optional container %s", tag),
						strings.ToUpper(tag),
					))
				}
			}
			recommendedItems = &productv1.ListOfStorageItem{
				List: &productv1.ListOfStorageItem_List{Items: items},
			}
		}

		results = append(results, &productv1.RequireStorageOptionalRecommendedItemsByIdResult{
			OptionalRecommendedItems: recommendedItems,
		})
	}

	return &productv1.RequireStorageOptionalRecommendedItemsByIdResponse{Result: results}, nil
}

// RequireStorageOptionalOperationHistoryById implements [productv1.ProductServiceServer].
// Returns a nullable list of unions (StorageOperationResult) wrapped in a
// ListOfStorageOperationResult message, derived from the required storageKind enum:
//   - BOOK / ELECTRONICS -> a success followed by a failure
//   - FURNITURE          -> empty (but non-null) list
//   - OTHER / unspecified -> null list
func (s *MockService) RequireStorageOptionalOperationHistoryById(_ context.Context, req *productv1.RequireStorageOptionalOperationHistoryByIdRequest) (*productv1.RequireStorageOptionalOperationHistoryByIdResponse, error) {
	results := make([]*productv1.RequireStorageOptionalOperationHistoryByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		kind := ctx.GetFields().GetStorageKind()

		var operationHistory *productv1.ListOfStorageOperationResult
		switch kind {
		case productv1.CategoryKind_CATEGORY_KIND_BOOK,
			productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS:
			operationHistory = &productv1.ListOfStorageOperationResult{
				List: &productv1.ListOfStorageOperationResult_List{
					Items: []*productv1.StorageOperationResult{
						{
							Value: &productv1.StorageOperationResult_StorageSuccess{
								StorageSuccess: &productv1.StorageSuccess{
									Message:     fmt.Sprintf("History entry completed for %s", kind.String()),
									CompletedAt: "2024-01-03T00:00:00Z",
								},
							},
						},
						{
							Value: &productv1.StorageOperationResult_StorageFailure{
								StorageFailure: &productv1.StorageFailure{
									Message:   fmt.Sprintf("History entry failed for %s", kind.String()),
									ErrorCode: "HISTORIC_FAILURE",
								},
							},
						},
					},
				},
			}
		case productv1.CategoryKind_CATEGORY_KIND_FURNITURE:
			operationHistory = &productv1.ListOfStorageOperationResult{
				List: &productv1.ListOfStorageOperationResult_List{},
			}
		default:
			// OTHER and unspecified produce a null list.
		}

		results = append(results, &productv1.RequireStorageOptionalOperationHistoryByIdResult{
			OptionalOperationHistory: operationHistory,
		})
	}

	return &productv1.RequireStorageOptionalOperationHistoryByIdResponse{Result: results}, nil
}

// RequireStorageTagsByLengthsById implements [productv1.ProductServiceServer].
// Both the nullable list field argument and the nullable list return type are carried in
// ListOfX wrappers:
//   - the lengths argument is null -> null list
//   - the lengths argument is empty -> empty (but non-null) list
//   - otherwise -> the tags whose length matches one of the given lengths
func (s *MockService) RequireStorageTagsByLengthsById(_ context.Context, req *productv1.RequireStorageTagsByLengthsByIdRequest) (*productv1.RequireStorageTagsByLengthsByIdResponse, error) {
	lengths := req.GetFieldArgs().GetLengths()
	results := make([]*productv1.RequireStorageTagsByLengthsByIdResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		tags := ctx.GetFields().GetTags()

		var tagsByLengths *productv1.ListOfString
		if lengths.GetList() != nil {
			matched := make([]string, 0, len(tags))
			for _, tag := range tags {
				if slices.Contains(lengths.GetList().GetItems(), int32(len(tag))) {
					matched = append(matched, tag)
				}
			}
			tagsByLengths = &productv1.ListOfString{
				List: &productv1.ListOfString_List{Items: matched},
			}
		}

		results = append(results, &productv1.RequireStorageTagsByLengthsByIdResult{
			TagsByLengths: tagsByLengths,
		})
	}

	return &productv1.RequireStorageTagsByLengthsByIdResponse{Result: results}, nil
}

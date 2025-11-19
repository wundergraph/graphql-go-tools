package grpctest

import (
	context "context"
	"fmt"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// Implementation for QueryCategories
func (s *MockService) QueryCategories(ctx context.Context, in *productv1.QueryCategoriesRequest) (*productv1.QueryCategoriesResponse, error) {
	// Generate a list of categories
	var categories []*productv1.Category

	// Create sample categories for each CategoryKind
	categoryKinds := []productv1.CategoryKind{
		productv1.CategoryKind_CATEGORY_KIND_BOOK,
		productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
		productv1.CategoryKind_CATEGORY_KIND_FURNITURE,
		productv1.CategoryKind_CATEGORY_KIND_OTHER,
	}

	for i, kind := range categoryKinds {
		categories = append(categories, &productv1.Category{
			Id:            fmt.Sprintf("category-%d", i+1),
			Name:          fmt.Sprintf("%s Category", kind.String()),
			Kind:          kind,
			Subcategories: createSubcategories(fmt.Sprintf("category-%d", i+1), kind, i+1),
		})
	}

	return &productv1.QueryCategoriesResponse{
		Categories: categories,
	}, nil
}

// Implementation for QueryCategoriesByKind
func (s *MockService) QueryCategoriesByKind(ctx context.Context, in *productv1.QueryCategoriesByKindRequest) (*productv1.QueryCategoriesByKindResponse, error) {
	kind := in.GetKind()

	// Generate categories for the specified kind
	var categories []*productv1.Category

	// Create 3 categories of the requested kind
	for i := 1; i <= 3; i++ {

		subcategories := make([]*productv1.Subcategory, 0, i)
		for j := 1; j <= i; j++ {
			subcategories = append(subcategories, &productv1.Subcategory{
				Id:          fmt.Sprintf("%s-subcategory-%d", kind.String(), j),
				Name:        fmt.Sprintf("%s Subcategory %d", kind.String(), j),
				Description: &wrapperspb.StringValue{Value: fmt.Sprintf("%s Subcategory %d", kind.String(), j)},
				IsActive:    true,
			})
		}

		categories = append(categories, &productv1.Category{
			Id:   fmt.Sprintf("%s-category-%d", kind.String(), i),
			Name: fmt.Sprintf("%s Category %d", kind.String(), i),
			Kind: kind,
			Subcategories: &productv1.ListOfSubcategory{
				List: &productv1.ListOfSubcategory_List{
					Items: subcategories,
				},
			},
		})
	}

	return &productv1.QueryCategoriesByKindResponse{
		CategoriesByKind: categories,
	}, nil
}

func (s *MockService) QueryCategoriesByKinds(ctx context.Context, in *productv1.QueryCategoriesByKindsRequest) (*productv1.QueryCategoriesByKindsResponse, error) {
	kinds := in.GetKinds()

	var categories []*productv1.Category

	for i, kind := range kinds {
		categories = append(categories, &productv1.Category{
			Id:            fmt.Sprintf("%s-category-%d", kind.String(), i),
			Name:          fmt.Sprintf("%s Category %d", kind.String(), i),
			Kind:          kind,
			Subcategories: createSubcategories(fmt.Sprintf("%s-category-%d", kind.String(), i), kind, i+1),
		})
	}

	return &productv1.QueryCategoriesByKindsResponse{
		CategoriesByKinds: categories,
	}, nil
}

// Implementation for QueryFilterCategories
func (s *MockService) QueryFilterCategories(ctx context.Context, in *productv1.QueryFilterCategoriesRequest) (*productv1.QueryFilterCategoriesResponse, error) {
	filter := in.GetFilter()

	if filter == nil {
		return &productv1.QueryFilterCategoriesResponse{
			FilterCategories: []*productv1.Category{},
		}, nil
	}

	kind := filter.GetCategory()

	// Generate filtered categories
	var categories []*productv1.Category

	// Create categories that match the filter
	for i := 1; i <= 5; i++ {
		categories = append(categories, &productv1.Category{
			Id:            fmt.Sprintf("filtered-%s-category-%d", kind.String(), i),
			Name:          fmt.Sprintf("Filtered %s Category %d", kind.String(), i),
			Kind:          kind,
			Subcategories: createSubcategories(fmt.Sprintf("filtered-%s-category-%d", kind.String(), i), kind, i),
		})
	}

	// Apply pagination if provided
	pagination := filter.GetPagination()
	if pagination != nil {
		page := int(pagination.GetPage())
		perPage := int(pagination.GetPerPage())

		if page > 0 && perPage > 0 {
			startIdx := (page - 1) * perPage
			endIdx := startIdx + perPage

			if startIdx < len(categories) {
				if endIdx > len(categories) {
					endIdx = len(categories)
				}
				categories = categories[startIdx:endIdx]
			} else {
				categories = []*productv1.Category{}
			}
		}
	}

	return &productv1.QueryFilterCategoriesResponse{
		FilterCategories: categories,
	}, nil
}

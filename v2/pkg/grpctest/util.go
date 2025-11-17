package grpctest

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Helper function to create subcategories for a category
func createSubcategories(categoryId string, kind productv1.CategoryKind, count int) *productv1.ListOfSubcategory {
	if count <= 0 {
		return &productv1.ListOfSubcategory{
			List: &productv1.ListOfSubcategory_List{
				Items: []*productv1.Subcategory{},
			},
		}
	}

	subcategories := make([]*productv1.Subcategory, 0, count)
	for j := 1; j <= count; j++ {
		subcategories = append(subcategories, &productv1.Subcategory{
			Id:          fmt.Sprintf("%s-subcategory-%d", categoryId, j),
			Name:        fmt.Sprintf("%s Subcategory %d", kind.String(), j),
			Description: &wrapperspb.StringValue{Value: fmt.Sprintf("Subcategory %d for %s", j, categoryId)},
			IsActive:    true,
		})
	}

	return &productv1.ListOfSubcategory{
		List: &productv1.ListOfSubcategory_List{
			Items: subcategories,
		},
	}
}

// Helper functions to convert input types to output types
func convertCategoryInputsToCategories(inputs []*productv1.CategoryInput) []*productv1.Category {
	if inputs == nil {
		return nil
	}
	results := make([]*productv1.Category, len(inputs))
	for i, input := range inputs {
		results[i] = &productv1.Category{
			Id:            fmt.Sprintf("cat-input-%d", i),
			Name:          input.GetName(),
			Kind:          input.GetKind(),
			Subcategories: createSubcategories(fmt.Sprintf("cat-input-%d", i), input.GetKind(), i+1),
		}
	}
	return results
}

func convertCategoryInputListToCategories(inputs *productv1.ListOfCategoryInput) []*productv1.Category {
	if inputs == nil || inputs.List == nil || inputs.List.Items == nil {
		return nil
	}
	results := make([]*productv1.Category, len(inputs.List.Items))
	for i, input := range inputs.List.Items {
		results[i] = &productv1.Category{
			Id:            fmt.Sprintf("cat-list-input-%d", i),
			Name:          input.GetName(),
			Kind:          input.GetKind(),
			Subcategories: createSubcategories(fmt.Sprintf("cat-list-input-%d", i), input.GetKind(), i+1),
		}
	}
	return results
}

func convertUserInputsToUsers(inputs *productv1.ListOfUserInput) []*productv1.User {
	if inputs == nil || inputs.List == nil || inputs.List.Items == nil {
		return nil
	}
	results := make([]*productv1.User, len(inputs.List.Items))
	for i, input := range inputs.List.Items {
		results[i] = &productv1.User{
			Id:   fmt.Sprintf("user-input-%d", i),
			Name: input.GetName(),
		}
	}
	return results
}

func convertNestedUserInputsToUsers(nestedInputs *productv1.ListOfListOfUserInput) *productv1.ListOfListOfUser {
	if nestedInputs == nil || nestedInputs.List == nil {
		return &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{},
			},
		}
	}

	results := make([]*productv1.ListOfUser, len(nestedInputs.List.Items))
	for i, userList := range nestedInputs.List.Items {
		users := make([]*productv1.User, len(userList.List.Items))
		for j, userInput := range userList.List.Items {
			users[j] = &productv1.User{
				Id:   fmt.Sprintf("nested-user-%d-%d", i, j),
				Name: userInput.GetName(),
			}
		}
		results[i] = &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: users,
			},
		}
	}

	return &productv1.ListOfListOfUser{
		List: &productv1.ListOfListOfUser_List{
			Items: results,
		},
	}
}

func convertNestedCategoryInputsToCategories(nestedInputs *productv1.ListOfListOfCategoryInput) *productv1.ListOfListOfCategory {
	if nestedInputs == nil || nestedInputs.List == nil {
		return &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{},
			},
		}
	}

	results := make([]*productv1.ListOfCategory, len(nestedInputs.List.Items))
	for i, categoryList := range nestedInputs.List.Items {
		categories := make([]*productv1.Category, len(categoryList.List.Items))
		for j, categoryInput := range categoryList.List.Items {
			categories[j] = &productv1.Category{
				Id:            fmt.Sprintf("nested-cat-%d-%d", i, j),
				Name:          categoryInput.GetName(),
				Kind:          categoryInput.GetKind(),
				Subcategories: createSubcategories(fmt.Sprintf("nested-cat-%d-%d", i, j), categoryInput.GetKind(), j+1),
			}
		}
		results[i] = &productv1.ListOfCategory{
			List: &productv1.ListOfCategory_List{
				Items: categories,
			},
		}
	}

	return &productv1.ListOfListOfCategory{
		List: &productv1.ListOfListOfCategory_List{
			Items: results,
		},
	}
}

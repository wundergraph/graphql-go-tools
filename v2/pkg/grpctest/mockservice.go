package grpctest

import (
	context "context"
	"fmt"
	"math/rand"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

var _ productv1.ProductServiceServer = &MockService{}

type MockService struct {
	productv1.UnimplementedProductServiceServer
}

// QueryTestContainer implements productv1.ProductServiceServer.
func (s *MockService) QueryTestContainer(_ context.Context, req *productv1.QueryTestContainerRequest) (*productv1.QueryTestContainerResponse, error) {
	id := req.GetId()

	return &productv1.QueryTestContainerResponse{
		TestContainer: &productv1.TestContainer{
			Id:          id,
			Name:        fmt.Sprintf("TestContainer-%s", id),
			Description: &wrapperspb.StringValue{Value: fmt.Sprintf("Description for TestContainer %s", id)},
		},
	}, nil
}

// QueryTestContainers implements productv1.ProductServiceServer.
func (s *MockService) QueryTestContainers(_ context.Context, _ *productv1.QueryTestContainersRequest) (*productv1.QueryTestContainersResponse, error) {
	var containers []*productv1.TestContainer

	// Generate 3 test containers
	for i := 1; i <= 3; i++ {
		containers = append(containers, &productv1.TestContainer{
			Id:          fmt.Sprintf("container-%d", i),
			Name:        fmt.Sprintf("TestContainer %d", i),
			Description: &wrapperspb.StringValue{Value: fmt.Sprintf("Description for container %d", i)},
		})
	}

	return &productv1.QueryTestContainersResponse{
		TestContainers: containers,
	}, nil
}

// MutationPerformAction implements productv1.ProductServiceServer.
func (s *MockService) MutationPerformAction(ctx context.Context, in *productv1.MutationPerformActionRequest) (*productv1.MutationPerformActionResponse, error) {
	input := in.GetInput()
	actionType := input.GetType()

	// Simulate different action results based on the action type
	var result *productv1.ActionResult

	switch actionType {
	case "error_action":
		// Return an error result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionError{
				ActionError: &productv1.ActionError{
					Message: "Action failed due to validation error",
					Code:    "VALIDATION_ERROR",
				},
			},
		}
	case "invalid_action":
		// Return a different error result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionError{
				ActionError: &productv1.ActionError{
					Message: "Invalid action type provided",
					Code:    "INVALID_ACTION",
				},
			},
		}
	default:
		// Return a success result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionSuccess{
				ActionSuccess: &productv1.ActionSuccess{
					Message:   fmt.Sprintf("Action '%s' completed successfully", actionType),
					Timestamp: "2024-01-01T00:00:00Z",
				},
			},
		}
	}

	return &productv1.MutationPerformActionResponse{
		PerformAction: result,
	}, nil
}

// QueryRandomSearchResult implements productv1.ProductServiceServer.
func (s *MockService) QueryRandomSearchResult(ctx context.Context, in *productv1.QueryRandomSearchResultRequest) (*productv1.QueryRandomSearchResultResponse, error) {
	// Randomly return one of the three union types
	var result *productv1.SearchResult

	switch rand.Intn(3) {
	case 0:
		// Return a Product
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_Product{
				Product: &productv1.Product{
					Id:    "product-random-1",
					Name:  "Random Product",
					Price: 29.99,
				},
			},
		}
	case 1:
		// Return a User
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_User{
				User: &productv1.User{
					Id:   "user-random-1",
					Name: "Random User",
				},
			},
		}
	default:
		// Return a Category
		kind := productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_Category{
				Category: &productv1.Category{
					Id:            "category-random-1",
					Name:          "Random Category",
					Kind:          kind,
					Subcategories: createSubcategories("category-random-1", kind, 2),
				},
			},
		}
	}

	return &productv1.QueryRandomSearchResultResponse{
		RandomSearchResult: result,
	}, nil
}

// QuerySearch implements productv1.ProductServiceServer.
func (s *MockService) QuerySearch(ctx context.Context, in *productv1.QuerySearchRequest) (*productv1.QuerySearchResponse, error) {
	input := in.GetInput()
	query := input.GetQuery()
	limit := input.GetLimit()

	// Default limit if not specified
	if limit.GetValue() <= 0 {
		limit = &wrapperspb.Int32Value{Value: 10}
	}

	var results []*productv1.SearchResult

	// Generate a mix of different union types based on the query
	for i := int32(0); i < limit.GetValue() && i < 6; i++ { // Cap at 6 results for testing
		switch i % 3 {
		case 0:
			// Add a Product
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_Product{
					Product: &productv1.Product{
						Id:    fmt.Sprintf("product-search-%d", i+1),
						Name:  fmt.Sprintf("Product matching '%s' #%d", query, i+1),
						Price: float64(10 + i*5),
					},
				},
			})
		case 1:
			// Add a User
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_User{
					User: &productv1.User{
						Id:   fmt.Sprintf("user-search-%d", i+1),
						Name: fmt.Sprintf("User matching '%s' #%d", query, i+1),
					},
				},
			})
		case 2:
			// Add a Category
			kinds := []productv1.CategoryKind{
				productv1.CategoryKind_CATEGORY_KIND_BOOK,
				productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
				productv1.CategoryKind_CATEGORY_KIND_FURNITURE,
			}
			kind := kinds[i%int32(len(kinds))]
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_Category{
					Category: &productv1.Category{
						Id:            fmt.Sprintf("category-search-%d", i+1),
						Name:          fmt.Sprintf("Category matching '%s' #%d", query, i+1),
						Kind:          kind,
						Subcategories: createSubcategories(fmt.Sprintf("category-search-%d", i+1), kind, int(i%3)+1),
					},
				},
			})
		}
	}

	return &productv1.QuerySearchResponse{
		Search: results,
	}, nil
}

func (s *MockService) QueryUsers(ctx context.Context, in *productv1.QueryUsersRequest) (*productv1.QueryUsersResponse, error) {
	var results []*productv1.User

	// Generate 3 random users
	for i := 1; i <= 3; i++ {
		results = append(results, &productv1.User{
			Id:   fmt.Sprintf("user-%d", i),
			Name: fmt.Sprintf("User %d", i),
		})
	}

	return &productv1.QueryUsersResponse{
		Users: results,
	}, nil
}

func (s *MockService) QueryUser(ctx context.Context, in *productv1.QueryUserRequest) (*productv1.QueryUserResponse, error) {
	userId := in.GetId()

	// Return a gRPC status error for a specific test case
	if userId == "error-user" {
		return nil, status.Errorf(codes.NotFound, "user not found: %s", userId)
	}

	return &productv1.QueryUserResponse{
		User: &productv1.User{
			Id:   userId,
			Name: fmt.Sprintf("User %s", userId),
		},
	}, nil
}

func (s *MockService) QueryNestedType(ctx context.Context, in *productv1.QueryNestedTypeRequest) (*productv1.QueryNestedTypeResponse, error) {
	var nestedTypes []*productv1.NestedTypeA

	// Generate 2 nested types
	for i := 1; i <= 2; i++ {
		nestedTypes = append(nestedTypes, &productv1.NestedTypeA{
			Id:   fmt.Sprintf("nested-a-%d", i),
			Name: fmt.Sprintf("Nested A %d", i),
			B: &productv1.NestedTypeB{
				Id:   fmt.Sprintf("nested-b-%d", i),
				Name: fmt.Sprintf("Nested B %d", i),
				C: &productv1.NestedTypeC{
					Id:   fmt.Sprintf("nested-c-%d", i),
					Name: fmt.Sprintf("Nested C %d", i),
				},
			},
		})
	}

	return &productv1.QueryNestedTypeResponse{
		NestedType: nestedTypes,
	}, nil
}

func (s *MockService) QueryRecursiveType(ctx context.Context, in *productv1.QueryRecursiveTypeRequest) (*productv1.QueryRecursiveTypeResponse, error) {
	// Create a recursive structure 3 levels deep
	recursiveType := &productv1.RecursiveType{
		Id:   "recursive-1",
		Name: "Level 1",
		RecursiveType: &productv1.RecursiveType{
			Id:   "recursive-2",
			Name: "Level 2",
			RecursiveType: &productv1.RecursiveType{
				Id:   "recursive-3",
				Name: "Level 3",
			},
		},
	}

	return &productv1.QueryRecursiveTypeResponse{
		RecursiveType: recursiveType,
	}, nil
}

func (s *MockService) QueryTypeFilterWithArguments(ctx context.Context, in *productv1.QueryTypeFilterWithArgumentsRequest) (*productv1.QueryTypeFilterWithArgumentsResponse, error) {
	filterField1 := in.GetFilterField_1()
	filterField2 := in.GetFilterField_2()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Create results that echo the filter values
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:            fmt.Sprintf("multi-filter-%d", i),
			Name:          fmt.Sprintf("MultiFilter %d", i),
			FilterField_1: filterField1,
			FilterField_2: filterField2,
		})
	}

	return &productv1.QueryTypeFilterWithArgumentsResponse{
		TypeFilterWithArguments: fields,
	}, nil
}

func (s *MockService) QueryTypeWithMultipleFilterFields(ctx context.Context, in *productv1.QueryTypeWithMultipleFilterFieldsRequest) (*productv1.QueryTypeWithMultipleFilterFieldsResponse, error) {
	filter := in.GetFilter()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Echo the filter values in the results
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:            fmt.Sprintf("filtered-%d", i),
			Name:          "Filter: " + strconv.Itoa(i),
			FilterField_1: filter.FilterField_1,
			FilterField_2: filter.FilterField_2,
		})
	}

	return &productv1.QueryTypeWithMultipleFilterFieldsResponse{
		TypeWithMultipleFilterFields: fields,
	}, nil
}

func (s *MockService) QueryComplexFilterType(ctx context.Context, in *productv1.QueryComplexFilterTypeRequest) (*productv1.QueryComplexFilterTypeResponse, error) {
	filter := in.GetFilter()

	var name string
	if filter != nil && filter.GetFilter() != nil {
		name = filter.GetFilter().GetName()
	} else {
		name = "Default Product"
	}

	return &productv1.QueryComplexFilterTypeResponse{
		ComplexFilterType: []*productv1.TypeWithComplexFilterInput{
			{
				Id:   "test-id-123",
				Name: name,
			},
		},
	}, nil
}

func (s *MockService) QueryRandomPet(ctx context.Context, in *productv1.QueryRandomPetRequest) (*productv1.QueryRandomPetResponse, error) {
	// Create either a cat or dog randomly
	var pet *productv1.Animal

	// Random choice between cat and dog
	if rand.Intn(2) == 0 {
		// Create a cat
		pet = &productv1.Animal{
			Instance: &productv1.Animal_Cat{
				Cat: &productv1.Cat{
					Id:         "cat-1",
					Name:       "Whiskers",
					Kind:       "Siamese",
					MeowVolume: int32(rand.Intn(10) + 1), // Random volume between 1-10
					Owner: &productv1.Owner{
						Id:   "owner-cat-1",
						Name: "Alice Johnson",
						Contact: &productv1.ContactInfo{
							Email: "alice@example.com",
							Phone: "555-100-2000",
							Address: &productv1.Address{
								Street:  "10 Cat Street",
								City:    "Catville",
								Country: "USA",
								ZipCode: "10101",
							},
						},
					},
					Breed: &productv1.CatBreed{
						Id:     "breed-cat-1",
						Name:   "Siamese",
						Origin: "Thailand",
						Characteristics: &productv1.BreedCharacteristics{
							Size:        "Medium",
							Temperament: "Vocal and Active",
							Lifespan:    "15-20 years",
						},
					},
				},
			},
		}
	} else {
		// Create a dog
		pet = &productv1.Animal{
			Instance: &productv1.Animal_Dog{
				Dog: &productv1.Dog{
					Id:         "dog-1",
					Name:       "Spot",
					Kind:       "Dalmatian",
					BarkVolume: int32(rand.Intn(10) + 1), // Random volume between 1-10
					Owner: &productv1.Owner{
						Id:   "owner-dog-1",
						Name: "Bob Smith",
						Contact: &productv1.ContactInfo{
							Email: "bob@example.com",
							Phone: "555-200-3000",
							Address: &productv1.Address{
								Street:  "20 Dog Lane",
								City:    "Dogtown",
								Country: "USA",
								ZipCode: "20202",
							},
						},
					},
					Breed: &productv1.DogBreed{
						Id:     "breed-dog-1",
						Name:   "Dalmatian",
						Origin: "Croatia",
						Characteristics: &productv1.BreedCharacteristics{
							Size:        "Large",
							Temperament: "Outgoing and Friendly",
							Lifespan:    "10-13 years",
						},
					},
				},
			},
		}
	}

	return &productv1.QueryRandomPetResponse{
		RandomPet: pet,
	}, nil
}

func (s *MockService) QueryAllPets(ctx context.Context, in *productv1.QueryAllPetsRequest) (*productv1.QueryAllPetsResponse, error) {
	// Create a mix of cats and dogs
	var pets []*productv1.Animal

	// Add 2 cats
	for i := 1; i <= 2; i++ {
		pets = append(pets, &productv1.Animal{
			Instance: &productv1.Animal_Cat{
				Cat: &productv1.Cat{
					Id:         fmt.Sprintf("cat-%d", i),
					Name:       fmt.Sprintf("Cat %d", i),
					Kind:       fmt.Sprintf("Breed %d", i),
					MeowVolume: int32(i + 3), // Different volumes
					Owner: &productv1.Owner{
						Id:   fmt.Sprintf("owner-cat-%d", i),
						Name: fmt.Sprintf("Cat Owner %d", i),
						Contact: &productv1.ContactInfo{
							Email: fmt.Sprintf("cat-owner-%d@example.com", i),
							Phone: fmt.Sprintf("555-%03d-0000", i*100),
							Address: &productv1.Address{
								Street:  fmt.Sprintf("%d Cat Street", i*100),
								City:    "Feline City",
								Country: "USA",
								ZipCode: fmt.Sprintf("%05d", i*10000),
							},
						},
					},
					Breed: &productv1.CatBreed{
						Id:     fmt.Sprintf("breed-cat-%d", i),
						Name:   fmt.Sprintf("Cat Breed %d", i),
						Origin: "Various",
						Characteristics: &productv1.BreedCharacteristics{
							Size:        "Medium",
							Temperament: "Friendly",
							Lifespan:    "12-18 years",
						},
					},
				},
			},
		})
	}

	// Add 2 dogs
	for i := 1; i <= 2; i++ {
		pets = append(pets, &productv1.Animal{
			Instance: &productv1.Animal_Dog{
				Dog: &productv1.Dog{
					Id:         fmt.Sprintf("dog-%d", i),
					Name:       fmt.Sprintf("Dog %d", i),
					Kind:       fmt.Sprintf("Breed %d", i),
					BarkVolume: int32(i + 5), // Different volumes
					Owner: &productv1.Owner{
						Id:   fmt.Sprintf("owner-dog-%d", i),
						Name: fmt.Sprintf("Dog Owner %d", i),
						Contact: &productv1.ContactInfo{
							Email: fmt.Sprintf("dog-owner-%d@example.com", i),
							Phone: fmt.Sprintf("555-%03d-1111", i*100),
							Address: &productv1.Address{
								Street:  fmt.Sprintf("%d Dog Avenue", i*200),
								City:    "Canine City",
								Country: "USA",
								ZipCode: fmt.Sprintf("%05d", i*20000),
							},
						},
					},
					Breed: &productv1.DogBreed{
						Id:     fmt.Sprintf("breed-dog-%d", i),
						Name:   fmt.Sprintf("Dog Breed %d", i),
						Origin: "Various",
						Characteristics: &productv1.BreedCharacteristics{
							Size:        "Large",
							Temperament: "Loyal",
							Lifespan:    "10-14 years",
						},
					},
				},
			},
		})
	}

	return &productv1.QueryAllPetsResponse{
		AllPets: pets,
	}, nil
}

// Implementation for CreateUser mutation
func (s *MockService) MutationCreateUser(ctx context.Context, in *productv1.MutationCreateUserRequest) (*productv1.MutationCreateUserResponse, error) {
	input := in.GetInput()

	// Create a new user with the input name and a random ID
	user := &productv1.User{
		Id:   fmt.Sprintf("user-%d", rand.Intn(1000)),
		Name: input.GetName(),
	}

	return &productv1.MutationCreateUserResponse{
		CreateUser: user,
	}, nil
}

// Implementation for QueryCalculateTotals
func (s *MockService) QueryCalculateTotals(ctx context.Context, in *productv1.QueryCalculateTotalsRequest) (*productv1.QueryCalculateTotalsResponse, error) {
	orders := in.GetOrders()
	var calculatedOrders []*productv1.Order

	for _, orderInput := range orders {
		// Calculate total items by summing up quantities from all order lines
		var totalItems int32
		for _, line := range orderInput.GetLines() {
			totalItems += line.GetQuantity()
		}

		orderLines := []*productv1.OrderLine{}
		for _, line := range orderInput.GetLines() {
			orderLines = append(orderLines, &productv1.OrderLine{
				ProductId: line.GetProductId(),
				Quantity:  line.GetQuantity(),
				Modifiers: line.GetModifiers(),
			})
		}

		calculatedOrders = append(calculatedOrders, &productv1.Order{
			OrderId:      orderInput.GetOrderId(),
			CustomerName: orderInput.GetCustomerName(),
			TotalItems:   totalItems,
			OrderLines: &productv1.ListOfOrderLine{
				List: &productv1.ListOfOrderLine_List{
					Items: orderLines,
				},
			},
		})
	}

	return &productv1.QueryCalculateTotalsResponse{
		CalculateTotals: calculatedOrders,
	}, nil
}

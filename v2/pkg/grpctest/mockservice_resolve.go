package grpctest

import (
	context "context"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ResolveProductMascotRecommendation implements productv1.ProductServiceServer.
func (s *MockService) ResolveProductMascotRecommendation(_ context.Context, req *productv1.ResolveProductMascotRecommendationRequest) (*productv1.ResolveProductMascotRecommendationResponse, error) {
	results := make([]*productv1.ResolveProductMascotRecommendationResult, 0, len(req.GetContext()))

	includeDetails := false
	if req.GetFieldArgs() != nil {
		includeDetails = req.GetFieldArgs().GetIncludeDetails()
	}

	for i, ctx := range req.GetContext() {
		// Alternate between Cat and Dog based on index
		var animal *productv1.Animal
		if i%2 == 0 {
			volume := int32(5)
			if includeDetails {
				volume = int32((i + 1) * 8)
			}
			animal = &productv1.Animal{
				Instance: &productv1.Animal_Cat{
					Cat: &productv1.Cat{
						Id:         fmt.Sprintf("mascot-cat-%s", ctx.GetId()),
						Name:       fmt.Sprintf("MascotCat for %s", ctx.GetName()),
						Kind:       "Cat",
						MeowVolume: volume,
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-cat-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerCat for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-cat@example.com",
								Phone: "123-456-7890",
								Address: &productv1.Address{
									Street:  "123 Main St",
									City:    "Anytown",
									Country: "USA",
									ZipCode: "12345",
								},
							},
						},
						Breed: &productv1.CatBreed{
							Id:     fmt.Sprintf("breed-cat-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedCat for %s", ctx.GetName()),
							Origin: "USA",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Small",
								Temperament: "Curious",
								Lifespan:    "14-16 years",
							},
						},
					},
				},
			}
		} else {
			volume := int32(7)
			if includeDetails {
				volume = int32((i + 1) * 10)
			}
			animal = &productv1.Animal{
				Instance: &productv1.Animal_Dog{
					Dog: &productv1.Dog{
						Id:         fmt.Sprintf("mascot-dog-%s", ctx.GetId()),
						Name:       fmt.Sprintf("MascotDog for %s", ctx.GetName()),
						Kind:       "Dog",
						BarkVolume: volume,
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-dog-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerDog for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-dog@example.com",
								Phone: "123-456-7890",
								Address: &productv1.Address{
									Street:  "123 Main St",
									City:    "Anytown",
									Country: "USA",
									ZipCode: "12345",
								},
							},
						},
						Breed: &productv1.DogBreed{
							Id:     fmt.Sprintf("breed-dog-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedDog for %s", ctx.GetName()),
							Origin: "USA",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Medium",
								Temperament: "Loyal",
								Lifespan:    "10-12 years",
							},
						},
					},
				},
			}
		}

		results = append(results, &productv1.ResolveProductMascotRecommendationResult{
			MascotRecommendation: animal,
		})
	}

	return &productv1.ResolveProductMascotRecommendationResponse{
		Result: results,
	}, nil
}

// ResolveProductProductDetails implements productv1.ProductServiceServer.
func (s *MockService) ResolveProductProductDetails(_ context.Context, req *productv1.ResolveProductProductDetailsRequest) (*productv1.ResolveProductProductDetailsResponse, error) {
	results := make([]*productv1.ResolveProductProductDetailsResult, 0, len(req.GetContext()))

	includeExtended := false
	if req.GetFieldArgs() != nil {
		includeExtended = req.GetFieldArgs().GetIncludeExtended()
	}

	for i, ctx := range req.GetContext() {
		// Create recommended pet (alternate between Cat and Dog)
		var pet *productv1.Animal
		if i%2 == 0 {
			pet = &productv1.Animal{
				Instance: &productv1.Animal_Cat{
					Cat: &productv1.Cat{
						Id:         fmt.Sprintf("details-cat-%s", ctx.GetId()),
						Name:       fmt.Sprintf("RecommendedCat for %s", ctx.GetName()),
						Kind:       "Cat",
						MeowVolume: int32((i + 1) * 6),
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-details-cat-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerDetailsCat for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-details-cat@example.com",
								Phone: "555-111-2222",
								Address: &productv1.Address{
									Street:  "456 Oak Ave",
									City:    "Springfield",
									Country: "USA",
									ZipCode: "54321",
								},
							},
						},
						Breed: &productv1.CatBreed{
							Id:     fmt.Sprintf("breed-details-cat-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedDetailsCat for %s", ctx.GetName()),
							Origin: "France",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Medium",
								Temperament: "Friendly",
								Lifespan:    "12-15 years",
							},
						},
					},
				},
			}
		} else {
			pet = &productv1.Animal{
				Instance: &productv1.Animal_Dog{
					Dog: &productv1.Dog{
						Id:         fmt.Sprintf("details-dog-%s", ctx.GetId()),
						Name:       fmt.Sprintf("RecommendedDog for %s", ctx.GetName()),
						Kind:       "Dog",
						BarkVolume: int32((i + 1) * 9),
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-details-dog-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerDetailsDog for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-details-dog@example.com",
								Phone: "555-333-4444",
								Address: &productv1.Address{
									Street:  "789 Elm St",
									City:    "Riverside",
									Country: "USA",
									ZipCode: "67890",
								},
							},
						},
						Breed: &productv1.DogBreed{
							Id:     fmt.Sprintf("breed-details-dog-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedDetailsDog for %s", ctx.GetName()),
							Origin: "Germany",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Large",
								Temperament: "Loyal",
								Lifespan:    "10-12 years",
							},
						},
					},
				},
			}
		}

		// Create review summary (alternate between success and error based on price and extended flag)
		var reviewSummary *productv1.ActionResult
		if includeExtended && ctx.GetPrice() > 500 {
			reviewSummary = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionError{
					ActionError: &productv1.ActionError{
						Message: fmt.Sprintf("Product %s has negative reviews", ctx.GetName()),
						Code:    "NEGATIVE_REVIEWS",
					},
				},
			}
		} else {
			reviewSummary = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionSuccess{
					ActionSuccess: &productv1.ActionSuccess{
						Message:   fmt.Sprintf("Product %s has positive reviews", ctx.GetName()),
						Timestamp: "2024-01-01T15:00:00Z",
					},
				},
			}
		}

		description := fmt.Sprintf("Standard details for %s", ctx.GetName())
		if includeExtended {
			description = fmt.Sprintf("Extended details for %s with comprehensive information", ctx.GetName())
		}

		results = append(results, &productv1.ResolveProductProductDetailsResult{
			ProductDetails: &productv1.ProductDetails{
				Id:             fmt.Sprintf("details-%s-%d", ctx.GetId(), i),
				Description:    description,
				ReviewSummary:  reviewSummary,
				RecommendedPet: pet,
			},
		})
	}

	return &productv1.ResolveProductProductDetailsResponse{
		Result: results,
	}, nil
}

// ResolveProductStockStatus implements productv1.ProductServiceServer.
func (s *MockService) ResolveProductStockStatus(_ context.Context, req *productv1.ResolveProductStockStatusRequest) (*productv1.ResolveProductStockStatusResponse, error) {
	results := make([]*productv1.ResolveProductStockStatusResult, 0, len(req.GetContext()))

	checkAvailability := false
	if req.GetFieldArgs() != nil {
		checkAvailability = req.GetFieldArgs().GetCheckAvailability()
	}

	for i, ctx := range req.GetContext() {
		var stockStatus *productv1.ActionResult

		// If checking availability and price is high, return out of stock error
		if checkAvailability && ctx.GetPrice() > 300 && i%2 == 0 {
			stockStatus = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionError{
					ActionError: &productv1.ActionError{
						Message: fmt.Sprintf("Product %s is currently out of stock", ctx.GetName()),
						Code:    "OUT_OF_STOCK",
					},
				},
			}
		} else {
			stockStatus = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionSuccess{
					ActionSuccess: &productv1.ActionSuccess{
						Message:   fmt.Sprintf("Product %s is in stock and available", ctx.GetName()),
						Timestamp: "2024-01-01T10:00:00Z",
					},
				},
			}
		}

		results = append(results, &productv1.ResolveProductStockStatusResult{
			StockStatus: stockStatus,
		})
	}

	return &productv1.ResolveProductStockStatusResponse{
		Result: results,
	}, nil
}

// ResolveTestContainerDetails implements productv1.ProductServiceServer.
func (s *MockService) ResolveTestContainerDetails(_ context.Context, req *productv1.ResolveTestContainerDetailsRequest) (*productv1.ResolveTestContainerDetailsResponse, error) {
	results := make([]*productv1.ResolveTestContainerDetailsResult, 0, len(req.GetContext()))

	includeExtended := false
	if req.GetFieldArgs() != nil {
		includeExtended = req.GetFieldArgs().GetIncludeExtended()
	}

	for i, ctx := range req.GetContext() {
		// Alternate between Cat and Dog for the pet field (Animal interface)
		var pet *productv1.Animal
		if i%2 == 0 {
			pet = &productv1.Animal{
				Instance: &productv1.Animal_Cat{
					Cat: &productv1.Cat{
						Id:         fmt.Sprintf("test-cat-%s", ctx.GetId()),
						Name:       fmt.Sprintf("TestCat-%s", ctx.GetName()),
						Kind:       "Cat",
						MeowVolume: int32((i + 1) * 5),
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-test-cat-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerTestCat for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-test-cat@example.com",
								Phone: "555-555-5555",
								Address: &productv1.Address{
									Street:  "321 Pine Rd",
									City:    "Lakeside",
									Country: "Canada",
									ZipCode: "A1B2C3",
								},
							},
						},
						Breed: &productv1.CatBreed{
							Id:     fmt.Sprintf("breed-test-cat-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedTestCat for %s", ctx.GetName()),
							Origin: "Egypt",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Small",
								Temperament: "Curious",
								Lifespan:    "14-16 years",
							},
						},
					},
				},
			}
		} else {
			pet = &productv1.Animal{
				Instance: &productv1.Animal_Dog{
					Dog: &productv1.Dog{
						Id:         fmt.Sprintf("test-dog-%s", ctx.GetId()),
						Name:       fmt.Sprintf("TestDog-%s", ctx.GetName()),
						Kind:       "Dog",
						BarkVolume: int32((i + 1) * 7),
						Owner: &productv1.Owner{
							Id:   fmt.Sprintf("owner-test-dog-%s", ctx.GetId()),
							Name: fmt.Sprintf("OwnerTestDog for %s", ctx.GetName()),
							Contact: &productv1.ContactInfo{
								Email: "owner-test-dog@example.com",
								Phone: "555-666-7777",
								Address: &productv1.Address{
									Street:  "654 Birch Ln",
									City:    "Mountain View",
									Country: "Canada",
									ZipCode: "X9Y8Z7",
								},
							},
						},
						Breed: &productv1.DogBreed{
							Id:     fmt.Sprintf("breed-test-dog-%s", ctx.GetId()),
							Name:   fmt.Sprintf("BreedTestDog for %s", ctx.GetName()),
							Origin: "England",
							Characteristics: &productv1.BreedCharacteristics{
								Size:        "Medium",
								Temperament: "Energetic",
								Lifespan:    "11-13 years",
							},
						},
					},
				},
			}
		}

		// Alternate between ActionSuccess and ActionError for the status field (ActionResult union)
		var status *productv1.ActionResult
		if includeExtended && i%3 == 0 {
			// Return error status for extended mode on certain items
			status = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionError{
					ActionError: &productv1.ActionError{
						Message: fmt.Sprintf("Extended check failed for %s", ctx.GetName()),
						Code:    "EXTENDED_CHECK_FAILED",
					},
				},
			}
		} else {
			// Return success status
			status = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionSuccess{
					ActionSuccess: &productv1.ActionSuccess{
						Message:   fmt.Sprintf("TestContainer %s details loaded successfully", ctx.GetName()),
						Timestamp: "2024-01-01T12:00:00Z",
					},
				},
			}
		}

		summary := fmt.Sprintf("Summary for %s", ctx.GetName())
		if includeExtended {
			summary = fmt.Sprintf("Extended summary for %s with additional details", ctx.GetName())
		}

		results = append(results, &productv1.ResolveTestContainerDetailsResult{
			Details: &productv1.TestDetails{
				Id:      fmt.Sprintf("details-%s-%d", ctx.GetId(), i),
				Summary: summary,
				Pet:     pet,
				Status:  status,
			},
		})
	}

	return &productv1.ResolveTestContainerDetailsResponse{
		Result: results,
	}, nil
}

// ResolveCategoryMetricsNormalizedScore implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryMetricsNormalizedScore(_ context.Context, req *productv1.ResolveCategoryMetricsNormalizedScoreRequest) (*productv1.ResolveCategoryMetricsNormalizedScoreResponse, error) {
	results := make([]*productv1.ResolveCategoryMetricsNormalizedScoreResult, 0, len(req.GetContext()))

	baseline := req.GetFieldArgs().GetBaseline()
	if baseline == 0 {
		baseline = 1.0 // Avoid division by zero
	}

	for _, ctx := range req.GetContext() {
		// Calculate normalized score: (value / baseline) * 100
		// This gives a percentage relative to the baseline
		normalizedScore := (ctx.GetValue() / baseline) * 100.0

		results = append(results, &productv1.ResolveCategoryMetricsNormalizedScoreResult{
			NormalizedScore: normalizedScore,
		})
	}

	resp := &productv1.ResolveCategoryMetricsNormalizedScoreResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveCategoryMascot implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryMascot(_ context.Context, req *productv1.ResolveCategoryMascotRequest) (*productv1.ResolveCategoryMascotResponse, error) {
	results := make([]*productv1.ResolveCategoryMascotResult, 0, len(req.GetContext()))

	includeVolume := false
	if req.GetFieldArgs() != nil {
		includeVolume = req.GetFieldArgs().GetIncludeVolume()
	}

	for i, ctx := range req.GetContext() {
		// Return nil for certain categories to test optional return
		if ctx.GetKind() == productv1.CategoryKind_CATEGORY_KIND_OTHER {
			results = append(results, &productv1.ResolveCategoryMascotResult{
				Mascot: nil,
			})
		} else {
			// Alternate between Cat and Dog based on category kind
			var animal *productv1.Animal
			if ctx.GetKind() == productv1.CategoryKind_CATEGORY_KIND_BOOK || ctx.GetKind() == productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS {
				volume := int32(0)
				if includeVolume {
					volume = int32(i*10 + 5)
				}
				animal = &productv1.Animal{
					Instance: &productv1.Animal_Cat{
						Cat: &productv1.Cat{
							Id:         fmt.Sprintf("cat-mascot-%s", ctx.GetId()),
							Name:       fmt.Sprintf("Whiskers-%s", ctx.GetId()),
							Kind:       "Cat",
							MeowVolume: volume,
							Owner: &productv1.Owner{
								Id:   fmt.Sprintf("owner-cat-mascot-%s", ctx.GetId()),
								Name: fmt.Sprintf("OwnerCatMascot for %s", ctx.GetId()),
								Contact: &productv1.ContactInfo{
									Email: "owner-cat-mascot@example.com",
									Phone: "555-777-8888",
									Address: &productv1.Address{
										Street:  "111 Maple Dr",
										City:    "Booktown",
										Country: "USA",
										ZipCode: "11111",
									},
								},
							},
							Breed: &productv1.CatBreed{
								Id:     fmt.Sprintf("breed-cat-mascot-%s", ctx.GetId()),
								Name:   fmt.Sprintf("BreedCatMascot for %s", ctx.GetId()),
								Origin: "Scotland",
								Characteristics: &productv1.BreedCharacteristics{
									Size:        "Large",
									Temperament: "Gentle",
									Lifespan:    "13-17 years",
								},
							},
						},
					},
				}
			} else {
				volume := int32(0)
				if includeVolume {
					volume = int32(i*10 + 10)
				}
				animal = &productv1.Animal{
					Instance: &productv1.Animal_Dog{
						Dog: &productv1.Dog{
							Id:         fmt.Sprintf("dog-mascot-%s", ctx.GetId()),
							Name:       fmt.Sprintf("Buddy-%s", ctx.GetId()),
							Kind:       "Dog",
							BarkVolume: volume,
							Owner: &productv1.Owner{
								Id:   fmt.Sprintf("owner-dog-mascot-%s", ctx.GetId()),
								Name: fmt.Sprintf("OwnerDogMascot for %s", ctx.GetId()),
								Contact: &productv1.ContactInfo{
									Email: "owner-dog-mascot@example.com",
									Phone: "555-888-9999",
									Address: &productv1.Address{
										Street:  "222 Cedar Ct",
										City:    "Mascotville",
										Country: "USA",
										ZipCode: "22222",
									},
								},
							},
							Breed: &productv1.DogBreed{
								Id:     fmt.Sprintf("breed-dog-mascot-%s", ctx.GetId()),
								Name:   fmt.Sprintf("BreedDogMascot for %s", ctx.GetId()),
								Origin: "Australia",
								Characteristics: &productv1.BreedCharacteristics{
									Size:        "Medium",
									Temperament: "Playful",
									Lifespan:    "10-14 years",
								},
							},
						},
					},
				}
			}
			results = append(results, &productv1.ResolveCategoryMascotResult{
				Mascot: animal,
			})
		}
	}

	resp := &productv1.ResolveCategoryMascotResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveCategoryCategoryStatus implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryCategoryStatus(_ context.Context, req *productv1.ResolveCategoryCategoryStatusRequest) (*productv1.ResolveCategoryCategoryStatusResponse, error) {
	results := make([]*productv1.ResolveCategoryCategoryStatusResult, 0, len(req.GetContext()))

	checkHealth := false
	if req.GetFieldArgs() != nil {
		checkHealth = req.GetFieldArgs().GetCheckHealth()
	}

	for i, ctx := range req.GetContext() {
		var actionResult *productv1.ActionResult

		if checkHealth && i%3 == 0 {
			// Return error status for health check failures
			actionResult = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionError{
					ActionError: &productv1.ActionError{
						Message: fmt.Sprintf("Health check failed for category %s", ctx.GetName()),
						Code:    "HEALTH_CHECK_FAILED",
					},
				},
			}
		} else {
			// Return success status
			actionResult = &productv1.ActionResult{
				Value: &productv1.ActionResult_ActionSuccess{
					ActionSuccess: &productv1.ActionSuccess{
						Message:   fmt.Sprintf("Category %s is healthy", ctx.GetName()),
						Timestamp: "2024-01-01T00:00:00Z",
					},
				},
			}
		}

		results = append(results, &productv1.ResolveCategoryCategoryStatusResult{
			CategoryStatus: actionResult,
		})
	}

	resp := &productv1.ResolveCategoryCategoryStatusResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveProductRecommendedCategory implements productv1.ProductServiceServer.
func (s *MockService) ResolveProductRecommendedCategory(_ context.Context, req *productv1.ResolveProductRecommendedCategoryRequest) (*productv1.ResolveProductRecommendedCategoryResponse, error) {
	results := make([]*productv1.ResolveProductRecommendedCategoryResult, 0, len(req.GetContext()))

	maxPrice := int32(0)
	if req.GetFieldArgs() != nil {
		maxPrice = req.GetFieldArgs().GetMaxPrice()
	}

	for _, ctx := range req.GetContext() {
		// Return nil for products with high price to test optional return
		if maxPrice > 0 && ctx.GetPrice() > float64(maxPrice) {
			results = append(results, &productv1.ResolveProductRecommendedCategoryResult{
				RecommendedCategory: nil,
			})
		} else {
			// Create a recommended category based on product context
			var categoryKind productv1.CategoryKind
			if ctx.GetPrice() < 50 {
				categoryKind = productv1.CategoryKind_CATEGORY_KIND_BOOK
			} else if ctx.GetPrice() < 200 {
				categoryKind = productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS
			} else {
				categoryKind = productv1.CategoryKind_CATEGORY_KIND_FURNITURE
			}

			results = append(results, &productv1.ResolveProductRecommendedCategoryResult{
				RecommendedCategory: &productv1.Category{
					Id:            fmt.Sprintf("recommended-cat-%s", ctx.GetId()),
					Name:          fmt.Sprintf("Recommended for %s", ctx.GetName()),
					Kind:          categoryKind,
					Subcategories: createSubcategories(fmt.Sprintf("recommended-cat-%s", ctx.GetId()), categoryKind, 2),
				},
			})
		}
	}

	resp := &productv1.ResolveProductRecommendedCategoryResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveProductShippingEstimate implements productv1.ProductServiceServer.
func (s *MockService) ResolveProductShippingEstimate(_ context.Context, req *productv1.ResolveProductShippingEstimateRequest) (*productv1.ResolveProductShippingEstimateResponse, error) {
	results := make([]*productv1.ResolveProductShippingEstimateResult, 0, len(req.GetContext()))

	for _, ctx := range req.GetContext() {
		// Base shipping cost calculation
		baseCost := ctx.GetPrice() * 0.1 // 10% of product price

		// Add weight-based cost if input provided
		if req.GetFieldArgs() != nil && req.GetFieldArgs().GetInput() != nil {
			input := req.GetFieldArgs().GetInput()

			// Add weight cost
			weightCost := float64(input.GetWeight()) * 2.5
			baseCost += weightCost

			// Add expedited shipping cost
			if input.GetExpedited() != nil && input.GetExpedited().GetValue() {
				baseCost *= 1.5 // 50% surcharge for expedited
			}

			// Add destination-based cost
			destination := input.GetDestination()
			switch destination {
			case productv1.ShippingDestination_SHIPPING_DESTINATION_INTERNATIONAL:
				baseCost += 25.0
			case productv1.ShippingDestination_SHIPPING_DESTINATION_EXPRESS:
				baseCost += 10.0
			case productv1.ShippingDestination_SHIPPING_DESTINATION_DOMESTIC:
				// No additional cost for domestic shipping
			}
		}

		results = append(results, &productv1.ResolveProductShippingEstimateResult{
			ShippingEstimate: baseCost,
		})
	}

	resp := &productv1.ResolveProductShippingEstimateResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveCategoryCategoryMetrics implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryCategoryMetrics(_ context.Context, req *productv1.ResolveCategoryCategoryMetricsRequest) (*productv1.ResolveCategoryCategoryMetricsResponse, error) {
	results := make([]*productv1.ResolveCategoryCategoryMetricsResult, 0, len(req.GetContext()))

	metricType := ""
	if req.GetFieldArgs() != nil {
		metricType = req.GetFieldArgs().GetMetricType()
	}

	for i, ctx := range req.GetContext() {
		// Return nil for certain metric types to test optional return
		if metricType == "unavailable" {
			results = append(results, &productv1.ResolveCategoryCategoryMetricsResult{
				CategoryMetrics: nil,
			})
		} else {
			results = append(results, &productv1.ResolveCategoryCategoryMetricsResult{
				CategoryMetrics: &productv1.CategoryMetrics{
					Id:         fmt.Sprintf("metrics-%s-%d", ctx.GetId(), i),
					MetricType: metricType,
					Value:      float64(i*25 + 100), // Different values based on index
					Timestamp:  "2024-01-01T00:00:00Z",
					CategoryId: ctx.GetId(),
				},
			})
		}
	}

	resp := &productv1.ResolveCategoryCategoryMetricsResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveCategoryPopularityScore implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryPopularityScore(_ context.Context, req *productv1.ResolveCategoryPopularityScoreRequest) (*productv1.ResolveCategoryPopularityScoreResponse, error) {
	results := make([]*productv1.ResolveCategoryPopularityScoreResult, 0, len(req.GetContext()))

	threshold := req.GetFieldArgs().GetThreshold()

	baseScore := 50
	for range req.GetContext() {
		if int(threshold.GetValue()) > baseScore {
			results = append(results, &productv1.ResolveCategoryPopularityScoreResult{
				PopularityScore: nil,
			})
		} else {
			results = append(results, &productv1.ResolveCategoryPopularityScoreResult{
				PopularityScore: &wrapperspb.Int32Value{Value: int32(baseScore)},
			})
		}
	}

	resp := &productv1.ResolveCategoryPopularityScoreResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveSubcategoryItemCount implements productv1.ProductServiceServer.
func (s *MockService) ResolveSubcategoryItemCount(_ context.Context, req *productv1.ResolveSubcategoryItemCountRequest) (*productv1.ResolveSubcategoryItemCountResponse, error) {
	results := make([]*productv1.ResolveSubcategoryItemCountResult, 0, len(req.GetContext()))
	for i := range req.GetContext() {
		results = append(results, &productv1.ResolveSubcategoryItemCountResult{
			ItemCount: int32(i * 10), // Different multiplier to distinguish from productCount
		})
	}

	resp := &productv1.ResolveSubcategoryItemCountResponse{
		Result: results,
	}

	return resp, nil
}

// ResolveCategoryProductCount implements productv1.ProductServiceServer.
func (s *MockService) ResolveCategoryProductCount(_ context.Context, req *productv1.ResolveCategoryProductCountRequest) (*productv1.ResolveCategoryProductCountResponse, error) {
	results := make([]*productv1.ResolveCategoryProductCountResult, 0, len(req.GetContext()))
	for i := range req.GetContext() {
		results = append(results, &productv1.ResolveCategoryProductCountResult{
			ProductCount: int32(i),
		})
	}

	resp := &productv1.ResolveCategoryProductCountResponse{
		Result: results,
	}

	return resp, nil
}

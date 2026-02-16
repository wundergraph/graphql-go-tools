package shareddata

type Review struct {
	Text  string
	Stars int
}

type GeoLocation struct {
	Lat float64
	Lon float64
}

type TestProduct struct {
	ID           string
	Name         string
	Description  string
	Category     string
	Price        float64
	InStock      bool
	Reviews      []Review
	Rating       float64
	Manufacturer string
	Location     *GeoLocation // optional geo coordinates for the store
	CreatedAt    string       // ISO 8601 date, e.g. "2024-01-15"
	UpdatedAt    string       // RFC 3339 datetime, e.g. "2024-01-15T10:30:00Z"
}

func Products() []TestProduct {
	return []TestProduct{
		{
			ID:           "1",
			Name:         "Running Shoes",
			Description:  "Great for jogging and marathons",
			Category:     "Footwear",
			Price:        89.99,
			InStock:      true,
			Reviews:      []Review{{Text: "Great shoes", Stars: 5}},
			Rating:       4.5,
			Manufacturer: "Nike",
			Location:     &GeoLocation{Lat: 40.7128, Lon: -74.0060}, // New York
			CreatedAt:    "2024-01-15",
			UpdatedAt:    "2024-01-15T10:30:00Z",
		},
		{
			ID:           "2",
			Name:         "Basketball Shoes",
			Description:  "High-top basketball sneakers",
			Category:     "Footwear",
			Price:        129.99,
			InStock:      true,
			Reviews:      []Review{{Text: "Good grip", Stars: 4}},
			Rating:       4.2,
			Manufacturer: "Adidas",
			Location:     &GeoLocation{Lat: 40.7580, Lon: -73.9855}, // Midtown Manhattan (~5km from #1)
			CreatedAt:    "2024-03-20",
			UpdatedAt:    "2024-03-20T14:00:00Z",
		},
		{
			ID:           "3",
			Name:         "Leather Belt",
			Description:  "Genuine leather dress belt",
			Category:     "Accessories",
			Price:        35.00,
			InStock:      false,
			Reviews:      []Review{{Text: "Nice belt", Stars: 3}},
			Rating:       3.8,
			Manufacturer: "Gucci",
			Location:     &GeoLocation{Lat: 34.0522, Lon: -118.2437}, // Los Angeles (~3,940km from #1)
			CreatedAt:    "2024-06-01",
			UpdatedAt:    "2024-06-01T09:00:00Z",
		},
		{
			ID:           "4",
			Name:         "Wool Socks",
			Description:  "Warm wool socks for winter",
			Category:     "Footwear",
			Price:        12.99,
			InStock:      true,
			Reviews:      []Review{{Text: "Warm socks", Stars: 5}},
			Rating:       4.7,
			Manufacturer: "Smartwool",
			Location:     &GeoLocation{Lat: 51.5074, Lon: -0.1278}, // London (~5,570km from #1)
			CreatedAt:    "2024-09-10",
			UpdatedAt:    "2024-09-10T16:45:00Z",
		},
	}
}

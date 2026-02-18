package graph

import (
	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/productdetails/graph/model"
	"github.com/wundergraph/graphql-go-tools/execution/searchtesting/shareddata"
)

var productDetails map[string]*model.Product

func init() {
	productDetails = make(map[string]*model.Product)
	for _, p := range shareddata.Products() {
		p := p
		reviews := make([]*model.Review, len(p.Reviews))
		for i, r := range p.Reviews {
			reviews[i] = &model.Review{Text: r.Text, Stars: r.Stars}
		}
		name := p.Name
		desc := p.Description
		cat := p.Category
		price := p.Price
		inStock := p.InStock
		rating := p.Rating
		mfr := p.Manufacturer
		productDetails[p.ID] = &model.Product{
			ID:           p.ID,
			Name:         &name,
			Description:  &desc,
			Category:     &cat,
			Price:        &price,
			InStock:      &inStock,
			Reviews:      reviews,
			Rating:       &rating,
			Manufacturer: &mfr,
		}
	}
}

func LookupProduct(id string) *model.Product {
	p, ok := productDetails[id]
	if !ok {
		return nil
	}
	return p
}

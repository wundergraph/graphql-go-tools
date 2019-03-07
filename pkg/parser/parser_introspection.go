package parser

import "github.com/jensneuse/graphql-go-tools/pkg/introspection"

func (p *Parser) ParseIntrospectionResponse(response *introspection.Response) (err error) {

	mod := NewManualAstMod(p)

	p.resetCaches()
	p.l.ResetTypeSystemInput()

	if response.Data.Schema.QueryType != nil {
		ref, err := mod.PutLiteralString(response.Data.Schema.QueryType.Name)
		if err != nil {
			return err
		}
		mod.SetQueryTypeName(ref)
	}

	if response.Data.Schema.MutationType != nil {
		ref, err := mod.PutLiteralString(response.Data.Schema.MutationType.Name)
		if err != nil {
			return err
		}
		mod.SetMutationTypeName(ref)
	}

	if response.Data.Schema.SubscriptionType != nil {
		ref, err := mod.PutLiteralString(response.Data.Schema.SubscriptionType.Name)
		if err != nil {
			return err
		}
		mod.SetSubscriptionTypeName(ref)
	}

	return
}

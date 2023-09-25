package plan

import (
	"fmt"
)

type FederationMetaData struct {
	Keys     FederationFieldConfigurations
	Requires FederationFieldConfigurations
	Provides FederationFieldConfigurations
}

type FederationFieldConfiguration struct {
	TypeName     string
	FieldName    string
	SelectionSet string
}

type FederationFieldConfigurations []FederationFieldConfiguration

func (f FederationFieldConfigurations) FilterByType(typeName string) (out []FederationFieldConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != "" {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f FederationFieldConfigurations) FilterByTypeAndField(typeName, fieldName string) (out []FederationFieldConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != fieldName {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f FederationFieldConfigurations) ForType(typeName string) (*FederationFieldConfiguration, int) {
	for i := range f {
		if f[i].TypeName == typeName {
			return &f[i], i
		}
	}
	return nil, -1
}

func (f FederationFieldConfigurations) HasTypeAndField(typeName, fieldName string) bool {
	for i := range f {
		if f[i].TypeName == typeName && f[i].FieldName == fieldName {
			return true
		}
	}
	return false
}

func (f FederationFieldConfigurations) HasSelectionSet(typeName, selectionSet string) bool {
	for i := range f {
		if typeName != f[i].TypeName {
			continue
		}
		if f[i].SelectionSet == selectionSet {
			return true
		}
	}
	return false
}

func AppendRequiredFieldsConfigurationWithMerge(configs FederationFieldConfigurations, config FederationFieldConfiguration) FederationFieldConfigurations {
	cfg, i := configs.ForType(config.TypeName)
	if i == -1 {
		return append(configs, config)
	}

	cfg.SelectionSet = fmt.Sprintf("%s %s", cfg.SelectionSet, config.SelectionSet)
	if cfg.FieldName == "" {
		cfg.FieldName = config.FieldName
	}

	return configs
}

func AppendRequiredFieldsConfigurationIfNotPresent(configs FederationFieldConfigurations, config FederationFieldConfiguration) FederationFieldConfigurations {
	ok := configs.HasSelectionSet(config.TypeName, config.SelectionSet)
	if ok {
		return configs
	}

	return append(configs, config)
}

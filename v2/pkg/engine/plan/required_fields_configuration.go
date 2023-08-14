package plan

import (
	"fmt"
)

type RequiredFieldsConfiguration struct {
	TypeName     string
	FieldName    string
	SelectionSet string
}

type RequiredFieldsConfigurations []RequiredFieldsConfiguration

func (f RequiredFieldsConfigurations) FilterByType(typeName string) (out []RequiredFieldsConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != "" {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f RequiredFieldsConfigurations) FilterByTypeAndField(typeName, fieldName string) (out []RequiredFieldsConfiguration) {
	for i := range f {
		if f[i].TypeName != typeName || f[i].FieldName != fieldName {
			continue
		}
		out = append(out, f[i])
	}
	return out
}

func (f RequiredFieldsConfigurations) ForType(typeName string) (*RequiredFieldsConfiguration, int) {
	for i := range f {
		if f[i].TypeName == typeName {
			return &f[i], i
		}
	}
	return nil, -1
}

func (f RequiredFieldsConfigurations) HasRequirement(typeName, requiresFields string) bool {
	for i := range f {
		if typeName != f[i].TypeName {
			continue
		}
		if f[i].SelectionSet == requiresFields {
			return true
		}
	}
	return false
}

func AppendRequiredFieldsConfigurationWithMerge(configs RequiredFieldsConfigurations, config RequiredFieldsConfiguration) RequiredFieldsConfigurations {
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

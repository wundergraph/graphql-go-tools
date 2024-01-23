package engine

import (
	"os"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"
)

// LoadStringVariable is a shorthand for LookupStringVariable when you do not care about
// the value being explicitly set
func LoadStringVariable(variable *nodev1.ConfigurationVariable) string {
	val, _ := LookupStringVariable(variable)
	return val
}

// LookupStringVariable returns the value for the given configuration variable as well
// as whether it was explicitly set. If the variable is nil or the environment
// variable it references is not set, it returns false as its second value.
// Otherwise, (e.g. environment variable set but empty, static string), the
// second return value is true. If you don't need to know if the variable
// was explicitly set, use LoadStringVariable.
func LookupStringVariable(variable *nodev1.ConfigurationVariable) (string, bool) {
	if variable == nil {
		return "", false
	}
	switch variable.GetKind() {
	case nodev1.ConfigurationVariableKind_ENV_CONFIGURATION_VARIABLE:
		if varName := variable.GetEnvironmentVariableName(); varName != "" {
			value, found := os.LookupEnv(variable.GetEnvironmentVariableName())
			if found {
				return value, found
			}
		}
		defValue := variable.GetEnvironmentVariableDefaultValue()
		return defValue, defValue != ""
	case nodev1.ConfigurationVariableKind_STATIC_CONFIGURATION_VARIABLE:
		return variable.GetStaticVariableContent(), true
	default:
		panic("unhandled wgpb.ConfigurationVariableKind")
	}
}

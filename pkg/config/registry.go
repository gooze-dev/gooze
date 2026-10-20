package config

// Registry gives access to the configuration schema. Consumers depend on it
// instead of the package functions when they need to fake the schema in tests.
type Registry interface {
	// Schema returns the top-level fields of the configuration file.
	Schema() []Field
	// Lookup finds a field by its dotted key.
	Lookup(key string) (Field, bool)
	// Keys returns the dotted keys of every leaf field in schema order.
	Keys() []string
	// IsConfigFile reports whether base is the name of a file gooze loads.
	IsConfigFile(base string) bool
}

// DefaultRegistry is the Registry backed by the package functions.
type DefaultRegistry struct{}

// NewRegistry returns the Registry backed by the package functions.
func NewRegistry() DefaultRegistry {
	return DefaultRegistry{}
}

// Schema implements Registry.
func (DefaultRegistry) Schema() []Field {
	return Schema()
}

// Lookup implements Registry.
func (DefaultRegistry) Lookup(key string) (Field, bool) {
	return Lookup(key)
}

// Keys implements Registry.
func (DefaultRegistry) Keys() []string {
	return Keys()
}

// IsConfigFile implements Registry.
func (DefaultRegistry) IsConfigFile(base string) bool {
	return IsConfigFile(base)
}

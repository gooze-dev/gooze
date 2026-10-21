package config

// Kind is the type of value a configuration key expects.
type Kind int

const (
	// KindObject is a mapping whose keys are the field's Children.
	KindObject Kind = iota
	// KindString is a free-form string.
	KindString
	// KindBool is a boolean.
	KindBool
	// KindInt is an integer.
	KindInt
	// KindStringList is a sequence of strings.
	KindStringList
	// KindEnum is a string from Field.Enum, or an integer when Field.IntEnum is set.
	KindEnum
	// KindFloat is a number, which may have a fractional part.
	KindFloat
)

// String returns the name of the kind.
func (k Kind) String() string {
	switch k {
	case KindObject:
		return "object"
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindStringList:
		return "string list"
	case KindEnum:
		return "enum"
	case KindFloat:
		return "float"
	default:
		return "unknown"
	}
}

package zugzug

import "fmt"

// Settings provide a way to configure data from an environment.
type Settings []struct {
	Var  any
	Name string
	Use  string
}

// Apply will resolve settings by name using the provided lookup function, stopping at the first error.
func (seq Settings) Apply(lookup func(string) (string, bool)) error {
	for _, it := range seq {
		if it.Name == `` {
			continue
		}
		if value, ok := lookup(it.Name); ok {
			if err := set(it.Var, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// set will set the value of a variable.
func set(target any, value string) error {
	switch target := target.(type) {
	case *string:
		*target = value
	default:
		_, err := fmt.Sscan(value, target)
		if err != nil {
			return fmt.Errorf(`%w in %q`, err, target)
		}
	}
	return nil
}

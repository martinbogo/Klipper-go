package config

import "fmt"

func ValidateIntRange(option, section string, value, minval, maxval int) error {
	if minval != 0 && value < minval {
		return fmt.Errorf("Option '%s' in section '%s' must have minimum of %d", option, section, minval)
	}
	if maxval != 0 && value > maxval {
		return fmt.Errorf("Option '%s' in section '%s' must have maximum of %d", option, section, maxval)
	}
	return nil
}

func ValidateInt64Range(option, section string, value, minval, maxval int64) error {
	if minval != 0 && value < minval {
		return fmt.Errorf("Option '%s' in section '%s' must have minimum of %d", option, section, minval)
	}
	if maxval != 0 && value > maxval {
		return fmt.Errorf("Option '%s' in section '%s' must have maximum of %d", option, section, maxval)
	}
	return nil
}

func ValidateFloatRange(option, section string, value, minval, maxval, above, below float64) error {
	if minval != 0 && value < minval {
		return fmt.Errorf("Option '%s' in section '%s' must have minimum of %f", option, section, minval)
	}
	if maxval != 0 && value > maxval {
		return fmt.Errorf("Option '%s' in section '%s' must have maximum of %f", option, section, maxval)
	}
	if above != 0 && value <= above {
		return fmt.Errorf("Option '%s' in section '%s' must be above %f", option, section, above)
	}
	if below != 0 && value >= below {
		return fmt.Errorf("Option '%s' in section '%s' must be below %f", option, section, below)
	}
	return nil
}

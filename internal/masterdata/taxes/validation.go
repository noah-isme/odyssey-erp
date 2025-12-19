package taxes

import (
	"errors"
	"strings"
)

func (s *Service) validate(t Tax) error {
	if strings.TrimSpace(t.Code) == "" {
		return errors.New("tax code is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tax name is required")
	}
	if t.Rate < 0 || t.Rate > 100 {
		return errors.New("tax rate must be between 0 and 100")
	}
	return nil
}

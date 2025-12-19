package units

import (
	"errors"
	"strings"
)

func (s *Service) validate(u Unit) error {
	if strings.TrimSpace(u.Code) == "" {
		return errors.New("unit code is required")
	}
	if strings.TrimSpace(u.Name) == "" {
		return errors.New("unit name is required")
	}
	return nil
}

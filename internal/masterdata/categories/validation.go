package categories

import (
	"errors"
	"strings"
)

func (s *Service) validate(c Category) error {
	if strings.TrimSpace(c.Code) == "" {
		return errors.New("category code is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("category name is required")
	}
	return nil
}

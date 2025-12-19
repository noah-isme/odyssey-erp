package products

import (
	"errors"
	"strings"
)

func (s *Service) validate(p Product) error {
	if strings.TrimSpace(p.Code) == "" {
		return errors.New("product code is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("product name is required")
	}
	return nil
}

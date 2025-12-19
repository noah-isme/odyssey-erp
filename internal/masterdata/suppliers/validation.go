package suppliers

import (
	"errors"
	"strings"
)

func (s *Service) validate(sup Supplier) error {
	if strings.TrimSpace(sup.Code) == "" {
		return errors.New("supplier code is required")
	}
	if strings.TrimSpace(sup.Name) == "" {
		return errors.New("supplier name is required")
	}
	return nil
}

package companies

import (
	"errors"
	"strings"
)

func (s *Service) validate(c Company) error {
	if strings.TrimSpace(c.Code) == "" {
		return errors.New("company code is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("company name is required")
	}
	// Add more validation as needed (e.g. tax ID format)
	return nil
}

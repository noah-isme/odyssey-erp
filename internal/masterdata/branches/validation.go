package branches

import (
	"errors"
	"strings"
)

func (s *Service) validate(b Branch) error {
	if b.CompanyID <= 0 {
		return errors.New("company is required")
	}
	if strings.TrimSpace(b.Code) == "" {
		return errors.New("branch code is required")
	}
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("branch name is required")
	}
	return nil
}

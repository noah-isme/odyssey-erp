package warehouses

import (
	"errors"
	"strings"
)

func (s *Service) validate(w Warehouse) error {
	if w.BranchID <= 0 {
		return errors.New("branch is required")
	}
	if strings.TrimSpace(w.Code) == "" {
		return errors.New("warehouse code is required")
	}
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("warehouse name is required")
	}
	return nil
}

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
	if p.CostMethod == "" {
		p.CostMethod = "AVG"
	}
	if p.CostMethod != "AVG" && p.CostMethod != "FIFO" {
		return errors.New("cost method must be AVG or FIFO")
	}
	if p.MinStock < 0 || p.ReorderTarget < 0 {
		return errors.New("stock thresholds cannot be negative")
	}
	if p.TrackBatch && p.TrackSerial {
		return errors.New("a product cannot track both batches and serial numbers")
	}
	return nil
}

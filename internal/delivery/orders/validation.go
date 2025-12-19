package orders

import (
	"fmt"
	"time"
)

// ValidateCreateRequest validates create request.
func ValidateCreateRequest(req CreateRequest) error {
	if len(req.Lines) == 0 {
		return ErrEmptyLines
	}
	for i, line := range req.Lines {
		if line.QuantityToDeliver <= 0 {
			return fmt.Errorf("line %d: %w", i+1, ErrInvalidQuantity)
		}
	}
	return nil
}

// ValidateUpdateRequest validates update request.
func ValidateUpdateRequest(req UpdateRequest) error {
	if req.DeliveryDate != nil && req.DeliveryDate.Before(time.Now().Truncate(24*time.Hour)) {
		return ErrInvalidDeliveryDate
	}
	if req.Lines != nil {
		if len(*req.Lines) == 0 {
			return ErrEmptyLines
		}
		for i, line := range *req.Lines {
			if line.QuantityToDeliver <= 0 {
				return fmt.Errorf("line %d: %w", i+1, ErrInvalidQuantity)
			}
		}
	}
	return nil
}

// ValidateCancelRequest validates cancel request.
func ValidateCancelRequest(req CancelRequest) error {
	if len(req.Reason) < 10 {
		return ErrReasonTooShort
	}
	return nil
}

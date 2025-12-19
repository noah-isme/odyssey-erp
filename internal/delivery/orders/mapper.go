package orders

// Mapper functions for converting between layers:
// CreateRequest → DeliveryOrder (domain)
// WithDetails → DetailResponse (dto)

// ToDeliveryOrder maps CreateRequest to domain DeliveryOrder.
func (r CreateRequest) ToDeliveryOrder(docNumber string, customerID, createdBy int64) DeliveryOrder {
	return DeliveryOrder{
		DocNumber:      docNumber,
		CompanyID:      r.CompanyID,
		SalesOrderID:   r.SalesOrderID,
		WarehouseID:    r.WarehouseID,
		CustomerID:     customerID,
		DeliveryDate:   r.DeliveryDate,
		Status:         StatusDraft,
		DriverName:     r.DriverName,
		VehicleNumber:  r.VehicleNumber,
		TrackingNumber: r.TrackingNumber,
		Notes:          r.Notes,
		CreatedBy:      createdBy,
	}
}

// ToLine maps CreateLineReq to domain Line.
func (r CreateLineReq) ToLine(doID int64, deliverable *DeliverableSOLine) Line {
	return Line{
		DeliveryOrderID:   doID,
		SalesOrderLineID:  r.SalesOrderLineID,
		ProductID:         r.ProductID,
		QuantityToDeliver: r.QuantityToDeliver,
		QuantityDelivered: 0,
		UOM:               deliverable.UOM,
		UnitPrice:         deliverable.UnitPrice,
		Notes:             r.Notes,
		LineOrder:         r.LineOrder,
	}
}

// ToDetailResponse maps WithDetails and lines to DetailResponse DTO.
func ToDetailResponse(wd *WithDetails, lines []LineWithDetails) DetailResponse {
	return DetailResponse{
		DeliveryOrder: *wd,
		Lines:         lines,
	}
}

// ToListResponse maps list results to ListResponse DTO.
func ToListResponse(orders []WithDetails, total, limit, offset int) ListResponse {
	return ListResponse{
		DeliveryOrders: orders,
		Total:          total,
		Limit:          limit,
		Offset:         offset,
	}
}

// BuildDeliverableMap creates a lookup map for deliverable SO lines.
func BuildDeliverableMap(lines []DeliverableSOLine) map[int64]*DeliverableSOLine {
	m := make(map[int64]*DeliverableSOLine, len(lines))
	for i := range lines {
		m[lines[i].SalesOrderLineID] = &lines[i]
	}
	return m
}

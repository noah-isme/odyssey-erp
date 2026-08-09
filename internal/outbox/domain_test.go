package outbox

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPublishRequestValidate(t *testing.T) {
	valid := PublishRequest{CompanyID: 1, CorrelationID: uuid.New(), EventType: "sales.order.created", AggregateType: "sales_order", AggregateID: 1, Payload: map[string]string{"number": "SO-1"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	for _, req := range []PublishRequest{{}, {CompanyID: 1}, {CompanyID: 1, CorrelationID: uuid.New()}, {CompanyID: 1, CorrelationID: uuid.New(), EventType: "event"}, {CompanyID: 1, CorrelationID: uuid.New(), EventType: "event", AggregateType: "aggregate"}, {CompanyID: 1, CorrelationID: uuid.New(), EventType: "event", AggregateType: "aggregate", AggregateID: 1}} {
		if err := req.Validate(); err == nil || !strings.HasPrefix(err.Error(), "outbox:") {
			t.Fatalf("request %+v error=%v", req, err)
		}
	}
}

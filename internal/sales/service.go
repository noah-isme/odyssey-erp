package sales

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/products"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/orders"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
)

type Service struct {
	Customers  *customers.Service
	Quotations *quotations.Service
	Orders     *orders.Service
	Products   *products.Service
	pool       *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	// Repositories
	custRepo := customers.NewRepository(pool)
	quoteRepo := quotations.NewRepository(pool)
	orderRepo := orders.NewRepository(pool)
	prodRepo := products.NewRepository(pool)

	// Services
	custSvc := customers.NewService(custRepo)
	prodSvc := products.NewService(prodRepo)
	quoteSvc := quotations.NewService(quoteRepo, custRepo)
	orderSvc := orders.NewService(orderRepo, custRepo, quoteRepo)

	return &Service{
		Customers:  custSvc,
		Quotations: quoteSvc,
		Orders:     orderSvc,
		Products:   prodSvc,
		pool:       pool,
	}
}

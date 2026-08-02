package app

import (
	"context"
	"sync"
	"time"
)

// CacheEntry represents a cached value with expiration
type CacheEntry struct {
	value      interface{}
	expiresAt  time.Time
	createdAt  time.Time
}

// Cache implements a simple in-memory cache with TTL
type Cache struct {
	mu    sync.RWMutex
	items map[string]*CacheEntry
}

// NewCache creates a new cache instance
func NewCache() *Cache {
	cache := &Cache{
		items: make(map[string]*CacheEntry),
	}
	// Start cleanup goroutine
	go cache.cleanup()
	return cache
}

// Set stores a value in cache with TTL
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items[key] = &CacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
		createdAt: time.Now(),
	}
}

// Get retrieves a value from cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.items[key]
	if !exists {
		return nil, false
	}
	
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	
	return entry.value, true
}

// Delete removes a key from cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*CacheEntry)
}

// cleanup removes expired entries periodically
func (c *Cache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.items {
			if now.After(entry.expiresAt) {
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}

// Stats returns cache statistics
func (c *Cache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"total_entries": len(c.items),
		"timestamp":     time.Now(),
	}
}

// ContextKeyCache is the context key for cache
type contextKey string

const ContextKeyCache contextKey = "cache"

// CacheFromContext retrieves cache from context
func CacheFromContext(ctx context.Context) *Cache {
	if cache, ok := ctx.Value(ContextKeyCache).(*Cache); ok {
		return cache
	}
	return nil
}

// ContextWithCache adds cache to context
func ContextWithCache(ctx context.Context, cache *Cache) context.Context {
	return context.WithValue(ctx, ContextKeyCache, cache)
}

// QueryOptimizations provides SQL query optimization recommendations
type QueryOptimizations struct {
	// Index recommendations
	Indexes []string
	
	// Query patterns to optimize
	Patterns map[string]string
	
	// Common queries to cache
	CacheableQueries []string
}

// PerformanceMonitor tracks operation metrics
type PerformanceMonitor struct {
	mu              sync.RWMutex
	operationTimes  map[string][]time.Duration
	cacheHits       int64
	cacheMisses     int64
	slowQueryCount  int64
	slowQueryThreshold time.Duration
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(slowQueryThreshold time.Duration) *PerformanceMonitor {
	return &PerformanceMonitor{
		operationTimes:     make(map[string][]time.Duration),
		slowQueryThreshold: slowQueryThreshold,
	}
}

// RecordOperation records an operation duration
func (pm *PerformanceMonitor) RecordOperation(name string, duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.operationTimes[name] = append(pm.operationTimes[name], duration)
	
	if duration > pm.slowQueryThreshold {
		pm.slowQueryCount++
	}
}

// RecordCacheHit records a cache hit
func (pm *PerformanceMonitor) RecordCacheHit() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cacheHits++
}

// RecordCacheMiss records a cache miss
func (pm *PerformanceMonitor) RecordCacheMiss() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cacheMisses++
}

// GetStats returns performance statistics
func (pm *PerformanceMonitor) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	stats := map[string]interface{}{
		"cache_hits":           pm.cacheHits,
		"cache_misses":         pm.cacheMisses,
		"slow_query_count":     pm.slowQueryCount,
		"timestamp":            time.Now(),
	}
	
	// Calculate averages per operation
	operationStats := make(map[string]interface{})
	for op, durations := range pm.operationTimes {
		if len(durations) == 0 {
			continue
		}
		
		var total time.Duration
		var max time.Duration
		min := time.Hour // Large initial value
		
		for _, d := range durations {
			total += d
			if d > max {
				max = d
			}
			if d < min {
				min = d
			}
		}
		
		avg := total / time.Duration(len(durations))
		operationStats[op] = map[string]interface{}{
			"average": avg,
			"max":     max,
			"min":     min,
			"count":   len(durations),
		}
	}
	
	stats["operations"] = operationStats
	return stats
}

// PRODUCTION OPTIMIZATION CHECKLIST:
// ═══════════════════════════════════════════════════════════════════════════

// Database Indexes (add to migrations):
// CREATE INDEX idx_loads_company_status ON loads(company_id, status);
// CREATE INDEX idx_loads_origin_destination ON loads(origin_warehouse_id, destination_warehouse_id);
// CREATE INDEX idx_routes_load_status ON delivery_routes(load_id, status);
// CREATE INDEX idx_route_stops_route_sequence ON route_stops(route_id, stop_sequence);
// CREATE INDEX idx_transfers_company_status ON transfer_orders(company_id, status);
// CREATE INDEX idx_contracts_supplier_status ON supplier_contracts(supplier_id, status);
// CREATE INDEX idx_scorecards_company_supplier ON supplier_scorecards(company_id, supplier_id);
// CREATE INDEX idx_shipments_order_status ON shipments(order_id, status);

// Query Optimization Patterns:
// 1. Load all items with loads in single query (avoid N+1)
//    SELECT l.*, li.* FROM loads l LEFT JOIN load_items li ON l.id = li.load_id WHERE l.company_id = $1
// 
// 2. Cache planning rules by warehouse (rarely change)
//    Cache key: "planning_rules:{warehouse_id}" TTL: 1 hour
//
// 3. Cache supplier scorecards (updated monthly)
//    Cache key: "scorecard:{company_id}:{supplier_id}" TTL: 6 hours
//
// 4. Paginate large result sets
//    Always use LIMIT/OFFSET on list queries
//    Recommended: 50-100 items per page

// HTTP Handler Optimizations:
// 1. Enable gzip compression in middleware
// 2. Set proper cache headers for static assets
// 3. Implement request timeout (30s default, 60s for reports)
// 4. Use connection pooling (pgxpool)
// 5. Implement circuit breaker for external services

// UI/Frontend Optimizations:
// 1. Minify CSS/JS in production
// 2. Load Bootstrap and icons from CDN
// 3. Lazy load images and heavy content
// 4. Implement client-side pagination
// 5. Cache API responses in browser (Service Worker)
// 6. Use efficient data formats (JSON over XML)

// Monitoring and Alerting:
// 1. Track slow queries (>100ms)
// 2. Monitor cache hit ratio (target: >80%)
// 3. Track database connection pool usage
// 4. Monitor HTTP response times (target: <200ms)
// 5. Alert on error rates >1%

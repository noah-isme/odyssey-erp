package http

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const cacheTTL = 5 * time.Minute

var viewModelCache = newResponseCache(cacheTTL)

type cacheItem struct {
	value   interface{}
	expires time.Time
}

type responseCache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]cacheItem
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{
		ttl:   ttl,
		items: make(map[string]cacheItem),
	}
}

func (c *responseCache) Get(key string) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expires) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return item.value, true
}

func (c *responseCache) Set(key string, value interface{}) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items[key] = cacheItem{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *responseCache) Bust() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items = make(map[string]cacheItem)
	c.mu.Unlock()
}

func buildCacheKey(report string, groupID int64, period string, entities []int64, fxOn bool) string {
	entityToken := "all"
	if len(entities) > 0 {
		sorted := append([]int64(nil), entities...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		parts := make([]string, len(sorted))
		for i, id := range sorted {
			parts[i] = fmt.Sprintf("%d", id)
		}
		entityToken = strings.Join(parts, ",")
	}
	fxToken := "0"
	if fxOn {
		fxToken = "1"
	}
	return fmt.Sprintf("consol:%s:%d|%s|%s|fx=%s", report, groupID, period, entityToken, fxToken)
}

// BustConsolViewCache invalidates the cached consolidation view models.
func BustConsolViewCache() {
	if viewModelCache != nil {
		viewModelCache.Bust()
	}
}

func clonePLViewModel(src ConsolPLViewModel) ConsolPLViewModel {
	dst := ConsolPLViewModel{
		Filters: ConsolPLFilters{
			GroupID:  src.Filters.GroupID,
			Period:   src.Filters.Period,
			FxOn:     src.Filters.FxOn,
			Entities: append([]int64(nil), src.Filters.Entities...),
		},
		Totals:        src.Totals,
		Errors:        map[string]string{},
		Warnings:      append([]string(nil), src.Warnings...),
		Lines:         make([]ConsolPLLine, len(src.Lines)),
		Contributions: make([]ConsolPLEntityContribution, len(src.Contributions)),
	}
	copy(dst.Lines, src.Lines)
	copy(dst.Contributions, src.Contributions)
	return dst
}

func cloneBSViewModel(src ConsolBSViewModel) ConsolBSViewModel {
	dst := ConsolBSViewModel{
		Filters: ConsolBSFilters{
			GroupID:  src.Filters.GroupID,
			Period:   src.Filters.Period,
			FxOn:     src.Filters.FxOn,
			Entities: append([]int64(nil), src.Filters.Entities...),
		},
		Totals:        src.Totals,
		Errors:        map[string]string{},
		Warnings:      append([]string(nil), src.Warnings...),
		Assets:        make([]ConsolBSLine, len(src.Assets)),
		LiabilitiesEq: make([]ConsolBSLine, len(src.LiabilitiesEq)),
		Contributions: make([]ConsolBSEntityContribution, len(src.Contributions)),
	}
	copy(dst.Assets, src.Assets)
	copy(dst.LiabilitiesEq, src.LiabilitiesEq)
	copy(dst.Contributions, src.Contributions)
	return dst
}

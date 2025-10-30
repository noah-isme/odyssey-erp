package http

import (
	"context"

	"golang.org/x/sync/singleflight"
)

var vmBuildGroup singleflight.Group

func singleflightBuild(ctx context.Context, key string, fn func(context.Context) (interface{}, error)) (interface{}, error, bool) {
	resultChan := vmBuildGroup.DoChan(key, func() (interface{}, error) {
		return fn(ctx)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err(), false
	case res := <-resultChan:
		return res.Val, res.Err, res.Shared
	}
}

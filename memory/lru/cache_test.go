// Copyright 2023 ecodeclub
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lru

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ecodeclub/ekit/set"

	"github.com/ecodeclub/ecache"
	"github.com/ecodeclub/ecache/internal/errs"
	"github.com/ecodeclub/ekit/list"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalCache_cleanCycle(t *testing.T) {
	c := NewCache(200, WithCycleInterval(time.Second))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key string

		wantVal string
		wantErr error
	}{
		{
			name: "tail exist TTL value",
			before: func(t *testing.T) {
				ctx := context.Background()
				err := c.Set(ctx, "test1", "hello1", time.Second)
				assert.Nil(t, err)
				err = c.Set(ctx, "test2", "hello2", time.Second*10)
				assert.Nil(t, err)
			},
			after: func(t *testing.T) {
				ctx := context.Background()
				_, err := c.Delete(ctx, "test1", "test2")
				assert.Nil(t, err)
			},
			key:     "test1",
			wantVal: "",
			wantErr: errs.ErrKeyNotExist,
		},
		{
			name: "not exist TTL value",
			before: func(t *testing.T) {
				ctx := context.Background()
				err := c.Set(ctx, "test1", "hello1", time.Second)
				assert.Nil(t, err)
				res := c.GetSet(ctx, "test1", "hello1")
				assert.Nil(t, res.Err)
			},
			after: func(t *testing.T) {
				_, err := c.Delete(context.Background(), "test1")
				assert.Nil(t, err)
			},
			key:     "test1",
			wantVal: "hello1",
		},
		{
			name: "exist TTL value",
			before: func(t *testing.T) {
				ctx := context.Background()
				err := c.Set(ctx, "test1", "hello1", time.Second*10)
				assert.Nil(t, err)
				err = c.Set(ctx, "test2", "hello2", time.Second)
				assert.Nil(t, err)
			},
			after: func(t *testing.T) {
				ctx := context.Background()
				_, err := c.Delete(ctx, "test1", "test2")
				assert.Nil(t, err)
			},
			key:     "test2",
			wantVal: "",
			wantErr: errs.ErrKeyNotExist,
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			time.Sleep(time.Second)
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()
			result := c.Get(ctx, tc.key)
			val, err := result.String()
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_Set(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name  string
		after func(t *testing.T)

		key        string
		val        string
		expiration time.Duration

		wantErr error
	}{
		{
			name: "set value",
			after: func(t *testing.T) {
				result, ok := cache.get("test")
				assert.Equal(t, true, ok)
				assert.Equal(t, "hello ecache", result.(string))
				assert.Equal(t, true, cache.remove("test"))
			},
			key:        "test",
			val:        "hello ecache",
			expiration: time.Minute,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			err := cache.Set(ctx, tc.key, tc.val, tc.expiration)
			require.NoError(t, err)
			tc.after(t)
		})
	}
}

func TestCache_Get(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key string

		wantVal string
		wantErr error
	}{
		{
			name: "get value",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "hello ecache"))
				assert.Equal(t, 0, evictCounter)
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
				assert.Equal(t, 1, evictCounter)
			},
			key:     "test",
			wantVal: "hello ecache",
		},
		{
			name: "get set TTL value",
			before: func(t *testing.T) {
				assert.Equal(t, true,
					cache.addTTL("test", "hello ecache", time.Second))
				assert.Equal(t, 1, evictCounter)
			},
			after: func(t *testing.T) {
				time.Sleep(time.Second)
				_, ok := cache.get("test")
				assert.Equal(t, false, ok)
				assert.Equal(t, 2, evictCounter)
			},
			key:     "test",
			wantVal: "hello ecache",
		},
		{
			name:    "get value err",
			before:  func(t *testing.T) {},
			after:   func(t *testing.T) {},
			key:     "test",
			wantErr: errs.ErrKeyNotExist,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			result := cache.Get(ctx, tc.key)
			val, err := result.String()
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_SetNX(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(1, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     string
		expire  time.Duration
		wantVal bool
	}{
		{
			name:    "setnx value",
			before:  func(t *testing.T) {},
			after:   func(t *testing.T) {},
			key:     "test",
			val:     "hello ecache",
			expire:  time.Minute,
			wantVal: true,
		},
		{
			name: "setnx value exist",
			before: func(t *testing.T) {
				assert.Equal(t, false, cache.add("test", "hello ecache"))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     "hello world",
			expire:  time.Minute,
			wantVal: false,
		},
		{
			name: "setnx expired value",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.addTTL("test", "hello ecache", time.Second))
			},
			after: func(t *testing.T) {
				time.Sleep(time.Second)
				assert.Equal(t, false, cache.remove("test"))
			},
			key:     "test",
			val:     "hello world",
			expire:  time.Minute,
			wantVal: false,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			result, err := cache.SetNX(ctx, tc.key, tc.val, tc.expire)
			assert.Equal(t, tc.wantVal, result)
			require.NoError(t, err)
			tc.after(t)
		})
	}
}

func TestCache_GetSet(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     string
		wantVal string
		wantErr error
	}{
		{
			name: "getset value",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "hello ecache"))
			},
			after: func(t *testing.T) {
				result, ok := cache.get("test")
				assert.Equal(t, true, ok)
				assert.Equal(t, "hello world", result)
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     "hello world",
			wantVal: "hello ecache",
		},
		{
			name:   "getset value not key error",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				result, ok := cache.get("test")
				assert.Equal(t, true, ok)
				assert.Equal(t, "hello world", result)
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     "hello world",
			wantErr: errs.ErrKeyNotExist,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			result := cache.GetSet(ctx, tc.key, tc.val)
			val, err := result.String()
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache(5)

	testCases := []struct {
		name   string
		before func(ctx context.Context, t *testing.T, cache ecache.Cache)

		ctxFunc func() context.Context
		key     []string

		wantN   int64
		wantErr error
	}{
		{
			name: "delete expired key",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 0))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name"},
			wantN: 0,
		},
		{
			name: "delete single existed key",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 10*time.Second))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name"},
			wantN: 1,
		},
		{
			name:   "delete single does not existed key",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key: []string{"notExistedKey"},
		},
		{
			name: "delete multiple expired keys",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 0))
				require.NoError(t, cache.Set(ctx, "age", 18, 0))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name", "age"},
			wantN: 0,
		},
		{
			name: "delete multiple existed keys",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 10*time.Second))
				require.NoError(t, cache.Set(ctx, "age", 18, 10*time.Second))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name", "age"},
			wantN: 2,
		},
		{
			name:   "delete multiple do not existed keys",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key: []string{"name", "age"},
		},
		{
			name: "delete multiple keys, some do not expired keys",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 0))
				require.NoError(t, cache.Set(ctx, "age", 18, 0))
				require.NoError(t, cache.Set(ctx, "gender", "male", 0))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name", "age", "gender", "addr"},
			wantN: 0,
		},
		{
			name: "delete multiple keys, some do not existed keys",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {
				require.NoError(t, cache.Set(ctx, "name", "Alex", 10*time.Second))
				require.NoError(t, cache.Set(ctx, "age", 18, 10*time.Second))
				require.NoError(t, cache.Set(ctx, "gender", "male", 10*time.Second))
			},
			ctxFunc: func() context.Context {
				return context.Background()
			},
			key:   []string{"name", "age", "gender", "addr"},
			wantN: 3,
		},
		{
			name:   "timeout",
			before: func(ctx context.Context, t *testing.T, cache ecache.Cache) {},
			ctxFunc: func() context.Context {
				timeout := time.Millisecond * 100
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				time.Sleep(timeout * 2)
				return ctx
			},
			key:     []string{"name", "age", "addr"},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctxFunc()
			tc.before(ctx, t, cache)
			n, err := cache.Delete(ctx, tc.key...)
			if err != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.Equal(t, tc.wantN, n)
		})
	}
}

func TestCache_LPush(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     []any
		wantVal int64
		wantErr error
	}{
		{
			name:   "lpush value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache"},
			wantVal: 1,
		},
		{
			name:   "lpush multiple value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache", "hello world"},
			wantVal: 2,
		},
		{
			name: "lpush value exists",
			before: func(t *testing.T) {
				val := ecache.Value{}
				val.Val = "hello ecache"
				l := &list.ConcurrentList[ecache.Value]{
					List: list.NewLinkedListOf[ecache.Value]([]ecache.Value{val}),
				}
				assert.Equal(t, true, cache.add("test", l))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello world"},
			wantVal: 2,
		},
		{
			name: "lpush value not type",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "string"))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache"},
			wantErr: errors.New("当前key不是list类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			length, err := cache.LPush(ctx, tc.key, tc.val...)
			assert.Equal(t, tc.wantVal, length)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_LPop(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		wantVal string
		wantErr error
	}{
		{
			name: "lpop value",
			before: func(t *testing.T) {
				val := ecache.Value{}
				val.Val = "hello ecache"
				l := &list.ConcurrentList[ecache.Value]{
					List: list.NewLinkedListOf[ecache.Value]([]ecache.Value{val}),
				}
				assert.Equal(t, true, cache.add("test", l))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			wantVal: "hello ecache",
		},
		{
			name: "lpop value not nil",
			before: func(t *testing.T) {
				val := ecache.Value{}
				val.Val = "hello ecache"
				val2 := ecache.Value{}
				val2.Val = "hello world"
				l := &list.ConcurrentList[ecache.Value]{
					List: list.NewLinkedListOf[ecache.Value]([]ecache.Value{val, val2}),
				}
				assert.Equal(t, true, cache.add("test", l))
			},
			after: func(t *testing.T) {
				val, ok := cache.get("test")
				assert.Equal(t, true, ok)
				result, ok := val.(list.List[ecache.Value])
				assert.Equal(t, true, ok)
				assert.Equal(t, 1, result.Len())
				value, err := result.Delete(0)
				assert.NoError(t, err)
				assert.Equal(t, "hello world", value.Val)
				assert.NoError(t, value.Err)

				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			wantVal: "hello ecache",
		},
		{
			name: "lpop value type error",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "hello world"))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			wantErr: errors.New("当前key不是list类型"),
		},
		{
			name:    "lpop not key",
			before:  func(t *testing.T) {},
			after:   func(t *testing.T) {},
			key:     "test",
			wantErr: errs.ErrKeyNotExist,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			val := cache.LPop(ctx, tc.key)
			result, err := val.String()
			assert.Equal(t, tc.wantVal, result)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_SAdd(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     []any
		wantVal int64
		wantErr error
	}{
		{
			name:   "sadd value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache", "hello world"},
			wantVal: 2,
		},
		{
			name: "sadd value exist",
			before: func(t *testing.T) {
				s := set.NewMapSet[any](8)
				s.Add("hello world")

				assert.Equal(t, true, cache.add("test", s))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache"},
			wantVal: 2,
		},
		{
			name: "sadd value type err",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "string"))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello"},
			wantErr: errors.New("当前key已存在不是set类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			val, err := cache.SAdd(ctx, tc.key, tc.val...)
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_SRem(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key string
		val []any

		wantVal int64
		wantErr error
	}{
		{
			name: "srem value",
			before: func(t *testing.T) {
				s := set.NewMapSet[any](8)

				s.Add("hello world")
				s.Add("hello ecache")

				assert.Equal(t, true, cache.add("test", s))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello world"},
			wantVal: 1,
		},
		{
			name: "srem value ignore",
			before: func(t *testing.T) {
				s := set.NewMapSet[any](8)
				s.Add("hello world")

				assert.Equal(t, true, cache.add("test", s))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello ecache"},
			wantVal: 0,
		},
		{
			name:    "srem value nil",
			before:  func(t *testing.T) {},
			after:   func(t *testing.T) {},
			key:     "test",
			val:     []any{"hello world"},
			wantErr: errs.ErrKeyNotExist,
		},
		{
			name: "srem value type error",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", int64(1)))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     []any{"hello world"},
			wantErr: errors.New("当前key已存在不是set类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.after(t)
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			val, err := cache.SRem(ctx, tc.key, tc.val...)
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantVal, val)
		})
	}
}

func TestCache_IncrBy(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     int64
		wantVal int64
		wantErr error
	}{
		{
			name:   "incrby value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     1,
			wantVal: 1,
		},
		{
			name: "incrby value add",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", int64(1)))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     1,
			wantVal: 2,
		},
		{
			name: "incrby value type error",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", 12.62))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     1,
			wantErr: errors.New("当前key不是int64类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			result, err := cache.IncrBy(ctx, tc.key, tc.val)
			assert.Equal(t, tc.wantVal, result)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_DecrBy(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     int64
		wantVal int64
		wantErr error
	}{
		{
			name:   "decrby value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     1,
			wantVal: -1,
		},
		{
			name: "decrby old value",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", int64(3)))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     2,
			wantVal: 1,
		},
		{
			name: "decrby value type error",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", 3.156))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     1,
			wantErr: errors.New("当前key不是int64类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			val, err := cache.DecrBy(ctx, tc.key, tc.val)
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

func TestCache_IncrByFloat(t *testing.T) {
	evictCounter := 0
	onEvicted := func(key string, value any) {
		evictCounter++
	}
	cache := NewCache(5, WithEvictCallback(onEvicted))

	testCase := []struct {
		name   string
		before func(t *testing.T)
		after  func(t *testing.T)

		key     string
		val     float64
		wantVal float64
		wantErr error
	}{
		{
			name:   "incrbyfloat value",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     2.0,
			wantVal: 2.0,
		},
		{
			name: "incrbyfloat decr value",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", 3.1))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     -2.0,
			wantVal: 1.1,
		},
		{
			name: "incrbyfloat value type error",
			before: func(t *testing.T) {
				assert.Equal(t, true, cache.add("test", "hello"))
			},
			after: func(t *testing.T) {
				assert.Equal(t, true, cache.remove("test"))
			},
			key:     "test",
			val:     10,
			wantErr: errors.New("当前key不是float64类型"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
			defer cancelFunc()

			tc.before(t)
			val, err := cache.IncrByFloat(ctx, tc.key, tc.val)
			assert.Equal(t, tc.wantVal, val)
			assert.Equal(t, tc.wantErr, err)
			tc.after(t)
		})
	}
}

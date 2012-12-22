// Copyright 2012 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// +build appengine

package app

import (
	"appengine"
	"appengine/memcache"
	"bytes"
	"encoding/gob"
	"time"
)

func cacheGet(c appengine.Context, key string, object interface{}) (*memcache.Item, error) {
	item, err := memcache.Get(c, key)
	switch {
	case err != nil:
		item = &memcache.Item{Key: key}
	case len(item.Value) == 1 && item.Value[0] == 0:
		// deleted sentinel.
		err = memcache.ErrCacheMiss
	default:
		err = gob.NewDecoder(bytes.NewBuffer(item.Value)).Decode(object)
	}
	return item, err
}

func cacheSet(c appengine.Context, item *memcache.Item) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(item.Object)
	if err != nil {
		return err
	}
	item.Value = buf.Bytes()
	return memcache.Set(c, item)
}

func cacheSafeSet(c appengine.Context, item *memcache.Item) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(item.Object)
	if err != nil {
		return err
	}

	swap := item.Value != nil
	item.Value = buf.Bytes()

	if swap {
		err = memcache.CompareAndSwap(c, item)
		switch err {
		case memcache.ErrCASConflict:
			// OK, cache item set by another request
			return nil
		case memcache.ErrNotStored:
			// Item expired. Try adding below.
		default:
			return err
		}
	}

	err = memcache.Add(c, item)
	if err == memcache.ErrNotStored {
		// OK, cache item set by another request
		err = nil
	}
	return err
}

func cacheClear(c appengine.Context, keys ...string) error {
	items := make([]*memcache.Item, len(keys))
	for i := range keys {
		items[i] = &memcache.Item{Key: keys[i], Expiration: 2 * time.Minute, Value: []byte{0}}
	}
	return memcache.SetMulti(c, items)
}

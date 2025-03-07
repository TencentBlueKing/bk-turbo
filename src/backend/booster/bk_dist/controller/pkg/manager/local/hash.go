/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package local

import (
	"crypto/sha256"
	"sort"
	"strconv"
)

type hash interface {
	AddNode(node string)
	RemoveNode(node string)
	GetNode(key string) string
}

// HashRing represents a consistent hash ring.
type HashRing struct {
	replicas int
	keys     []int
	hashMap  map[int]string
}

// NewHashRing creates a new HashRing with the given number of replicas.
func NewHashRing(replicas int) hash {
	return &HashRing{
		replicas: replicas,
		hashMap:  make(map[int]string),
	}
}

// AddNode adds a node to the hash ring.
func (h *HashRing) AddNode(node string) {
	for i := 0; i < h.replicas; i++ {
		hash := h.hashKey(node + strconv.Itoa(i))
		h.keys = append(h.keys, hash)
		h.hashMap[hash] = node
	}
	sort.Ints(h.keys)
}

// RemoveNode removes a node from the hash ring.
func (h *HashRing) RemoveNode(node string) {
	for i := 0; i < h.replicas; i++ {
		hash := h.hashKey(node + strconv.Itoa(i))
		index := sort.SearchInts(h.keys, hash)
		if index < len(h.keys) && h.keys[index] == hash {
			h.keys = append(h.keys[:index], h.keys[index+1:]...)
			delete(h.hashMap, hash)
		}
	}
}

// GetNode returns the closest node in the hash ring for the given key.
func (h *HashRing) GetNode(key string) string {
	if len(h.keys) == 0 {
		return ""
	}
	hash := h.hashKey(key)
	index := sort.Search(len(h.keys), func(i int) bool { return h.keys[i] >= hash })
	if index == len(h.keys) {
		index = 0
	}
	return h.hashMap[h.keys[index]]
}

// hashKey generates a hash for a given key.
func (h *HashRing) hashKey(key string) int {
	hash := sha256.Sum256([]byte(key))
	return int(hash[0])<<24 | int(hash[1])<<16 | int(hash[2])<<8 | int(hash[3])
}

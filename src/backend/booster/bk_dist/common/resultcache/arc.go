/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package resultcache

/*
Adaptive Replacement Cache (ARC)

. . . [    B1 <- [      T1 <- ! -> T2    ] -> B2    ] . .
       [ . . . . [ . . . . . . . ! . . ^ . . . ] . . . ]
                [   固定缓存大小 (c)     ]

T1 和 B1 合称为 L1，即近期单个引用的组合历史记录。同样，L2 是 T2 和 B2 的组合。
内部的[ ]括号表示实际缓存，虽然大小固定，但可以在 B1 和 B2 历史记录中自由移动。

新条目进入 T1，位于!的左侧，并逐渐被推向左侧，最终从 T1 被驱逐到 B1，最后完全退出。
L1 中的任何条目如果再次被引用，就会获得另一次机会，并进入 L2，就在中央!标记的右侧。从那里，
它再次被向外推，从 T2 进入 B2。L2 中再次被命中的条目可以无限重复此过程，直到它们最终在 B2 的最右侧退出。
*/

import (
	"container/list"
	"fmt"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

type entry struct {
	key   interface{}
	value interface{}
	ll    *list.List
	el    *list.Element
	ghost bool
}

func (e *entry) setLRU(list *list.List) {
	e.detach()
	e.ll = list
	e.el = e.ll.PushBack(e)
}

// 添加到指定list的最前面
func (e *entry) jumpto(list *list.List) {
	e.detach()
	e.ll = list
	e.el = e.ll.PushFront(e)
}

// 从原来的list中删掉
func (e *entry) detach() {
	if e.ll != nil {
		e.ll.Remove(e.el)
	}
}

func min(n1, n2 int) int {
	if n1 <= n2 {
		return n1
	}

	return n2
}

func max(n1, n2 int) int {
	if n1 >= n2 {
		return n1
	}

	return n2
}

type ARC struct {
	p int // t1与t2的partion，总长度一定，p表示t1的可用空间
	c int // 最大容量

	// 只使用过一次的列表，按使用时间排序,t1+b1被视为l1
	// l1 的容量不超过c
	// t1 + t2 的容量不超过c
	t1 *list.List
	b1 *list.List

	// 一次以上的列表，按使用时间排序,t2+b2被视为l2
	// l2 的容量不超过c
	// t1 + t2 的容量不超过c
	t2 *list.List
	b2 *list.List

	// 全局map
	cache map[interface{}]*entry
}

// NewARC returns a new Adaptive Replacement Cache (ARC).
func NewARC(c int) *ARC {
	return &ARC{
		p:     0,
		c:     c,
		t1:    list.New(),
		b1:    list.New(),
		t2:    list.New(),
		b2:    list.New(),
		cache: make(map[interface{}]*entry, c),
	}
}

func (a *ARC) Put(key, value interface{}) (bool, interface{}, interface{}) {
	ent, ok := a.cache[key]
	var evict, deleted interface{}
	if !ok {
		ent = &entry{
			key:   key,
			value: value,
			ghost: false,
		}

		evict, deleted = a.adjust(ent, "Put")
		a.cache[key] = ent
	} else {
		ent.value = value
		ent.ghost = false
		evict, deleted = a.adjust(ent, "Put")
	}

	// for debug
	a.Dump(fmt.Sprintf("after put %s", key.(string)))

	return ok, evict, deleted
}

func (a *ARC) Get(key interface{}) (interface{}, bool, interface{}, interface{}) {
	blog.Infof("[ARC]:ready get %s", key.(string))
	// for debug
	defer a.Dump(fmt.Sprintf("after get %s", key.(string)))

	ent, ok := a.cache[key]
	if ok {
		evict, deleted := a.adjust(ent, "Get")
		return ent.value, !ent.ghost, evict, deleted
	}
	return nil, false, nil, nil
}

func (a *ARC) Dump(prefix string) {
	blog.Infof("[ARC]:[%s] c:%d,p:%d,len(b1):%d,len(t1):%d,len(t2):%d,len(b2):%d,total len:%d",
		prefix, a.c, a.p, a.b1.Len(), a.t1.Len(), a.t2.Len(), a.b2.Len(),
		a.t1.Len()+a.b1.Len()+a.t2.Len()+a.b2.Len())
}

// 最多会触发两个淘汰，返回给业务层处理
func (a *ARC) adjust(ent *entry, action string) (interface{}, interface{}) {
	// 如果已存在，且不在ghost列表里，提到t2的最前面
	if ent.ll == a.t1 || ent.ll == a.t2 {
		if ent.ll == a.t1 {
			blog.Infof("[ARC]:%s %s hit in a.t1", action, ent.el.Value.(*entry).key.(string))
		} else {
			blog.Infof("[ARC]:%s %s hit in a.t2", action, ent.el.Value.(*entry).key.(string))
		}
		ent.jumpto(a.t2)
		return nil, nil
	}

	// 在b1里面，意味着t1值得增加容量
	if ent.ll == a.b1 {
		blog.Infof("[ARC]:%s %s hit in a.b1", action, ent.el.Value.(*entry).key.(string))
		var d int
		// 如果b1长度大于b2，则简单增加一个空间
		if a.b1.Len() >= a.b2.Len() {
			d = 1
		} else {
			// 否则按长度倍数增加
			d = a.b2.Len() / a.b1.Len()
		}
		a.p = min(a.p+d, a.c)

		ghost := a.selectGhost(ent)
		ent.jumpto(a.t2)

		return ghost, nil
	}

	// 在ghost b2里面，意味着t2值得增加容量
	if ent.ll == a.b2 {
		blog.Infof("[ARC]:%s %s hit in a.b1", action, ent.el.Value.(*entry).key.(string))

		var d int
		if a.b2.Len() >= a.b1.Len() {
			d = 1
		} else {
			d = a.b1.Len() / a.b2.Len()
		}
		a.p = max(a.p-d, 0)

		ghost := a.selectGhost(ent)
		ent.jumpto(a.t2)

		return ghost, nil
	}

	// 新元素，可能触发驱逐或者删除，驱逐意味着从t到b，去掉value只保留key，删除从cache删掉key
	if ent.ll == nil {
		var ghost, deleted interface{}
		l1len := a.t1.Len() + a.b1.Len()
		if l1len >= a.c {
			if a.b1.Len() > 0 {
				deleted = a.delLRU(a.b1, "a.b1")
				ghost = a.selectGhost(ent)
			} else {
				deleted = a.delLRU(a.t1, "a.t1")
			}
		} else {
			l2len := a.t2.Len() + a.b2.Len()
			if l1len+l2len >= a.c {
				if l1len+l2len >= 2*a.c {
					deleted = a.delLRU(a.b2, "a.b2")
				}
				ghost = a.selectGhost(ent)
			}
		}

		ent.jumpto(a.t1)

		return ghost, deleted
	}

	return nil, nil
}

// 彻底删除
func (a *ARC) delLRU(list *list.List, from string) interface{} {
	lru := list.Back()
	list.Remove(lru)
	delete(a.cache, lru.Value.(*entry).key)
	blog.Infof("[ARC]:deleted %s from %s", lru.Value.(*entry).key.(string), from)
	return lru.Value.(*entry).key
}

// 返回被驱逐的key
func (a *ARC) selectGhost(ent *entry) interface{} {
	if a.t1.Len() > 0 && ((a.t1.Len() > a.p) || (ent.ll == a.b2 && a.t1.Len() == a.p)) {
		back := a.t1.Back()
		if back != nil {
			lru := back.Value.(*entry)
			lru.value = nil
			lru.ghost = true
			lru.jumpto(a.b1)
			return lru.key
		}
	} else {
		back := a.t2.Back()
		if back != nil {
			lru := a.t2.Back().Value.(*entry)
			lru.value = nil
			lru.ghost = true
			lru.jumpto(a.b2)
			return lru.key
		}
	}

	return nil
}

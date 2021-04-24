package collections

import (
	"sync"
	"sync/atomic"
)

type IntList interface {
	// 检查一个元素是否存在，如果存在则返回 true，否则返回 false
	Contains(value int) bool

	// 插入一个元素，如果此操作成功插入一个元素，则返回 true，否则返回 false
	Insert(value int) bool

	// 删除一个元素，如果此操作成功删除一个元素，则返回 true，否则返回 false
	Delete(value int) bool

	// 遍历此有序链表的所有元素，如果 f 返回 false，则停止遍历
	Range(f func(value int) bool)

	// 返回有序链表的元素个数
	Len() int
}

type intNode struct {
	value       int
	nextPtr     atomic.Value
	markedValue atomic.Value
	mutex       sync.Mutex
}

func (n *intNode) mark() {
	n.markedValue.Store(true)
}

func (n *intNode) marked() bool {
	b, ok := n.markedValue.Load().(bool)
	return b && ok
}

func (n *intNode) next() *intNode {
	nxt, _ := n.nextPtr.Load().(*intNode)
	return nxt
}

func (n *intNode) updateNext(next *intNode) {
	n.nextPtr.Store(next)
}

func newIntNode(value int) *intNode {
	return &intNode{value: value}
}

// ConcurrentIntList
type ConcurrentIntList struct {
	root *intNode
	size int64
}

func NewConcurrentIntList() *ConcurrentIntList {
	return &ConcurrentIntList{root: newIntNode(-1)}
}

func (intList *ConcurrentIntList) Contains(value int) bool {
	next := intList.root.next()
	for next != nil && (next.marked() || next.value < value) {
		next = next.next()
	}
	if next == nil {
		return false
	}
	return next.value == value
}

func (intList *ConcurrentIntList) Insert(value int) bool {
start:
	pre := intList.root
	current := pre.next()
	// step1: find first node lager then value
	for current != nil && current.value < value {
		pre = current
		current = pre.next()
	}
	// not find
	if current != nil && current.value == value {
		return false
	}
	// step2: lock pre
	pre.mutex.Lock()
	// step3: check if other goroutine modified
	if pre.next() != current || pre.marked() || (current != nil && current.marked()) {
		pre.mutex.Unlock()
		goto start
	}
	// step4: add net node
	newNode := newIntNode(value)
	// set next for new node first, avoid other goroutine get a invalid node
	newNode.updateNext(current)
	// add
	intList.sizeIncr()
	pre.updateNext(newNode)
	pre.mutex.Unlock()
	return true
}

func (intList *ConcurrentIntList) Delete(value int) bool {
start:
	pre := intList.root
	current := pre.next()
	// step1: find first node equal to value
	for current != nil && (current.marked() || current.value < value) {
		pre = current
		current = pre.next()
	}
	// not find
	if current == nil || current.value != value {
		return false
	}
	// step2: lock current
	current.mutex.Lock()
	// check if has been modified by other goroutine
	if current.marked() {
		current.mutex.Unlock()
		goto start
	}
	// step3: lock pre node
	pre.mutex.Lock()
	// check if has been modified by other goroutine
	if pre.next() != current || pre.marked() {
		// anti flow, avoid dead lock
		pre.mutex.Unlock()
		current.mutex.Unlock()
		goto start
	}
	// step4: mark and remove
	current.mark()
	pre.updateNext(current.next())
	intList.sizeDecr()
	// anti flow, avoid dead lock
	pre.mutex.Unlock()
	current.mutex.Unlock()
	return true
}

func (intList *ConcurrentIntList) Range(f func(value int) bool) {
	n := intList.root.next()
	// we can't make sure list is not modified during range, so ignore the modify during range.
	for n != nil && f(n.value) {
		n = n.next()
	}
}

func (intList *ConcurrentIntList) sizeIncr() {
	atomic.AddInt64(&intList.size, 1)
}

func (intList *ConcurrentIntList) sizeDecr() {
	atomic.AddInt64(&intList.size, -1)
}

// Len doesn't make sense in concurrent
func (intList *ConcurrentIntList) Len() int {
	return int(atomic.LoadInt64(&intList.size))
}

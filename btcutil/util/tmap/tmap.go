package tmap

import (
	"github.com/emirpasic/gods/trees/redblacktree"
	"github.com/pkt-cash/pktd/btcutil/er"
)

type Map[K, V any] struct {
	tm   *redblacktree.Tree
	comp func(a, b *K) int
}

func New[K, V any](comp func(a, b *K) int) *Map[K, V] {
	return &Map[K, V]{
		tm: redblacktree.NewWith(func(a interface{}, b interface{}) int {
			return comp((a).(*K), (b).(*K))
		}),
		comp: comp,
	}
}

func ForEach[K, V any](s *Map[K, V], f func(k *K, v *V) er.R) er.R {
	it := s.tm.Iterator()
	for it.Next() {
		if err := f(it.Key().(*K), it.Value().(*V)); err != nil {
			if er.IsLoopBreak(err) {
				return nil
			} else {
				return err
			}
		}
	}
	return nil
}

func Insert[K, V any](s *Map[K, V], k *K, v *V) (*K, *V) {
	if n, ok := s.tm.Ceiling(k); ok {
		if ok && s.comp(k, n.Key.(*K)) == 0 {
			s.tm.Put(k, v)
			return n.Key.(*K), n.Value.(*V)
		}
	}
	s.tm.Put(k, v)
	return nil, nil
}

func Len[K, V any](s *Map[K, V]) int {
	return s.tm.Size()
}

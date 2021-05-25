package mem

import (
	"context"
	"sort"
	"sync"

	"github.com/glrf/tyny/api"
	"github.com/glrf/tyny/store"
)

func New() *Store {
	return &Store{redirects: map[string]*api.Redirect{}}
}

type Store struct {
	mu        sync.Mutex
	redirects map[string]*api.Redirect
}

func (s *Store) Get(ctx context.Context, id string) (*api.Redirect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.redirects[id]
	if !ok {
		return nil, store.NotFound
	}
	return r, nil
}
func (s *Store) Put(ctx context.Context, r *api.Redirect) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.redirects[r.Name]
	if ok {
		return store.Conflict
	}
	s.redirects[r.Name] = r
	return nil
}
func (s *Store) Update(ctx context.Context, id string, updateFn func(*api.Redirect)(*api.Redirect, error)) (*api.Redirect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
  r, ok := s.redirects[id]
	if !ok {
		return nil, store.NotFound
	}

  r, err := updateFn(r)
  if err != nil {
    return r, err
  }

	s.redirects[r.Name] = r
	return r, nil
}
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.redirects[id]
	if !ok {
		return store.NotFound
	}
	delete(s.redirects, id)
	return nil
}
func (s *Store) List(ctx context.Context, token string, pagesize int) ([]*api.Redirect, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := []string{}
	for k := range s.redirects {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	i := sort.SearchStrings(keys, token)

	res := []*api.Redirect{}

	count := 0
	for i < len(keys) {
		key := keys[i]
		if pagesize > 0 && count >= pagesize {
			return res, key, nil
		}
		res = append(res, s.redirects[key])
		i = i + 1
		count = count + 1
	}

	return res, "", nil
}

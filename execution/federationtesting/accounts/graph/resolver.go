// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.
package graph

import "sync"

type Resolver struct {
	usersMu sync.RWMutex
	users   map[string]string
}

func NewResolver() *Resolver {
	return &Resolver{
		users: map[string]string{
			"1234": "Me",
			"7777": "User 7777",
		},
	}
}

func (r *Resolver) GetUsername(id string) string {
	r.usersMu.RLock()
	defer r.usersMu.RUnlock()
	if name, ok := r.users[id]; ok {
		return name
	}
	return "User " + id
}

func (r *Resolver) SetUsername(id, newUsername string) {
	r.usersMu.Lock()
	defer r.usersMu.Unlock()
	r.users[id] = newUsername
}

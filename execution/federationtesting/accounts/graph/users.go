package graph

import "sync"

var (
	usersMu sync.RWMutex
	users   = map[string]string{
		"1234": "Me",
		"7777": "User 7777",
	}
	defaultUsers = map[string]string{
		"1234": "Me",
		"7777": "User 7777",
	}
)

func GetUsername(id string) string {
	usersMu.RLock()
	defer usersMu.RUnlock()
	if name, ok := users[id]; ok {
		return name
	}
	return "User " + id
}

func SetUsername(id, newUsername string) {
	usersMu.Lock()
	defer usersMu.Unlock()
	users[id] = newUsername
}

func ResetUsers() {
	usersMu.Lock()
	defer usersMu.Unlock()
	users = make(map[string]string)
	for k, v := range defaultUsers {
		users[k] = v
	}
}

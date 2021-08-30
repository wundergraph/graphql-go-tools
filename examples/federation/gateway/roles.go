package main

var roleToInt = map[string]int{
	"ADMIN": 1000,
	"USER":  100,
	"GUEST": 10,
}

func highestRole(roles []string) string {
	var highestRole string

	for _, role := range roles {
		if roleToInt[highestRole] < roleToInt[role] {
			highestRole = role
		}
	}

	return highestRole
}

func lessRoles(r1, r2 string) bool {
	return roleToInt[r1] < roleToInt[r2]
}
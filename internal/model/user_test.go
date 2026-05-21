package model

import "testing"

func TestActorMayManageUser(t *testing.T) {
	sa := &User{Role: RoleSuperadmin}
	user := &User{Role: RoleUser}
	lei := &User{Role: RoleLeitung}

	if !ActorMayManageUser(string(RoleSuperadmin), sa) {
		t.Fatal("superadmin should manage superadmin")
	}
	if ActorMayManageUser(string(RoleLeitung), sa) {
		t.Fatal("leitung must not manage superadmin")
	}
	if ActorMayManageUser(string(RoleLeitung), user) {
		// ok
	} else {
		t.Fatal("leitung should manage normal user")
	}
	if !ActorMayManageUser(string(RoleLeitung), lei) {
		t.Fatal("leitung should manage leitung account")
	}
	if ActorMayManageUser(string(RoleSuperadmin), nil) {
		t.Fatal("nil target")
	}
}

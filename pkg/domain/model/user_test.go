package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

func validUser() *model.User {
	now := time.Now()
	return &model.User{
		TeamID:    "T0123ABCD",
		UserID:    "U0123ABCD",
		Name:      "Alice",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestUser_Validate(t *testing.T) {
	gt.NoError(t, validUser().Validate())

	cases := map[string]func(u *model.User){
		"empty team":       func(u *model.User) { u.TeamID = "" },
		"empty user":       func(u *model.User) { u.UserID = "" },
		"empty name":       func(u *model.User) { u.Name = "" },
		"empty created_at": func(u *model.User) { u.CreatedAt = time.Time{} },
		"empty updated_at": func(u *model.User) { u.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			u := validUser()
			mutate(u)
			gt.Error(t, u.Validate())
		})
	}
}

func TestUser_Key(t *testing.T) {
	gt.Value(t, validUser().Key()).Equal(model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"})
}

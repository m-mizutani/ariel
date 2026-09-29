package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func TestUserKey_Validate(t *testing.T) {
	valid := []model.UserKey{
		{TeamID: "T0123ABCD", UserID: "U0123ABCD"},
		{TeamID: "T0123ABCD", UserID: "W0123ABCD"},
	}
	for _, k := range valid {
		gt.NoError(t, k.Validate())
	}

	invalid := map[string]model.UserKey{
		"empty team":             {TeamID: "", UserID: "U0123ABCD"},
		"empty user":             {TeamID: "T0123ABCD", UserID: ""},
		"lowercase team":         {TeamID: "T0123abcd", UserID: "U0123ABCD"},
		"lowercase user":         {TeamID: "T0123ABCD", UserID: "u0123ABCD"},
		"slash in team":          {TeamID: "T01/23", UserID: "U0123ABCD"},
		"slash in user":          {TeamID: "T0123ABCD", UserID: "U01/23"},
		"team starts with U":     {TeamID: "U0123ABCD", UserID: "U0123ABCD"},
		"user starts with T":     {TeamID: "T0123ABCD", UserID: "T0123ABCD"},
		"team is prefix only":    {TeamID: "T", UserID: "U0123ABCD"},
		"user has trailing dash": {TeamID: "T0123ABCD", UserID: "U0123-"},
	}
	for name, k := range invalid {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, k.Validate())
		})
	}
}

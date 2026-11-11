package router

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
)

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		phone string
		want  string
	}{
		{"", ""},
		{"1", "*"},
		{"12345", "*****"},
		{"123456", "123*56"},
		{"1234567", "123**67"},
		{"12345678", "123***78"},
		{"13800138000", "138******00"},
		{"+8613800138000", "+86138******00"},
		{"+14155552671", "+141******71"},
		{"+85291234567", "+85291****67"},
		{"+447911123456", "+4479******56"},
		{"+420601123456", "+42060*****56"},
		{"+421901123456", "+42190*****56"},
		{"+4236612345", "+42366***45"},
		{"+71234567890", "+712******90"},
		{"+80012345678", "+80012****78"},
		{"+86", "+86"},
		{"+8612345", "+86*****"},
		{"+8521234", "+852****"},
		{"+85212345", "+85212*45"},
		{"138******00", "138******00"},
		{"+86138******00", "+86138******00"},
		{"+85291****67", "+85291****67"},
	}
	for _, tc := range cases {
		t.Run(tc.phone, func(t *testing.T) {
			t.Parallel()
			if got := maskPhone(tc.phone); got != tc.want {
				t.Fatalf("maskPhone(%q) = %q, want %q", tc.phone, got, tc.want)
			}
		})
	}
}

func TestSelfProfileMasksPhoneForAllRoles(t *testing.T) {
	for _, role := range []string{"Standard", models.AdminRole} {
		t.Run(role, func(t *testing.T) {
			user := &models.User{
				Username: "alice",
				Phone:    "13800138000",
				RolesLst: []string{role},
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/n9e/self/profile", nil)
			c.Set("user", user)

			(&Router{}).selfProfileGet(c)

			var response struct {
				Data models.User `json:"dat"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.Phone != "138******00" || response.Data.Username != user.Username {
				t.Fatalf("unexpected user response: %s", w.Body.String())
			}
			if user.Phone != "13800138000" {
				t.Fatalf("response masking changed the original phone: %q", user.Phone)
			}
		})
	}
}

func TestCallbackOutputMasksPhone(t *testing.T) {
	user := &models.User{Username: "alice", Phone: "13800138000"}
	body, err := json.Marshal(CallbackOutput{User: userForResponse(user)})
	if err != nil {
		t.Fatal(err)
	}
	var response CallbackOutput
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.User.Phone != "138******00" || user.Phone != "13800138000" {
		t.Fatalf("unexpected callback phone: response=%q, original=%q", response.User.Phone, user.Phone)
	}
}

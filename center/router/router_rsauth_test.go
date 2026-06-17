package router

import (
	"testing"

	"github.com/golang-jwt/jwt"
)

func TestTokenHasIssuer(t *testing.T) {
	sign := func(claims jwt.MapClaims) string {
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return signed
	}

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			// Shape minted by createTokens — no iss claim, must stay on the
			// existing self-signed session-JWT path.
			name: "session jwt has no issuer",
			raw:  sign(jwt.MapClaims{"authorized": true, "access_uuid": "u", "user_identity": "1-alice"}),
			want: false,
		},
		{
			name: "idp access token has issuer",
			raw:  sign(jwt.MapClaims{"iss": "https://idp.example.com", "sub": "alice"}),
			want: true,
		},
		{
			name: "empty issuer is not an idp token",
			raw:  sign(jwt.MapClaims{"iss": "", "sub": "alice"}),
			want: false,
		},
		{name: "garbage is not a jwt", raw: "not-a-jwt", want: false},
		{name: "empty string", raw: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenHasIssuer(tc.raw); got != tc.want {
				t.Errorf("tokenHasIssuer() = %v, want %v", got, tc.want)
			}
		})
	}
}

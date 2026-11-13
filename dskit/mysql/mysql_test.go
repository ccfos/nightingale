// @Author: Ciusyan 5/11/24

package mysql

import (
	"context"
	"testing"
)

func TestNewMySQLWithSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings interface{}
		wantErr  bool
	}{
		{
			name:     "valid string settings",
			settings: `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`,
			wantErr:  false,
		},
		{
			name:     "invalid settings type",
			settings: 12345,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMySQLWithSettings(context.Background(), tt.settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMySQLWithSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Log(got)
		})
	}
}

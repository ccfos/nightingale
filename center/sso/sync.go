package sso

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

func (s *SsoClient) SyncSsoUsers(ctx *ctx.Context) {
	if err := s.LDAP.SyncAddAndDelUsers(ctx); err != nil {
		fmt.Println("failed to sync the addition and deletion of ldap users:", err)
	}

	if err := s.LDAP.SyncDelUsers(ctx); err != nil {
		fmt.Println("failed to sync deletion of ldap users:", err)
	}

	go s.loopSyncSsoUsers(ctx)
}

func (s *SsoClient) loopSyncSsoUsers(ctx *ctx.Context) {
	for {
		select {
		case <-s.LDAP.Ticker.C:
			lc := s.LDAP.Copy()

			if err := lc.SyncAddAndDelUsers(ctx); err != nil {
				logger.Warningf("failed to sync the addition and deletion of ldap users: %v", err)
			}

			if err := lc.SyncDelUsers(ctx); err != nil {
				logger.Warningf("failed to sync deletion of ldap users: %v", err)
			}
		}
	}
}

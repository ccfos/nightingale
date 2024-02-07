package sso

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"time"
)

func (s *SsoClient) SyncSsoUsers(ctx *ctx.Context) {
	err := s.LDAP.SyncUserAddAndDel(ctx)
	if err != nil {
		fmt.Println("failed to sync the addition and deletion of ldap users:", err)
	}

	go s.loopSyncSsoUsers(ctx)
}

func (s *SsoClient) loopSyncSsoUsers(ctx *ctx.Context) {
	for {
		select {
		case <-s.LDAP.Ticker.C:
			if s.LDAP.SyncUsers {
				if err := s.LDAP.SyncUserAddAndDel(ctx); err != nil {
					logger.Warningf("failed to sync the addition and deletion of ldap users: %v", err)
				}
			}
		}
	}
}

func (s *SsoClient) SyncSsoUserDel(ctx *ctx.Context) {
	err := s.LDAP.SyncUserDel(ctx)
	if err != nil {
		fmt.Println("failed to sync deletion of ldap users:", err)
	}
	go s.loopSyncSsoUserDel(ctx)
}

func (s *SsoClient) loopSyncSsoUserDel(ctx *ctx.Context) {
	duration := 24 * time.Hour
	for {
		time.Sleep(duration)
		if err := s.LDAP.SyncUserDel(ctx); err != nil {
			logger.Warningf("failed to sync deletion of ldap users: %v", err)
		}
	}
}

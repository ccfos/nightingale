package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertRuleUpdateColumn_LegacyRoutingClearsNewNotificationRules(t *testing.T) {
	for _, column := range []string{"notify_groups", "notify_channels"} {
		t.Run(column, func(t *testing.T) {
			c := permissionContext(t, &AlertRule{})
			rule := AlertRule{Id: 1, NotifyVersion: 1, NotifyRuleIds: []int64{9}}
			require.NoError(t, c.DB.Create(&rule).Error)

			err := rule.UpdateColumn(c, column, "7")

			require.NoError(t, err)
			var stored AlertRule
			require.NoError(t, c.DB.First(&stored, 1).Error)
			require.Zero(t, stored.NotifyVersion)
			require.Empty(t, stored.NotifyRuleIds)
			if column == "notify_groups" {
				require.Equal(t, "7", stored.NotifyGroups)
			} else {
				require.Equal(t, "7", stored.NotifyChannels)
			}
		})
	}
}
